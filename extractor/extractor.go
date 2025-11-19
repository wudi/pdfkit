package extractor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
	"pdflib/scanner"
)

// Extractor exposes helper routines for pulling structured data out of a decoded PDF.
type Extractor struct {
	dec        *decoded.DecodedDocument
	raw        *raw.Document
	catalog    *raw.DictObj
	pages      []*raw.DictObj
	pageLabels map[int]string
	inflated   bool
}

// New creates an extractor backed by the provided decoded document.
func New(dec *decoded.DecodedDocument) (*Extractor, error) {
	if dec == nil {
		return nil, errors.New("decoded document is required")
	}
	if dec.Raw == nil {
		return nil, errors.New("decoded document missing raw representation")
	}
	catalog := rootCatalog(dec.Raw)
	if catalog == nil {
		return nil, errors.New("pdf catalog not found in trailer")
	}
	pages := collectPages(dec.Raw)
	e := &Extractor{
		dec:     dec,
		raw:     dec.Raw,
		catalog: catalog,
		pages:   pages,
	}
	e.pageLabels = collectPageLabels(dec.Raw, catalog, len(pages))
	e.inflateObjectStreams()
	return e, nil
}

// Metadata holds high-level document metadata and flags.
type Metadata struct {
	Version           string
	Info              raw.DocumentMetadata
	Lang              string
	Marked            bool
	Permissions       raw.Permissions
	Encrypted         bool
	MetadataEncrypted bool
	PageCount         int
	XMP               []byte
}

// ExtractMetadata aggregates document metadata, language, tagging flags, and XMP payloads.
func (e *Extractor) ExtractMetadata() Metadata {
	meta := Metadata{
		Version:           e.raw.Version,
		Info:              e.raw.Metadata,
		Permissions:       e.dec.Perms,
		Encrypted:         e.dec.Encrypted,
		MetadataEncrypted: e.dec.MetadataEncrypted,
		PageCount:         len(e.pages),
	}
	if lang, ok := stringFromDict(e.catalog, "Lang"); ok {
		meta.Lang = lang
	}
	if markInfo := derefDict(e.raw, valueFromDict(e.catalog, "MarkInfo")); markInfo != nil {
		if marked, ok := boolFromDict(markInfo, "Marked"); ok {
			meta.Marked = marked
		}
	}
	if data, _ := e.streamBytes(valueFromDict(e.catalog, "Metadata")); len(data) > 0 {
		meta.XMP = data
	}
	return meta
}

// PageLabels returns the computed label for every page index.
func (e *Extractor) PageLabels() map[int]string {
	out := make(map[int]string, len(e.pageLabels))
	for k, v := range e.pageLabels {
		out[k] = v
	}
	return out
}

func (e *Extractor) streamBytes(obj raw.Object) ([]byte, raw.Dictionary) {
	if data, dict, ok := streamData(e.dec, obj); ok {
		copyData := make([]byte, len(data))
		copy(copyData, data)
		return copyData, dict
	}
	return nil, nil
}

func (e *Extractor) inflateObjectStreams() {
	if e.inflated || e.dec == nil || e.raw == nil {
		return
	}
	newObjects := make(map[raw.ObjectRef]raw.Object)
	for ref, obj := range e.raw.Objects {
		stream, ok := obj.(raw.Stream)
		if !ok {
			continue
		}
		typ, _ := nameFromDict(stream.Dictionary(), "Type")
		if typ != "ObjStm" {
			continue
		}
		objects, err := e.decodeObjectStream(raw.RefObj{R: ref})
		if err != nil {
			continue
		}
		for num, embedded := range objects {
			key := raw.ObjectRef{Num: num, Gen: 0}
			if _, exists := e.raw.Objects[key]; !exists {
				newObjects[key] = embedded
			}
		}
	}
	for ref, obj := range newObjects {
		e.raw.Objects[ref] = obj
	}
	e.inflated = true
}

