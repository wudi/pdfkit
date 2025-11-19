package writer

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"crypto/rand"
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/hex"
	"fmt"
	"math"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/security"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

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
		h.Write([]byte(doc.Info.Author))
		h.Write([]byte(doc.Info.Subject))
		h.Write([]byte(doc.Info.Creator))
		h.Write([]byte(doc.Info.Producer))
		if len(doc.Info.Keywords) > 0 {
			h.Write([]byte(strings.Join(doc.Info.Keywords, ",")))
		}
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

func rectArray(r semantic.Rectangle) *raw.ArrayObj {
	return raw.NewArray(
		raw.NumberFloat(r.LLX),
		raw.NumberFloat(r.LLY),
		raw.NumberFloat(r.URX),
		raw.NumberFloat(r.URY),
	)
}

func cropSet(r semantic.Rectangle) bool {
	return r.LLX != 0 || r.LLY != 0 || r.URX != 0 || r.URY != 0
}

func normalizeRotation(rot int) int {
	rot = rot % 360
	if rot < 0 {
		rot += 360
	}
	if rot%90 != 0 {
		return 0
	}
	return rot
}

func pickContentFilter(cfg Config) ContentFilter {
	if cfg.ContentFilter != FilterNone {
		return cfg.ContentFilter
	}
	if cfg.Compression != 0 {
		return FilterFlate
	}
	return FilterNone
}

func flateEncode(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, level)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func asciiHexEncode(data []byte) []byte {
	dst := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(dst, data)
	return dst
}

func ascii85Encode(data []byte) []byte {
	dst := make([]byte, ascii85.MaxEncodedLen(len(data)))
	n := ascii85.Encode(dst, data)
	return append(dst[:n], []byte("~>")...)
}

func runLengthEncode(data []byte) []byte {
	// Simplified RunLength encoder (just literal copy for now as placeholder, real one is complex)
	// In production this should be a real RLE encoder.
	// For now, we just return data as is (which is valid RLE if we don't compress)
	// Wait, RLE decoder expects RLE stream.
	// Let's implement a basic one.
	var buf bytes.Buffer
	for i := 0; i < len(data); {
		// Find run
		runLen := 1
		for i+runLen < len(data) && runLen < 128 && data[i+runLen] == data[i] {
			runLen++
		}
		if runLen > 1 {
			buf.WriteByte(byte(257 - runLen))
			buf.WriteByte(data[i])
			i += runLen
			continue
		}
		// Literal
		litLen := 1
		for i+litLen < len(data) && litLen < 128 && (i+litLen+1 >= len(data) || data[i+litLen] != data[i+litLen+1]) {
			litLen++
		}
		buf.WriteByte(byte(litLen - 1))
		buf.Write(data[i : i+litLen])
		i += litLen
	}
	buf.WriteByte(128) // EOD
	return buf.Bytes()
}

func lzwEncode(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzw.LSB, 8)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildToUnicodeCMap(font *semantic.Font) []byte {
	if font == nil || len(font.ToUnicode) == 0 {
		return nil
	}
	keys := make([]int, 0, len(font.ToUnicode))
	for cid := range font.ToUnicode {
		keys = append(keys, cid)
	}
	sort.Ints(keys)
	if len(keys) == 0 {
		return nil
	}
	registry, ordering, supplement := "Adobe", "Identity", 0
	if font.CIDSystemInfo != nil {
		if font.CIDSystemInfo.Registry != "" {
			registry = font.CIDSystemInfo.Registry
		}
		if font.CIDSystemInfo.Ordering != "" {
			ordering = font.CIDSystemInfo.Ordering
		}
		supplement = font.CIDSystemInfo.Supplement
	} else if font.DescendantFont != nil {
		registry = font.DescendantFont.CIDSystemInfo.Registry
		ordering = font.DescendantFont.CIDSystemInfo.Ordering
		supplement = font.DescendantFont.CIDSystemInfo.Supplement
	}
	name := font.BaseFont
	if name == "" {
		name = "ToUnicode"
	}
	name = strings.ReplaceAll(name, " ", "") + "-UTF16"
	minCID, maxCID := keys[0], keys[len(keys)-1]
	var buf bytes.Buffer
	buf.WriteString("/CIDInit /ProcSet findresource begin\n")
	buf.WriteString("12 dict begin\n")
	buf.WriteString("begincmap\n")
	buf.WriteString(fmt.Sprintf("/CIDSystemInfo << /Registry (%s) /Ordering (%s) /Supplement %d >> def\n", registry, ordering, supplement))
	buf.WriteString(fmt.Sprintf("/CMapName /%s def\n", name))
	buf.WriteString("/CMapType 2 def\n")
	buf.WriteString("1 begincodespacerange\n")
	buf.WriteString(fmt.Sprintf("<%04X> <%04X>\n", minCID, maxCID))
	buf.WriteString("endcodespacerange\n")
	for i := 0; i < len(keys); {
		chunk := len(keys) - i
		if chunk > 100 {
			chunk = 100
		}
		buf.WriteString(fmt.Sprintf("%d beginbfchar\n", chunk))
		for j := 0; j < chunk; j++ {
			cid := keys[i+j]
			buf.WriteString(fmt.Sprintf("<%04X> <%s>\n", cid, utf16Hex(font.ToUnicode[cid])))
		}
		buf.WriteString("endbfchar\n")
		i += chunk
	}
	buf.WriteString("endcmap\n")
	buf.WriteString("CMapName currentdict /CMap defineresource pop\n")
	buf.WriteString("end\nend\n")
	return buf.Bytes()
}

