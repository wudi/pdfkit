package writer

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

func (w *impl) Write(ctx Context, doc *semantic.Document, out WriterAt, cfg Config) error {
	if doc.Encrypted && !(doc.Permissions.Modify || doc.Permissions.Assemble) {
		return fmt.Errorf("document permissions forbid modification")
	}
	version := pdfVersion(cfg)
	incr := incrementalContext(doc, out, cfg)
	// Build raw objects from semantic (minimal subset: catalog, pages, page, fonts, content streams)
	objects := make(map[raw.ObjectRef]raw.Object)
	objNum := incr.startObjNum
	nextRef := func() raw.ObjectRef {
		ref := raw.ObjectRef{Num: objNum, Gen: 0}
		objNum++
		return ref
	}
	catalogRef := nextRef()
	pagesRef := nextRef()
	pageRefs := make([]raw.ObjectRef, 0, len(doc.Pages))

	// Fonts (shared across pages by BaseFont)
	fontRefs := map[string]raw.ObjectRef{}
	ensureFont := func(base string) raw.ObjectRef {
		if base == "" {
			base = "Helvetica"
		}
		if ref, ok := fontRefs[base]; ok {
			return ref
		}
		ref := nextRef()
		fontDict := raw.Dict()
		fontDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
		fontDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Type1"))
		fontDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral(base))
		objects[ref] = fontDict
		fontRefs[base] = ref
		return ref
	}

	// Document info dictionary (Title only for now)
	var infoRef *raw.ObjectRef
	if doc.Info != nil && doc.Info.Title != "" {
		ref := nextRef()
		infoRef = &ref
		infoDict := raw.Dict()
		infoDict.Set(raw.NameLiteral("Title"), raw.Str([]byte(doc.Info.Title)))
		objects[ref] = infoDict
	}

	// XMP metadata stream reference
	var metadataRef *raw.ObjectRef
	if doc.Metadata != nil && len(doc.Metadata.Raw) > 0 {
		ref := nextRef()
		metadataRef = &ref
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Metadata"))
		dict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("XML"))
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(doc.Metadata.Raw))))
		objects[ref] = raw.NewStream(dict, doc.Metadata.Raw)
	}

	// Page content streams
	contentRefs := []raw.ObjectRef{}
	for _, p := range doc.Pages {
		contentData := []byte{}
		for _, cs := range p.Contents {
			contentData = append(contentData, cs.RawBytes...)
		}
		contentRef := nextRef()
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(contentData))))
		objects[contentRef] = raw.NewStream(dict, contentData)
		contentRefs = append(contentRefs, contentRef)
	}
	// Pages
	unionFonts := raw.Dict()
	for i, p := range doc.Pages {
		ref := nextRef()
		pageRefs = append(pageRefs, ref)
		p.MediaBox = semantic.Rectangle{0, 0, p.MediaBox.URX, p.MediaBox.URY}
		pageDict := raw.Dict()
		pageDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
		pageDict.Set(raw.NameLiteral("Parent"), raw.Ref(pagesRef.Num, pagesRef.Gen))
		// MediaBox
		pageDict.Set(raw.NameLiteral("MediaBox"), rectArray(p.MediaBox))
		if cropSet(p.CropBox) {
			pageDict.Set(raw.NameLiteral("CropBox"), rectArray(p.CropBox))
		}
		if rot := normalizeRotation(p.Rotate); rot != 0 {
			pageDict.Set(raw.NameLiteral("Rotate"), raw.NumberInt(int64(rot)))
		}
		if p.UserUnit > 0 {
			pageDict.Set(raw.NameLiteral("UserUnit"), raw.NumberFloat(p.UserUnit))
		}
		// Resources
		resDict := raw.Dict()
		fontResDict := raw.Dict()
		if p.Resources != nil && len(p.Resources.Fonts) > 0 {
			for name, font := range p.Resources.Fonts {
				fRef := ensureFont(font.BaseFont)
				fontResDict.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				if _, ok := unionFonts.KV[name]; !ok {
					unionFonts.Set(raw.NameLiteral(name), raw.Ref(fRef.Num, fRef.Gen))
				}
			}
		} else {
			fRef := ensureFont("Helvetica")
			fontResDict.Set(raw.NameLiteral("F1"), raw.Ref(fRef.Num, fRef.Gen))
			if _, ok := unionFonts.KV["F1"]; !ok {
				unionFonts.Set(raw.NameLiteral("F1"), raw.Ref(fRef.Num, fRef.Gen))
			}
		}
		resDict.Set(raw.NameLiteral("Font"), fontResDict)
		pageDict.Set(raw.NameLiteral("Resources"), resDict)
		// Contents
		contentRef := contentRefs[i]
		pageDict.Set(raw.NameLiteral("Contents"), raw.Ref(contentRef.Num, contentRef.Gen))
		objects[ref] = pageDict
	}
	// Pages tree
	kidsArr := raw.NewArray()
	for _, r := range pageRefs {
		kidsArr.Append(raw.Ref(r.Num, r.Gen))
	}
	pagesDict := raw.Dict()
	pagesDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pagesDict.Set(raw.NameLiteral("Count"), raw.NumberInt(int64(len(pageRefs))))
	pagesDict.Set(raw.NameLiteral("Kids"), kidsArr)
	if unionFonts.Len() > 0 {
		pagesRes := raw.Dict()
		pagesRes.Set(raw.NameLiteral("Font"), unionFonts)
		pagesDict.Set(raw.NameLiteral("Resources"), pagesRes)
	}
	objects[pagesRef] = pagesDict
	// Catalog
	catalogDict := raw.Dict()
	catalogDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	catalogDict.Set(raw.NameLiteral("Pages"), raw.Ref(pagesRef.Num, pagesRef.Gen))
	if metadataRef != nil {
		catalogDict.Set(raw.NameLiteral("Metadata"), raw.Ref(metadataRef.Num, metadataRef.Gen))
	}
	objects[catalogRef] = catalogDict

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

	ordered := make([]raw.ObjectRef, 0, len(objects))
	for ref := range objects {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Num < ordered[j].Num })
	for _, ref := range ordered {
		offset := initialOffset + int64(buf.Len())
		serialized, _ := w.SerializeObject(ref, objects[ref])
		buf.Write(serialized)
		offsets[ref.Num] = offset
	}
	// XRef
	if cfg.XRefStreams {
		xrefRef := nextRef()
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

		trailer := buildTrailer(size, catalogRef, infoRef, doc, cfg, incr.prevOffset)
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
		trailer := buildTrailer(size, catalogRef, infoRef, doc, cfg, incr.prevOffset)
		buf.WriteString("trailer\n")
		buf.Write(serializePrimitive(trailer))
		buf.WriteString("\nstartxref\n")
		buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))
	}

	_, err := out.Write(buf.Bytes())
	return err
}

