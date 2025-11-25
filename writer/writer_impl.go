package writer

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/wudi/pdfkit/fonts"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type impl struct {
	interceptors     []Interceptor
	annotSerializer  AnnotationSerializer
	actionSerializer ActionSerializer
	csSerializer     ColorSpaceSerializer
	funcSerializer   FunctionSerializer
}

func (w *impl) SerializeObject(ref raw.ObjectRef, obj raw.Object) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d %d obj\n", ref.Num, ref.Gen))
	switch o := obj.(type) {
	case *raw.DictObj:
		buf.WriteString("<<")
		keys := make([]string, 0, len(o.KV))
		for k := range o.KV {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("/" + k + " ")
			buf.Write(serializePrimitive(o.KV[k]))
		}
		buf.WriteString(">>\n")
	case *raw.ArrayObj, raw.NameObj, raw.NumberObj, raw.BoolObj, raw.NullObj, raw.StringObj, raw.HexStringObj, *raw.StreamObj, raw.RefObj:
		buf.Write(serializePrimitive(o))
		buf.WriteString("\n")
	default:
		buf.WriteString("null\n")
	}
	buf.WriteString("endobj\n")
	return buf.Bytes(), nil
}

func (w *impl) Write(ctx context.Context, doc *semantic.Document, out WriterAt, cfg Config) (err error) {
	checkCancelled := func() error {
		select {
		case <-ctx.Done():
			return fmt.Errorf("write cancelled")
		default:
			return nil
		}
	}
	if err := checkCancelled(); err != nil {
		return err
	}

	// Run optimization
	if cfg.Optimizer != nil {
		if err := cfg.Optimizer.Optimize(ctx, doc); err != nil {
			return err
		}
	}

	if cfg.SubsetFonts {
		analyzer := fonts.NewAnalyzer()
		analyzer.Analyze(doc)
		planner := fonts.NewPlanner()
		planner.Plan(analyzer)
		subsetter := fonts.NewSubsetter()
		subsetter.Apply(doc, planner)
	}

	if cfg.Linearize {
		return w.writeLinearized(ctx, doc, out, cfg)
	}

	if cfg.ObjectStreams {
		cfg.XRefStreams = true
	}

	version := pdfVersion(cfg)
	incr := incrementalContext(doc, out, cfg)
	idPair := fileID(doc, cfg)

	builder := newObjectBuilder(doc, cfg, incr.startObjNum, idPair, w.annotSerializer, w.actionSerializer, w.csSerializer, w.funcSerializer)
	objects, catalogRef, infoRef, encryptRef, err := builder.Build()
	if err != nil {
		return err
	}

	// Run raw optimization
	rawDoc := &raw.Document{Objects: objects}
	if cfg.Optimizer != nil {
		if err := cfg.Optimizer.OptimizeRaw(ctx, rawDoc); err != nil {
			return err
		}
	}

	// Serialize
	var buf bytes.Buffer
	initialOffset := int64(0)
	if len(incr.base) > 0 {
		initialOffset = int64(len(incr.base))
		if !bytes.HasSuffix(incr.base, []byte("\n")) {
			buf.WriteByte('\n')
		}
	} else {
		buf.WriteString("%PDF-" + version + "\n%\xE2\xE3\xCF\xD3\n")
	}

	xrefEntries := make(map[int]xrefEntry)
	for k, v := range incr.prevOffsets {
		xrefEntries[k] = xrefEntry{typ: 1, field2: v, field3: 0}
	}

	ordered := make([]raw.ObjectRef, 0, len(objects))
	for ref := range objects {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Num < ordered[j].Num })

	// Object Stream Buffer
	type bufferedObj struct {
		ref raw.ObjectRef
		obj raw.Object
	}
	var objStmBuffer []bufferedObj

	flushObjStm := func() error {
		if len(objStmBuffer) == 0 {
			return nil
		}
		stmRef := builder.nextRef()

		// Serialize objects into stream data
		var dataBuf bytes.Buffer
		var headerBuf bytes.Buffer

		for i, item := range objStmBuffer {
			if i == 0 {
				// First object offset is 0 relative to First
			}

			// Serialize object content (primitive only)
			content := serializePrimitive(item.obj)

			// Append to header: objNum offset
			headerBuf.WriteString(fmt.Sprintf("%d %d ", item.ref.Num, dataBuf.Len()))

			// Append to data
			dataBuf.Write(content)
			dataBuf.WriteByte(' ') // Separator

			// Record XRef entry
			xrefEntries[item.ref.Num] = xrefEntry{
				typ:    2,
				field2: int64(stmRef.Num),
				field3: i,
			}
		}

		// Create ObjStm object
		stmDict := raw.Dict()
		stmDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("ObjStm"))
		stmDict.Set(raw.NameLiteral("N"), raw.NumberInt(int64(len(objStmBuffer))))
		stmDict.Set(raw.NameLiteral("First"), raw.NumberInt(int64(headerBuf.Len())))

		// Combine header and data
		fullData := append(headerBuf.Bytes(), dataBuf.Bytes()...)

		// Compress if needed
		if cfg.Compression > 0 {
			compressed, err := flateEncode(fullData, cfg.Compression)
			if err == nil {
				fullData = compressed
				stmDict.Set(raw.NameLiteral("Filter"), raw.NameLiteral("FlateDecode"))
			}
		}
		stmDict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(fullData))))

		stmObj := raw.NewStream(stmDict, fullData)

		// Write ObjStm
		offset := initialOffset + int64(buf.Len())
		serialized, _ := w.SerializeObject(stmRef, stmObj)
		buf.Write(serialized)
		xrefEntries[stmRef.Num] = xrefEntry{typ: 1, field2: offset, field3: 0}

		objStmBuffer = objStmBuffer[:0]
		return nil
	}

	for _, ref := range ordered {
		if err := checkCancelled(); err != nil {
			return err
		}

		obj := objects[ref]

		// Check if suitable for ObjStm
		isSuitable := false
		if cfg.ObjectStreams {
			switch obj.(type) {
			case *raw.StreamObj:
				isSuitable = false
			case raw.RefObj:
				isSuitable = true // Indirect references are fine
			default:
				// Exclude Encryption, Linearization, etc. if we can identify them.
				// For now, exclude if it's the Encrypt dictionary
				if encryptRef != nil && ref == *encryptRef {
					isSuitable = false
				} else if infoRef != nil && ref == *infoRef {
					isSuitable = true // Info can be compressed
				} else {
					isSuitable = true
				}
			}
			// Generation must be 0
			if ref.Gen != 0 {
				isSuitable = false
			}
		}

		if isSuitable {
			objStmBuffer = append(objStmBuffer, bufferedObj{ref, obj})
			if len(objStmBuffer) >= 100 { // Batch size
				if err := flushObjStm(); err != nil {
					return err
				}
			}
		} else {
			offset := initialOffset + int64(buf.Len())
			serialized, _ := w.SerializeObject(ref, obj)
			buf.Write(serialized)
			xrefEntries[ref.Num] = xrefEntry{typ: 1, field2: offset, field3: ref.Gen}
		}
	}
	// Flush remaining
	if err := flushObjStm(); err != nil {
		return err
	}

	// XRef
	if cfg.XRefStreams {
		xrefRef := builder.nextRef()
		maxObjNum := xrefRef.Num
		for n := range xrefEntries {
			if n > maxObjNum {
				maxObjNum = n
			}
		}
		xrefOffset := initialOffset + int64(buf.Len())
		xrefEntries[xrefRef.Num] = xrefEntry{typ: 1, field2: xrefOffset, field3: 0}
		size := maxInt(maxObjNum, incr.prevMaxObj) + 1

		trailer := buildTrailer(size, catalogRef, infoRef, encryptRef, doc, cfg, incr.prevOffset, idPair)
		trailer.Set(raw.NameLiteral("Type"), raw.NameLiteral("XRef"))
		trailer.Set(raw.NameLiteral("W"), raw.NewArray(raw.NumberInt(1), raw.NumberInt(4), raw.NumberInt(1)))
		indexArr, entries := xrefStreamIndexAndEntries(xrefEntries)
		trailer.Set(raw.NameLiteral("Index"), indexArr)
		trailer.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(entries))))

		// Compress XRef stream
		if cfg.Compression > 0 {
			compressed, err := flateEncode(entries, cfg.Compression)
			if err == nil {
				entries = compressed
				trailer.Set(raw.NameLiteral("Filter"), raw.NameLiteral("FlateDecode"))
			}
		}
		trailer.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(entries))))

		xrefStream := raw.NewStream(trailer, entries)
		serialized, _ := w.SerializeObject(xrefRef, xrefStream)
		buf.Write(serialized)

		buf.WriteString("startxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	} else {
		xrefOffset := initialOffset + int64(buf.Len())
		maxObjNum := ordered[len(ordered)-1].Num
		size := maxInt(maxObjNum, incr.prevMaxObj) + 1
		buf.WriteString("xref\n0 ")
		buf.WriteString(fmt.Sprintf("%d\n", size))
		buf.WriteString("0000000000 65535 f \n")
		for i := 1; i <= maxObjNum; i++ {
			if e, ok := xrefEntries[i]; ok && e.typ == 1 {
				buf.WriteString(fmt.Sprintf("%010d %05d n \n", e.field2, e.field3))
			} else {
				// For ObjStm objects (type 2), they are not in standard xref table.
				// But wait, if we are NOT using XRefStreams, we CANNOT use ObjStm.
				// The code at the top forces XRefStreams if ObjectStreams is true.
				// So we should be safe here.
				buf.WriteString("0000000000 65535 f \n")
			}
		}
		trailer := buildTrailer(size, catalogRef, infoRef, encryptRef, doc, cfg, incr.prevOffset, idPair)
		buf.WriteString("trailer\n")
		buf.Write(serializePrimitive(trailer))
		buf.WriteString("\nstartxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	}

	_, err = out.Write(buf.Bytes())
	return err
}
