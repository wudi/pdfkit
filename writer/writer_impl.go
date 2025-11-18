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
	case *raw.ArrayObj, raw.NameObj, raw.NumberObj, raw.BoolObj, raw.NullObj, raw.StringObj, *raw.StreamObj, raw.RefObj:
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
	// Build raw objects from semantic (minimal subset: catalog, pages, page, fonts, content streams)
	objects := make(map[raw.ObjectRef]raw.Object)
	objNum := 1
	catalogRef := raw.ObjectRef{Num: objNum, Gen: 0}
	objNum++
	pagesRef := raw.ObjectRef{Num: objNum, Gen: 0}
	objNum++
	pageRefs := make([]raw.ObjectRef, 0, len(doc.Pages))

	// Fonts (single shared Helvetica core font)
	fontRef := raw.ObjectRef{Num: objNum, Gen: 0}
	objNum++
	fontDict := raw.Dict()
	fontDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
	fontDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Type1"))
	fontDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral("Helvetica"))
	objects[fontRef] = fontDict

	// Page content streams
	contentRefs := []raw.ObjectRef{}
	for _, p := range doc.Pages {
		contentData := []byte{}
		for _, cs := range p.Contents {
			contentData = append(contentData, cs.RawBytes...)
		}
		contentRef := raw.ObjectRef{Num: objNum, Gen: 0}
		objNum++
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Length"), raw.NumberInt(int64(len(contentData))))
		objects[contentRef] = raw.NewStream(dict, contentData)
		contentRefs = append(contentRefs, contentRef)
	}
	// Pages
	for i, p := range doc.Pages {
		ref := raw.ObjectRef{Num: objNum, Gen: 0}
		objNum++
		pageRefs = append(pageRefs, ref)
		p.MediaBox = semantic.Rectangle{0, 0, p.MediaBox.URX, p.MediaBox.URY}
		pageDict := raw.Dict()
		pageDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
		pageDict.Set(raw.NameLiteral("Parent"), raw.Ref(pagesRef.Num, pagesRef.Gen))
		// MediaBox
		mediaArr := raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(int64(p.MediaBox.URX)), raw.NumberInt(int64(p.MediaBox.URY)))
		pageDict.Set(raw.NameLiteral("MediaBox"), mediaArr)
		// Resources
		resDict := raw.Dict()
		fontResDict := raw.Dict()
		fontResDict.Set(raw.NameLiteral("F1"), raw.Ref(fontRef.Num, fontRef.Gen))
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
	objects[pagesRef] = pagesDict
	// Catalog
	catalogDict := raw.Dict()
	catalogDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	catalogDict.Set(raw.NameLiteral("Pages"), raw.Ref(pagesRef.Num, pagesRef.Gen))
	objects[catalogRef] = catalogDict

	// Serialize
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	offsets := make(map[int]int64)

	ordered := make([]raw.ObjectRef, 0, len(objects))
	for ref := range objects {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Num < ordered[j].Num })
	for _, ref := range ordered {
		offset := int64(buf.Len())
		serialized, _ := w.SerializeObject(ref, objects[ref])
		buf.Write(serialized)
		offsets[ref.Num] = offset
	}
	// XRef
	xrefOffset := buf.Len()
	maxObjNum := ordered[len(ordered)-1].Num
	buf.WriteString("xref\n0 ")
	buf.WriteString(fmt.Sprintf("%d\n", maxObjNum+1))
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxObjNum; i++ {
		if off, ok := offsets[i]; ok {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
		} else {
			buf.WriteString("0000000000 65535 f \n")
		}
	}
	// Trailer
	buf.WriteString("trailer\n<<")
	buf.WriteString("/Size ")
	buf.WriteString(fmt.Sprintf("%d ", maxObjNum+1))
	buf.WriteString("/Root ")
	buf.WriteString(fmt.Sprintf("%d 0 R", catalogRef.Num))
	buf.WriteString(">>\nstartxref\n")
	buf.WriteString(fmt.Sprintf("%d\n%%EOF\n", xrefOffset))

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
	case raw.StringObj:
		return []byte("(" + string(v.Value()) + ")")
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