func (e *Extractor) decodeObjectStream(ref raw.RefObj) (map[int]raw.Object, error) {
	data, dict := e.streamBytes(ref)
	if len(data) == 0 || dict == nil {
		return nil, fmt.Errorf("object stream missing data")
	}
	count, ok := intFromObject(valueFromDict(dict, "N"))
	if !ok || count <= 0 {
		return nil, fmt.Errorf("invalid object stream count")
	}
	first, ok := intFromObject(valueFromDict(dict, "First"))
	if !ok || first < 0 || first > len(data) {
		return nil, fmt.Errorf("invalid object stream First")
	}
	header := data[:first]
	body := data[first:]
	type entry struct {
		num int
		off int
	}
	entries := make([]entry, 0, count)
	reader := bufio.NewReader(bytes.NewReader(header))
	for i := 0; i < count; i++ {
		var objNum, offset int
		if _, err := fmt.Fscan(reader, &objNum, &offset); err != nil {
			return nil, fmt.Errorf("parse objstm header: %w", err)
		}
		entries = append(entries, entry{num: objNum, off: offset})
	}
	objects := make(map[int]raw.Object, len(entries))
	for idx, ent := range entries {
		start := ent.off
		if start < 0 || start > len(body) {
			continue
		}
		end := len(body)
		if idx+1 < len(entries) {
			next := entries[idx+1].off
			if next >= 0 && next <= len(body) {
				end = next
			}
		}
		segment := bytes.TrimSpace(body[start:end])
		if len(segment) == 0 {
			continue
		}
		obj, err := parseObjectBytes(segment)
		if err != nil {
			return nil, fmt.Errorf("parse objstm object %d: %w", ent.num, err)
		}
		objects[ent.num] = obj
	}
	return objects, nil
}

func parseObjectBytes(data []byte) (raw.Object, error) {
	reader := bytes.NewReader(data)
	sc := scanner.New(reader, scanner.Config{})
	tr := &tokenReader{s: sc}
	return parseObject(tr)
}

type tokenReader struct {
	s   scanner.Scanner
	buf []scanner.Token
}

func (r *tokenReader) next() (scanner.Token, error) {
	if l := len(r.buf); l > 0 {
		t := r.buf[l-1]
		r.buf = r.buf[:l-1]
		return t, nil
	}
	return r.s.Next()
}

func (r *tokenReader) unread(tok scanner.Token) {
	r.buf = append(r.buf, tok)
}

func parseObject(tr *tokenReader) (raw.Object, error) {
	tok, err := tr.next()
	if err != nil {
		return nil, err
	}
	switch tok.Type {
	case scanner.TokenName:
		if v, ok := tok.Value.(string); ok {
			return raw.NameObj{Val: v}, nil
		}
	case scanner.TokenNumber:
		if i, ok := toInt(tok.Value); ok {
			return raw.NumberObj{I: i, IsInt: true}, nil
		}
		if f, ok := tok.Value.(float64); ok {
			return raw.NumberObj{F: f, IsInt: false}, nil
		}
	case scanner.TokenBoolean:
		if v, ok := tok.Value.(bool); ok {
			return raw.BoolObj{V: v}, nil
		}
	case scanner.TokenNull:
		return raw.NullObj{}, nil
	case scanner.TokenString:
		if b, ok := tok.Value.([]byte); ok {
			return raw.StringObj{Bytes: b}, nil
		}
	case scanner.TokenArray:
		return parseArray(tr)
	case scanner.TokenDict:
		return parseDict(tr)
	case scanner.TokenRef:
		if v, ok := tok.Value.(struct{ Num, Gen int }); ok {
			return raw.RefObj{R: raw.ObjectRef{Num: v.Num, Gen: v.Gen}}, nil
		}
	case scanner.TokenKeyword:
		if tok.Value == "stream" {
			return nil, fmt.Errorf("unexpected stream keyword inside object stream")
		}
	}
	return nil, fmt.Errorf("unexpected token: %v", tok.Type)
}

func parseArray(tr *tokenReader) (raw.Object, error) {
	arr := &raw.ArrayObj{}
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Value == "]" {
			break
		}
		tr.unread(tok)
		item, err := parseObject(tr)
		if err != nil {
			return nil, err
		}
		arr.Append(item)
	}
	return arr, nil
}

func parseDict(tr *tokenReader) (raw.Object, error) {
	d := raw.Dict()
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Value == ">>" {
			break
		}
		if tok.Type != scanner.TokenName {
			return nil, fmt.Errorf("expected name in dict, got %v", tok.Type)
		}
		key, _ := tok.Value.(string)
		val, err := parseObject(tr)
		if err != nil {
			return nil, err
		}
		d.Set(raw.NameObj{Val: key}, val)
	}
	return d, nil
}

func toInt(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func rootCatalog(doc *raw.Document) *raw.DictObj {
	if doc == nil || doc.Trailer == nil {
		return nil
	}
	rootObj, ok := doc.Trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return nil
	}
	return derefDict(doc, rootObj)
}

func collectPages(doc *raw.Document) []*raw.DictObj {
	if doc == nil || doc.Trailer == nil {
		return nil
	}
	rootObj, ok := doc.Trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return nil
	}
	var pages []*raw.DictObj
	walkPages(doc, rootObj, func(page *raw.DictObj) {
		pages = append(pages, page)
	})
	return pages
}

