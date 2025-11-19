package writer

import (
	"bytes"
	"fmt"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"sort"
)

type impl struct{ interceptors []Interceptor }

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

func (w *impl) Write(ctx Context, doc *semantic.Document, out WriterAt, cfg Config) (err error) {
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
	version := pdfVersion(cfg)
	incr := incrementalContext(doc, out, cfg)
	idPair := fileID(doc, cfg)

	builder := newObjectBuilder(doc, cfg, incr.startObjNum)
	objects, catalogRef, infoRef, encryptRef, err := builder.Build()
	if err != nil {
		return err
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
	offsets := make(map[int]int64)
	for k, v := range incr.prevOffsets {
		offsets[k] = v
	}

	ordered := make([]raw.ObjectRef, 0, len(objects))
	for ref := range objects {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Num < ordered[j].Num })
	for _, ref := range ordered {
		if err := checkCancelled(); err != nil {
			return err
		}
		offset := initialOffset + int64(buf.Len())
		serialized, _ := w.SerializeObject(ref, objects[ref])
		buf.Write(serialized)
		offsets[ref.Num] = offset
	}
	// XRef
	if cfg.XRefStreams {
		xrefRef := builder.nextRef()
		maxObjNum := xrefRef.Num
		if len(offsets) > 0 {
			for n := range offsets {
				if n > maxObjNum {
					maxObjNum = n
				}
			}
		}
		xrefOffset := initialOffset + int64(buf.Len())
		offsets[xrefRef.Num] = xrefOffset
		size := maxInt(maxObjNum, incr.prevMaxObj) + 1

		trailer := buildTrailer(size, catalogRef, infoRef, encryptRef, doc, cfg, incr.prevOffset, idPair)
		trailer.Set(raw.NameLiteral("Type"), raw.NameLiteral("XRef"))
		trailer.Set(raw.NameLiteral("W"), raw.NewArray(raw.NumberInt(1), raw.NumberInt(4), raw.NumberInt(1)))
		indexArr, entries := xrefStreamIndexAndEntries(offsets)
		trailer.Set(raw.NameLiteral("Index"), indexArr)
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
			if off, ok := offsets[i]; ok {
				buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
			} else {
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
