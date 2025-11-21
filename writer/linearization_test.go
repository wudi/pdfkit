package writer

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/xref"
)

func TestLinearization(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
			},
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
			},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	cfg := Config{
		Linearize: true,
	}

	err := w.Write(context.Background(), doc, &buf, cfg)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("Output is empty")
	}

	output := buf.String()
	if !strings.Contains(output, "/Linearized 1") {
		t.Error("Output does not contain /Linearized 1")
	}

	// Check for Hint Stream dict entry
	if !strings.Contains(output, "/S 0") {
		t.Error("Output does not contain Hint Stream /S entry")
	}

	// Check for multiple xrefs (First Page XRef and Main XRef)
	if strings.Count(output, "xref") < 2 {
		t.Errorf("Expected at least 2 xref tables, found %d", strings.Count(output, "xref"))
	}
}

func TestLinearization_Structure(t *testing.T) {
	ctx := context.Background()
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 500, URY: 700}},
			{MediaBox: semantic.Rectangle{URX: 500, URY: 700}},
			{MediaBox: semantic.Rectangle{URX: 500, URY: 700}},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	cfg := Config{Linearize: true}
	if err := w.Write(ctx, doc, &buf, cfg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	pdf := buf.Bytes()

	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(ctx, bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !resolver.Linearized() {
		t.Fatal("Resolver did not detect linearized PDF")
	}
	if len(resolver.Incremental()) == 0 {
		t.Fatal("Expected incremental section for first-page xref")
	}

	rawDoc, err := parser.NewDocumentParser(parser.Config{}).Parse(ctx, bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	var (
		linRef     raw.ObjectRef
		linDict    *raw.DictObj
		hintRef    raw.ObjectRef
		hintStream *raw.StreamObj
	)
	for ref, obj := range rawDoc.Objects {
		if dict, ok := obj.(*raw.DictObj); ok {
			if val, ok := dict.Get(raw.NameLiteral("Linearized")); ok {
				if num, ok := val.(raw.NumberObj); ok && num.Int() > 0 {
					linRef = ref
					linDict = dict
				}
			}
		}
		if stream, ok := obj.(*raw.StreamObj); ok {
			if _, ok := stream.Dict.Get(raw.NameLiteral("S")); ok {
				hintRef = ref
				hintStream = stream
			}
		}
	}
	if linDict == nil {
		t.Fatal("Linearization dictionary not found")
	}
	if hintStream == nil {
		t.Fatal("Hint stream not found")
	}

	linOffset, _, ok := table.Lookup(linRef.Num)
	if !ok {
		t.Fatalf("Missing xref entry for linearization dict %d", linRef.Num)
	}
	serializedLin, err := (&impl{}).SerializeObject(linRef, linDict)
	if err != nil {
		t.Fatalf("Serialize linearization dict: %v", err)
	}
	firstXRefOffset := linOffset + int64(len(serializedLin))

	pdfLen := int64(len(pdf))
	assertNumberEquals(t, linDict, "L", pdfLen)

	firstPageNum, err := firstPageObjectNumber(rawDoc)
	if err != nil {
		t.Fatalf("Determine first page: %v", err)
	}
	assertNumberEquals(t, linDict, "O", int64(firstPageNum))

	hArrObj, ok := linDict.Get(raw.NameLiteral("H"))
	if !ok {
		t.Fatal("Linearization dictionary missing H entry")
	}
	hArr, ok := hArrObj.(*raw.ArrayObj)
	if !ok || hArr.Len() != 2 {
		t.Fatalf("H entry malformed: %#v", hArrObj)
	}
	hintOffset := mustNumber(t, hArr.Items[0])
	hintLength := mustNumber(t, hArr.Items[1])
	assertNumberEquals(t, linDict, "E", hintOffset)

	actualHintOffset, _, ok := table.Lookup(hintRef.Num)
	if !ok {
		t.Fatalf("Missing xref entry for hint stream %d", hintRef.Num)
	}
	if hintOffset != actualHintOffset {
		t.Fatalf("Hint offset mismatch: dict=%d actual=%d", hintOffset, actualHintOffset)
	}
	serializedHint, err := (&impl{}).SerializeObject(hintRef, hintStream)
	if err != nil {
		t.Fatalf("Serialize hint stream failed: %v", err)
	}
	if hintLength != int64(len(serializedHint)) {
		t.Fatalf("Hint length mismatch: dict=%d actual=%d", hintLength, len(serializedHint))
	}

	prevVal, ok := trailerNumber(rawDoc, "Prev")
	if !ok {
		t.Fatal("Trailer missing Prev entry")
	}
	if prevVal != firstXRefOffset {
		t.Fatalf("Prev mismatch: trailer=%d computed=%d", prevVal, firstXRefOffset)
	}

	mainXRefOffset, ok := parseStartXRef(pdf)
	if !ok {
		t.Fatal("Unable to parse startxref")
	}
	assertNumberEquals(t, linDict, "T", mainXRefOffset)
}

func firstPageObjectNumber(doc *raw.Document) (int, error) {
	if doc == nil || doc.Trailer == nil {
		return 0, fmt.Errorf("document trailer missing")
	}
	rootVal, ok := doc.Trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return 0, fmt.Errorf("root missing")
	}
	rootRef, ok := rootVal.(raw.RefObj)
	if !ok {
		return 0, fmt.Errorf("root not ref")
	}
	catalog, ok := doc.Objects[rootRef.Ref()]
	if !ok {
		return 0, fmt.Errorf("catalog object missing")
	}
	catalogDict, ok := catalog.(*raw.DictObj)
	if !ok {
		return 0, fmt.Errorf("catalog not dict")
	}
	pagesVal, ok := catalogDict.Get(raw.NameLiteral("Pages"))
	if !ok {
		return 0, fmt.Errorf("pages missing")
	}
	pagesRef, ok := pagesVal.(raw.RefObj)
	if !ok {
		return 0, fmt.Errorf("pages not ref")
	}
	pagesObj, ok := doc.Objects[pagesRef.Ref()]
	if !ok {
		return 0, fmt.Errorf("pages object missing")
	}
	pagesDict, ok := pagesObj.(*raw.DictObj)
	if !ok {
		return 0, fmt.Errorf("pages not dict")
	}
	kidsVal, ok := pagesDict.Get(raw.NameLiteral("Kids"))
	if !ok {
		return 0, fmt.Errorf("kids missing")
	}
	kidsArr, ok := kidsVal.(*raw.ArrayObj)
	if !ok || kidsArr.Len() == 0 {
		return 0, fmt.Errorf("kids empty")
	}
	firstRef, ok := kidsArr.Items[0].(raw.RefObj)
	if !ok {
		return 0, fmt.Errorf("first kid not ref")
	}
	return firstRef.Ref().Num, nil
}

func assertNumberEquals(t *testing.T, dict *raw.DictObj, key string, expected int64) {
	t.Helper()
	val, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		t.Fatalf("Linearization dictionary missing %s", key)
	}
	num := mustNumber(t, val)
	if num != expected {
		t.Fatalf("%s mismatch: got=%d expected=%d", key, num, expected)
	}
}

func trailerNumber(doc *raw.Document, key string) (int64, bool) {
	if doc == nil || doc.Trailer == nil {
		return 0, false
	}
	val, ok := doc.Trailer.Get(raw.NameLiteral(key))
	if !ok {
		return 0, false
	}
	return numberValue(val)
}

func mustNumber(t *testing.T, obj raw.Object) int64 {
	t.Helper()
	if val, ok := numberValue(obj); ok {
		return val
	}
	t.Fatalf("object is not numeric: %#v", obj)
	return 0
}

func numberValue(obj raw.Object) (int64, bool) {
	switch v := obj.(type) {
	case raw.NumberObj:
		return v.Int(), true
	default:
		return 0, false
	}
}

func parseStartXRef(pdf []byte) (int64, bool) {
	idx := bytes.LastIndex(pdf, []byte("startxref"))
	if idx < 0 {
		return 0, false
	}
	segment := pdf[idx+len("startxref"):]
	segment = bytes.TrimLeft(segment, "\r\n ")
	end := bytes.IndexAny(segment, "\r\n ")
	if end < 0 {
		return 0, false
	}
	val, err := strconv.ParseInt(string(segment[:end]), 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}