func walkPages(doc *raw.Document, obj raw.Object, visit func(*raw.DictObj)) {
	dict := derefDict(doc, obj)
	if dict == nil {
		return
	}
	if typ, ok := nameFromDict(dict, "Type"); ok {
		switch typ {
		case "Catalog":
			walkPages(doc, valueFromDict(dict, "Pages"), visit)
			return
		case "Pages":
			if kids := derefArray(doc, valueFromDict(dict, "Kids")); kids != nil {
				for _, kid := range kids.Items {
					walkPages(doc, kid, visit)
				}
			}
			return
		case "Page":
			visit(dict)
			return
		}
	}
	if _, ok := dict.Get(raw.NameLiteral("Contents")); ok {
		visit(dict)
	}
}

func collectPageLabels(doc *raw.Document, catalog *raw.DictObj, pageCount int) map[int]string {
	labels := make(map[int]string)
	if catalog == nil {
		return labels
	}
	pageLabels := derefDict(doc, valueFromDict(catalog, "PageLabels"))
	if pageLabels == nil {
		return labels
	}
	nums := derefArray(doc, valueFromDict(pageLabels, "Nums"))
	if nums == nil {
		return labels
	}
	for i := 0; i+1 < len(nums.Items); i += 2 {
		idx, ok := intFromObject(nums.Items[i])
		if !ok {
			continue
		}
		entry := derefDict(doc, nums.Items[i+1])
		if entry == nil {
			continue
		}
		prefix, _ := stringFromDict(entry, "P")
		start := 1
		if st, ok := intFromObject(valueFromDict(entry, "St")); ok {
			start = st
		}
		for p := idx; p < pageCount; p++ {
			if _, exists := labels[p]; exists {
				continue
			}
			labels[p] = fmt.Sprintf("%s%d", prefix, start+(p-idx))
		}
	}
	return labels
}

func valueFromDict(dict raw.Dictionary, key string) raw.Object {
	if dict == nil {
		return nil
	}
	val, _ := dict.Get(raw.NameLiteral(key))
	return val
}

func nameFromDict(dict raw.Dictionary, key string) (string, bool) {
	val, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		return "", false
	}
	return nameFromObject(val)
}

func stringFromDict(dict raw.Dictionary, key string) (string, bool) {
	val, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		return "", false
	}
	return stringFromObject(val)
}

func boolFromDict(dict raw.Dictionary, key string) (bool, bool) {
	val, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		return false, false
	}
	if b, ok := val.(raw.Boolean); ok {
		return b.Value(), true
	}
	if obj, ok := val.(raw.BoolObj); ok {
		return obj.Value(), true
	}
	return false, false
}

func nameFromObject(obj raw.Object) (string, bool) {
	switch v := obj.(type) {
	case raw.Name:
		return v.Value(), true
	}
	return "", false
}

func stringFromObject(obj raw.Object) (string, bool) {
	switch v := obj.(type) {
	case raw.String:
		return string(v.Value()), true
	}
	return "", false
}

func intFromObject(obj raw.Object) (int, bool) {
	switch v := obj.(type) {
	case raw.Number:
		return int(v.Int()), true
	}
	return 0, false
}

func floatFromObject(obj raw.Object) (float64, bool) {
	switch v := obj.(type) {
	case raw.Number:
		return v.Float(), true
	}
	return 0, false
}

func deref(doc *raw.Document, obj raw.Object) raw.Object {
	if ref, ok := obj.(raw.RefObj); ok {
		if doc != nil {
			if resolved, ok := doc.Objects[ref.Ref()]; ok {
				return resolved
			}
		}
	}
	return obj
}

func derefDict(doc *raw.Document, obj raw.Object) *raw.DictObj {
	if obj == nil {
		return nil
	}
	resolved := deref(doc, obj)
	if dict, ok := resolved.(*raw.DictObj); ok {
		return dict
	}
	if stream, ok := resolved.(raw.Stream); ok {
		if dict, ok := stream.Dictionary().(*raw.DictObj); ok {
			return dict
		}
	}
	return nil
}

func derefArray(doc *raw.Document, obj raw.Object) *raw.ArrayObj {
	if obj == nil {
		return nil
	}
	resolved := deref(doc, obj)
	if arr, ok := resolved.(*raw.ArrayObj); ok {
		return arr
	}
	return nil
}

func streamData(dec *decoded.DecodedDocument, obj raw.Object) ([]byte, raw.Dictionary, bool) {
	if dec == nil {
		return nil, nil, false
	}
	switch v := obj.(type) {
	case raw.RefObj:
		if stream, ok := dec.Streams[v.Ref()]; ok {
			data := stream.Data()
			out := make([]byte, len(data))
			copy(out, data)
			return out, stream.Dictionary(), true
		}
	case raw.Stream:
		data := v.RawData()
		out := make([]byte, len(data))
		copy(out, data)
		return out, v.Dictionary(), true
	}
	return nil, nil, false
}