func serializePrimitive(o raw.Object) []byte {
	switch v := o.(type) {
	case raw.NameObj:
		return []byte("/" + v.Value())
	case raw.NumberObj:
		if v.IsInteger() {
			return []byte(fmt.Sprintf("%d", v.Int()))
		}
		return []byte(fmt.Sprintf("%f", v.Float()))
	case raw.BoolObj:
		if v.Value() {
			return []byte("true")
		}
		return []byte("false")
	case raw.NullObj:
		return []byte("null")
	case raw.String:
		if v.IsHex() {
			dst := make([]byte, hex.EncodedLen(len(v.Value())))
			hex.Encode(dst, v.Value())
			return []byte("<" + strings.ToUpper(string(dst)) + ">")
		}
		return escapeLiteralString(v.Value())
	case *raw.ArrayObj:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, it := range v.Items {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.Write(serializePrimitive(it))
		}
		b.WriteByte(']')
		return b.Bytes()
	case *raw.DictObj:
		var b bytes.Buffer
		b.WriteString("<<")
		keys := make([]string, 0, len(v.KV))
		for k := range v.KV {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("/" + k + " ")
			b.Write(serializePrimitive(v.KV[k]))
		}
		b.WriteString(">>")
		return b.Bytes()
	case *raw.StreamObj:
		var b bytes.Buffer
		d := serializePrimitive(v.Dict)
		b.Write(d)
		b.WriteString("stream\n")
		b.Write(v.Data)
		b.WriteString("\nendstream")
		return b.Bytes()
	case raw.RefObj:
		return []byte(fmt.Sprintf("%d %d R", v.Ref().Num, v.Ref().Gen))
	default:
		return []byte("null")
	}
}

func pdfVersion(cfg Config) string {
	if cfg.Version == "" {
		return string(PDF17)
	}
	return string(cfg.Version)
}

func fileID(doc *semantic.Document, cfg Config) [2][]byte {
	seed := deterministicIDSeed(doc, cfg)
	if cfg.Deterministic {
		return [2][]byte{seed, seed}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		id = seed
	}
	idB := make([]byte, len(id))
	copy(idB, id)
	return [2][]byte{id, idB}
}

func deterministicIDSeed(doc *semantic.Document, cfg Config) []byte {
	h := sha256.New()
	h.Write([]byte(pdfVersion(cfg)))
	if doc.Info != nil {
		h.Write([]byte(doc.Info.Title))
	}
	if doc.Metadata != nil {
		h.Write(doc.Metadata.Raw)
	}
	h.Write([]byte(fmt.Sprintf("%d", len(doc.Pages))))
	for _, p := range doc.Pages {
		h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f-%d", p.MediaBox.LLX, p.MediaBox.LLY, p.MediaBox.URX, p.MediaBox.URY, p.Rotate)))
	}
	sum := h.Sum(nil)
	if len(sum) >= 16 {
		return sum[:16]
	}
	buf := make([]byte, 16)
	copy(buf, sum)
	return buf
}