func utf16Hex(runes []rune) string {
	if len(runes) == 0 {
		return ""
	}
	encoded := utf16.Encode(runes)
	var b strings.Builder
	for _, u := range encoded {
		b.WriteString(fmt.Sprintf("%04X", u))
	}
	return b.String()
}

func encodeWidths(widths map[int]int) (first, last int, arr *raw.ArrayObj) {
	if len(widths) == 0 {
		return 0, 0, raw.NewArray()
	}
	first = math.MaxInt32
	last = -1
	for k := range widths {
		if k < first {
			first = k
		}
		if k > last {
			last = k
		}
	}
	arr = raw.NewArray()
	for i := first; i <= last; i++ {
		if w, ok := widths[i]; ok {
			arr.Append(raw.NumberInt(int64(w)))
		} else {
			arr.Append(raw.NumberInt(0))
		}
	}
	return first, last, arr
}

func encodeCIDWidths(widths map[int]int) *raw.ArrayObj {
	arr := raw.NewArray()
	if len(widths) == 0 {
		return arr
	}
	codes := make([]int, 0, len(widths))
	for c := range widths {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	start := codes[0]
	prev := codes[0]
	current := widths[codes[0]]
	for i := 1; i < len(codes); i++ {
		code := codes[i]
		w := widths[code]
		if w == current && code == prev+1 {
			prev = code
			continue
		}
		arr.Append(raw.NumberInt(int64(start)))
		arr.Append(raw.NumberInt(int64(prev)))
		arr.Append(raw.NumberInt(int64(current)))
		start = code
		prev = code
		current = w
	}
	arr.Append(raw.NumberInt(int64(start)))
	arr.Append(raw.NumberInt(int64(prev)))
	arr.Append(raw.NumberInt(int64(current)))
	return arr
}

func fontKey(base, encoding, subtype string, font *semantic.Font) string {
	h := sha256.New()
	h.Write([]byte(base))
	h.Write([]byte(encoding))
	h.Write([]byte(subtype))
	if font != nil {
		keys := make([]int, 0, len(font.Widths))
		for k := range font.Widths {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			h.Write([]byte(fmt.Sprintf("%d:%d", k, font.Widths[k])))
		}
		if font.DescendantFont != nil {
			keys := make([]int, 0, len(font.DescendantFont.W))
			for k := range font.DescendantFont.W {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				h.Write([]byte(fmt.Sprintf("%d:%d", k, font.DescendantFont.W[k])))
			}
			h.Write([]byte(font.DescendantFont.Subtype))
			h.Write([]byte(font.DescendantFont.BaseFont))
		}
		if font.CIDSystemInfo != nil {
			h.Write([]byte(font.CIDSystemInfo.Registry))
			h.Write([]byte(font.CIDSystemInfo.Ordering))
			h.Write([]byte(fmt.Sprint(font.CIDSystemInfo.Supplement)))
		}
		if font.Descriptor != nil {
			fd := font.Descriptor
			h.Write([]byte(fd.FontName))
			h.Write([]byte(fmt.Sprint(fd.Flags, fd.ItalicAngle, fd.Ascent, fd.Descent, fd.CapHeight, fd.StemV)))
			h.Write([]byte(fmt.Sprint(fd.FontBBox)))
			h.Write([]byte(fd.FontFileType))
			h.Write(fd.FontFile)
		}
		if font.DescendantFont != nil && font.DescendantFont.Descriptor != nil {
			fd := font.DescendantFont.Descriptor
			h.Write([]byte(fd.FontName))
			h.Write([]byte(fmt.Sprint(fd.Flags, fd.ItalicAngle, fd.Ascent, fd.Descent, fd.CapHeight, fd.StemV)))
			h.Write([]byte(fmt.Sprint(fd.FontBBox)))
			h.Write([]byte(fd.FontFileType))
			h.Write(fd.FontFile)
		}
		if len(font.ToUnicode) > 0 {
			cids := make([]int, 0, len(font.ToUnicode))
			for cid := range font.ToUnicode {
				cids = append(cids, cid)
			}
			sort.Ints(cids)
			for _, cid := range cids {
				h.Write([]byte(fmt.Sprintf("%d:", cid)))
				for _, r := range font.ToUnicode[cid] {
					h.Write([]byte(fmt.Sprintf("%d", r)))
				}
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func xoKey(name string, xo semantic.XObject) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(xo.Subtype))
	h.Write([]byte(fmt.Sprintf("%d-%d-%d", xo.Width, xo.Height, xo.BitsPerComponent)))
	h.Write([]byte(xo.ColorSpace.Name))
	h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f", xo.BBox.LLX, xo.BBox.LLY, xo.BBox.URX, xo.BBox.URY)))
	h.Write(xo.Data)
	if xo.Interpolate {
		h.Write([]byte{1})
	}
	if xo.SMask != nil {
		h.Write([]byte(xoKey(name+":SMask", *xo.SMask)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func patternKey(name string, p semantic.Pattern) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(fmt.Sprintf("%d-%d-%d", p.PatternType, p.PaintType, p.TilingType)))
	h.Write([]byte(fmt.Sprintf("%f-%f-%f-%f-%f-%f", p.BBox.LLX, p.BBox.LLY, p.BBox.URX, p.BBox.URY, p.XStep, p.YStep)))
	h.Write(p.Content)
	return hex.EncodeToString(h.Sum(nil))
}

func shadingKey(name string, s semantic.Shading) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(fmt.Sprintf("%d-%s", s.ShadingType, s.ColorSpace.Name)))
	for _, c := range s.Coords {
		h.Write([]byte(fmt.Sprintf("%f", c)))
	}
	for _, d := range s.Domain {
		h.Write([]byte(fmt.Sprintf("%f", d)))
	}
	h.Write(s.Function)
	return hex.EncodeToString(h.Sum(nil))
}

func fontDescriptor(cid *semantic.CIDFont, font *semantic.Font) *semantic.FontDescriptor {
	if cid != nil && cid.Descriptor != nil {
		return cid.Descriptor
	}
	if font != nil {
		return font.Descriptor
	}
	return nil
}

func serializeContentStream(cs semantic.ContentStream) []byte {
	if len(cs.RawBytes) > 0 {
		return cs.RawBytes
	}
	if len(cs.Operations) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, op := range cs.Operations {
		for i, operand := range op.Operands {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(serializeOperand(operand))
		}
		if len(op.Operands) > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(op.Operator)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func serializeOperand(op semantic.Operand) []byte {
	switch v := op.(type) {
	case semantic.NumberOperand:
		// %g keeps minimal form while preserving integer vs float readability.
		return []byte(fmt.Sprintf("%g", v.Value))
	case semantic.NameOperand:
		return []byte("/" + v.Value)
	case semantic.StringOperand:
		return escapeLiteralString(v.Value)
	case semantic.ArrayOperand:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, it := range v.Values {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(serializeOperand(it))
		}
		buf.WriteByte(']')
		return buf.Bytes()
	case semantic.DictOperand:
		var buf bytes.Buffer
		buf.WriteString("<<")
		keys := make([]string, 0, len(v.Values))
		for k := range v.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("/" + k + " ")
			buf.Write(serializeOperand(v.Values[k]))
		}
		buf.WriteString(">>")
		return buf.Bytes()
	default:
		return []byte("null")
	}
}

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

func buildStructureTree(tree *semantic.StructureTree, pageRefs []raw.ObjectRef, nextRef func() raw.ObjectRef, objects map[raw.ObjectRef]raw.Object) (*raw.ObjectRef, *raw.ObjectRef, map[int]map[int]raw.ObjectRef) {
	if tree == nil {
		return nil, nil, nil
	}
	parentTree := make(map[int]map[int]raw.ObjectRef)
	var buildElem func(elem *semantic.StructureElement, parent raw.ObjectRef) *raw.ObjectRef
	buildElem = func(elem *semantic.StructureElement, parent raw.ObjectRef) *raw.ObjectRef {
		if elem == nil {
			return nil
		}
		ref := nextRef()
		dict := raw.Dict()
		dict.Set(raw.NameLiteral("Type"), raw.NameLiteral("StructElem"))
		if elem.Type != "" {
			dict.Set(raw.NameLiteral("S"), raw.NameLiteral(elem.Type))
		}
		if elem.Title != "" {
			dict.Set(raw.NameLiteral("T"), raw.Str([]byte(elem.Title)))
		}
		if elem.PageIndex != nil {
			if pg := pageRefAt(pageRefs, *elem.PageIndex); pg != nil {
				dict.Set(raw.NameLiteral("Pg"), raw.Ref(pg.Num, pg.Gen))
			}
		}
		if parent.Num != 0 || parent.Gen != 0 {
			dict.Set(raw.NameLiteral("P"), raw.Ref(parent.Num, parent.Gen))
		}
		kArr := raw.NewArray()
		for _, kid := range elem.Kids {
			if kid.Element != nil {
				childRef := buildElem(kid.Element, ref)
				if childRef != nil {
					kArr.Append(raw.Ref(childRef.Num, childRef.Gen))
				}
				continue
			}
			if kid.MCID != nil {
				pageIdx := kid.PageIndex
				if pageIdx == nil {
					pageIdx = elem.PageIndex
				}
				if pageIdx != nil {
					if pg := pageRefAt(pageRefs, *pageIdx); pg != nil {
						mcr := raw.Dict()
						mcr.Set(raw.NameLiteral("Type"), raw.NameLiteral("MCR"))
						mcr.Set(raw.NameLiteral("Pg"), raw.Ref(pg.Num, pg.Gen))
						mcr.Set(raw.NameLiteral("MCID"), raw.NumberInt(int64(*kid.MCID)))
						kArr.Append(mcr)
						if _, ok := parentTree[*pageIdx]; !ok {
							parentTree[*pageIdx] = make(map[int]raw.ObjectRef)
						}
						parentTree[*pageIdx][*kid.MCID] = ref
					}
				}
			}
		}
		if kArr.Len() > 0 {
			dict.Set(raw.NameLiteral("K"), kArr)
		}
		objects[ref] = dict
		return &ref
	}
	kids := raw.NewArray()
	for _, kid := range tree.Kids {
		ref := buildElem(kid, raw.ObjectRef{})
		if ref != nil {
			kids.Append(raw.Ref(ref.Num, ref.Gen))
		}
	}
	if kids.Len() == 0 && len(tree.RoleMap) == 0 {
		return nil, nil, nil
	}
	rootRef := nextRef()
	rootDict := raw.Dict()
	rootDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("StructTreeRoot"))
	if kids.Len() > 0 {
		rootDict.Set(raw.NameLiteral("K"), kids)
	}
	if len(tree.RoleMap) > 0 {
		roleDict := raw.Dict()
		for k, v := range tree.RoleMap {
			roleDict.Set(raw.NameLiteral(k), raw.NameLiteral(v))
		}
		rootDict.Set(raw.NameLiteral("RoleMap"), roleDict)
	}
	var parentTreeRef *raw.ObjectRef
	if len(parentTree) > 0 {
		nums := raw.NewArray()
		indices := make([]int, 0, len(parentTree))
		for k := range parentTree {
			indices = append(indices, k)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			nums.Append(raw.NumberInt(int64(idx)))
			arr := raw.NewArray()
			maxMCID := -1
			for mcid := range parentTree[idx] {
				if mcid > maxMCID {
					maxMCID = mcid
				}
			}
			for i := 0; i <= maxMCID; i++ {
				if ref, ok := parentTree[idx][i]; ok {
					arr.Append(raw.Ref(ref.Num, ref.Gen))
				} else {
					arr.Append(raw.NullObj{})
				}
			}
			nums.Append(arr)
		}
		ptDict := raw.Dict()
		ptDict.Set(raw.NameLiteral("Nums"), nums)
		ref := nextRef()
		parentTreeRef = &ref
		objects[ref] = ptDict
		rootDict.Set(raw.NameLiteral("ParentTree"), raw.Ref(ref.Num, ref.Gen))
	}
	objects[rootRef] = rootDict
	return &rootRef, parentTreeRef, parentTree
}

func pageRefAt(pageRefs []raw.ObjectRef, idx int) *raw.ObjectRef {
	if idx < 0 || idx >= len(pageRefs) {
		return nil
	}
	return &pageRefs[idx]
}

func encryptObject(obj raw.Object, ref raw.ObjectRef, handler security.Handler, metadataRef *raw.ObjectRef, encryptMetadata bool) raw.Object {
	if handler == nil {
		return obj
	}
	if metadataRef != nil && !encryptMetadata && ref == *metadataRef {
		return obj
	}
	switch v := obj.(type) {
	case raw.StringObj:
		encrypted, err := handler.Encrypt(ref.Num, ref.Gen, v.Value(), security.DataClassString)
		if err != nil {
			return obj
		}
		return raw.Str(encrypted)
	case raw.HexStringObj:
		encrypted, err := handler.Encrypt(ref.Num, ref.Gen, v.Value(), security.DataClassString)
		if err != nil {
			return obj
		}
		return raw.HexStr(encrypted)
	case *raw.ArrayObj:
		arr := raw.NewArray()
		for _, item := range v.Items {
			arr.Append(encryptObject(item, ref, handler, metadataRef, encryptMetadata))
		}
		return arr
	case *raw.DictObj:
		d := raw.Dict()
		for k, val := range v.KV {
			d.Set(raw.NameLiteral(k), encryptObject(val, ref, handler, metadataRef, encryptMetadata))
		}
		return d
	case *raw.StreamObj:
		class := security.DataClassStream
		if metadataRef != nil && ref == *metadataRef {
			class = security.DataClassMetadataStream
		}
		data, err := handler.Encrypt(ref.Num, ref.Gen, v.Data, class)
		if err != nil {
			return obj
		}
		dictEncrypted := encryptObject(v.Dict, ref, handler, metadataRef, encryptMetadata)
		if dd, ok := dictEncrypted.(*raw.DictObj); ok {
			return raw.NewStream(dd, data)
		}
		return raw.NewStream(v.Dict, data)
	default:
		return obj
	}
}

func xyzDestValue(v *float64) raw.Object {
	if v == nil {
		return raw.NullObj{}
	}
	return raw.NumberFloat(*v)
}

func buildTrailer(size int, catalogRef raw.ObjectRef, infoRef *raw.ObjectRef, encryptRef *raw.ObjectRef, doc *semantic.Document, cfg Config, prev int64, ids [2][]byte) *raw.DictObj {
	trailer := raw.Dict()
	trailer.Set(raw.NameLiteral("Size"), raw.NumberInt(int64(size)))
	trailer.Set(raw.NameLiteral("Root"), raw.Ref(catalogRef.Num, catalogRef.Gen))
	if infoRef != nil {
		trailer.Set(raw.NameLiteral("Info"), raw.Ref(infoRef.Num, infoRef.Gen))
	}
	if encryptRef != nil {
		trailer.Set(raw.NameLiteral("Encrypt"), raw.Ref(encryptRef.Num, encryptRef.Gen))
	}
	idArr := raw.NewArray(
		raw.HexStr(ids[0]),
		raw.HexStr(ids[1]),
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

func pdfNameLiteral(value string) string {
	if value == "" {
		return ""
	}
	if strings.Contains(value, "#") {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			b.WriteByte(ch)
			continue
		}
		fmt.Fprintf(&b, "#%02X", ch)
	}
	return b.String()
}

type incrementalInfo struct {
	base        []byte
	prevOffset  int64
	startObjNum int
	prevMaxObj  int
	prevOffsets map[int]int64
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
	info.prevOffsets = scanObjectOffsets(info.base)
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

func scanObjectOffsets(data []byte) map[int]int64 {
	re := regexp.MustCompile(`(?m)^(\d+)\s+0\s+obj`)
	matches := re.FindAllStringSubmatchIndex(string(data), -1)
	offsets := make(map[int]int64, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		num, err := strconv.Atoi(string(data[m[2]:m[3]]))
		if err != nil {
			continue
		}
		if _, exists := offsets[num]; exists {
			continue
		}
		offsets[num] = int64(m[0])
	}
	return offsets
}