func buildTrailer(size int, catalogRef raw.ObjectRef, infoRef *raw.ObjectRef, doc *semantic.Document, cfg Config, prev int64) *raw.DictObj {
	trailer := raw.Dict()
	trailer.Set(raw.NameLiteral("Size"), raw.NumberInt(int64(size)))
	trailer.Set(raw.NameLiteral("Root"), raw.Ref(catalogRef.Num, catalogRef.Gen))
	if infoRef != nil {
		trailer.Set(raw.NameLiteral("Info"), raw.Ref(infoRef.Num, infoRef.Gen))
	}
	fileIDs := fileID(doc, cfg)
	idArr := raw.NewArray(
		raw.Str([]byte(hex.EncodeToString(fileIDs[0]))),
		raw.Str([]byte(hex.EncodeToString(fileIDs[1]))),
	)
	trailer.Set(raw.NameLiteral("ID"), idArr)
	if prev > 0 && cfg.Incremental {
		trailer.Set(raw.NameLiteral("Prev"), raw.NumberInt(prev))
	}
	return trailer
}

func xrefStreamIndexAndEntries(offsets map[int]int64) (*raw.ArrayObj, []byte) {
	offCopy := make(map[int]int64, len(offsets)+1)
	for k, v := range offsets {
		offCopy[k] = v
	}
	if _, ok := offCopy[0]; !ok {
		offCopy[0] = 0
	}
	keys := make([]int, 0, len(offCopy))
	for k := range offCopy {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	indexArr := raw.NewArray()
	var entries []byte
	segStart := -1
	prev := -1
	for _, k := range keys {
		if segStart == -1 {
			segStart = k
			prev = k
		} else if k != prev+1 {
			indexArr.Append(raw.NumberInt(int64(segStart)))
			indexArr.Append(raw.NumberInt(int64(prev - segStart + 1)))
			segStart = k
		}
		prev = k

		off := offCopy[k]
		typ := 0
		gen := 0
		if k == 0 {
			gen = 65535
		}
		if off > 0 {
			typ = 1
		}
		entries = appendXRefStreamEntry(entries, typ, off, gen)
	}
	if segStart != -1 {
		indexArr.Append(raw.NumberInt(int64(segStart)))
		indexArr.Append(raw.NumberInt(int64(prev - segStart + 1)))
	}
	return indexArr, entries
}

func appendXRefStreamEntry(buf []byte, typ int, field2 int64, gen int) []byte {
	buf = append(buf, byte(typ))
	offset := uint32(field2)
	buf = append(buf, byte(offset>>24), byte(offset>>16), byte(offset>>8), byte(offset))
	buf = append(buf, byte(gen))
	return buf
}

// escapeLiteralString escapes characters that need backslash protection in literal strings.
func escapeLiteralString(rawBytes []byte) []byte {
	var b bytes.Buffer
	b.WriteByte('(')
	for _, ch := range rawBytes {
		switch ch {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteByte(ch)
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		default:
			if ch < 0x20 || ch >= 0x80 {
				fmt.Fprintf(&b, "\\%03o", ch)
			} else {
				b.WriteByte(ch)
			}
		}
	}
	b.WriteByte(')')
	return b.Bytes()
}

type incrementalInfo struct {
	base        []byte
	prevOffset  int64
	startObjNum int
	prevMaxObj  int
}

func incrementalContext(doc *semantic.Document, out WriterAt, cfg Config) incrementalInfo {
	info := incrementalInfo{startObjNum: 1}
	if !cfg.Incremental {
		return info
	}
	reader, ok := out.(interface{ Bytes() []byte })
	if !ok {
		return info
	}
	info.base = append([]byte(nil), reader.Bytes()...)
	info.prevOffset = lastStartXRef(info.base)
	info.prevMaxObj = maxObjNumFromBytes(info.base)
	info.startObjNum = info.prevMaxObj + 1
	if dec := doc.Decoded(); dec != nil && dec.Raw != nil {
		for ref := range dec.Raw.Objects {
			if ref.Num >= info.startObjNum {
				info.startObjNum = ref.Num + 1
			}
		}
	}
	if info.startObjNum < 1 {
		info.startObjNum = 1
	}
	return info
}

func lastStartXRef(data []byte) int64 {
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	matches := re.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return 0
	}
	// use last occurrence
	m := matches[len(matches)-1]
	off, err := strconv.ParseInt(string(m[1]), 10, 64)
	if err != nil {
		return 0
	}
	return off
}

func maxObjNumFromBytes(data []byte) int {
	re := regexp.MustCompile(`\s(\d+)\s+0\s+obj`)
	matches := re.FindAllSubmatch(data, -1)
	max := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		num, err := strconv.Atoi(string(m[1]))
		if err != nil {
			continue
		}
		if num > max {
			max = num
		}
	}
	return max
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func rectArray(r semantic.Rectangle) *raw.ArrayObj {
	return raw.NewArray(
		raw.NumberFloat(r.LLX),
		raw.NumberFloat(r.LLY),
		raw.NumberFloat(r.URX),
		raw.NumberFloat(r.URY),
	)
}

func cropSet(r semantic.Rectangle) bool {
	return r.URX > r.LLX && r.URY > r.LLY
}

func normalizeRotation(rot int) int {
	if rot == 0 {
		return 0
	}
	rot %= 360
	if rot < 0 {
		rot += 360
	}
	return rot
}
