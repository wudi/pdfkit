package writer

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"pdflib/builder"
	"pdflib/ir"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/xref"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestWriterRoundTripPipeline(t *testing.T) {
	// Build a simple document with one page and text.
	b := builder.NewBuilder()
	b.NewPage(200, 200).DrawText("Hello", 10, 20, builder.TextOptions{FontSize: 12}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse using default pipeline and ensure stream contains our text.
	p := ir.NewDefault()
	out, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse roundtrip: %v", err)
	}
	if out.Decoded() == nil || len(out.Decoded().Streams) == 0 {
		t.Fatalf("expected decoded streams")
	}
	found := false
	for _, s := range out.Decoded().Streams {
		if bytes.Contains(s.Data(), []byte("Hello")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("written content not found in decoded streams")
	}
}

func TestWriter_InfoMetadataIDDeterministic(t *testing.T) {
	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Sample Title"})
	b.SetMetadata([]byte("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\"></x:xmpmeta>"))
	b.NewPage(100, 100).DrawText("Hi", 5, 5, builder.TextOptions{FontSize: 10}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	cfg := Config{Version: PDFVersion("1.6"), Deterministic: true}
	w := (&WriterBuilder{}).Build()

	var first bytes.Buffer
	if err := w.Write(staticCtx{}, doc, &first, cfg); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := first.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-1.6")) {
		t.Fatalf("expected PDF 1.6 header, got %q", data[:8])
	}
	if !bytes.Contains(data, []byte("/Title (Sample Title)")) {
		t.Fatalf("missing Title in output")
	}
	if !bytes.Contains(data, []byte("/Metadata")) {
		t.Fatalf("missing Metadata reference in catalog")
	}
	reID := regexp.MustCompile(`/ID\s*\[\(\s*([0-9a-f]+)\s*\)\s*\(\s*([0-9a-f]+)\s*\)\]`)
	matches := reID.FindSubmatch(data)
	if len(matches) != 3 || !bytes.Equal(matches[1], matches[2]) {
		t.Fatalf("expected matching ID pair in trailer")
	}

	var second bytes.Buffer
	if err := w.Write(staticCtx{}, doc, &second, cfg); err != nil {
		t.Fatalf("write pdf second: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("deterministic write expected identical output")
	}
}

func TestWriter_PageGeometryAndResources(t *testing.T) {
	page1 := &semantic.Page{
		MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 400},
		CropBox:  semantic.Rectangle{LLX: 10, LLY: 10, URX: 290, URY: 390},
		Rotate:   90,
		UserUnit: 2.0,
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"FBody": {BaseFont: "Helvetica"},
				"FMono": {BaseFont: "Courier"},
			},
		},
		Contents: []semantic.ContentStream{
			{RawBytes: []byte("BT /FBody 12 Tf 0 0 Td (Hi) Tj ET")},
		},
	}
	page2 := &semantic.Page{
		MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 400},
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"FBody": {BaseFont: "Helvetica"},
			},
		},
		Contents: []semantic.ContentStream{
			{RawBytes: []byte("BT /FBody 10 Tf 10 10 Td (Two) Tj ET")},
		},
	}
	doc := &semantic.Document{Pages: []*semantic.Page{page1, page2}}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	strData := string(data)

	if !strings.Contains(strData, "/Rotate 90") {
		t.Fatalf("expected Rotate entry")
	}
	if !regexp.MustCompile(`/CropBox\s*\[\s*10\.?0*\s+10\.?0*\s+290\.?0*\s+390\.?0*\s*\]`).Match(data) {
		t.Fatalf("expected CropBox in page dictionary")
	}
	if !strings.Contains(strData, "/UserUnit 2") {
		t.Fatalf("expected UserUnit entry")
	}
	if strings.Count(strData, "/BaseFont /Helvetica") != 1 {
		t.Fatalf("expected shared Helvetica font object")
	}
	if strings.Count(strData, "/BaseFont /Courier") != 1 {
		t.Fatalf("expected single Courier font object")
	}
	fontsRegex := regexp.MustCompile(`/Font\s*<<[^>]*?/FBody\s+\d+\s+0\s+R[^>]*?/FMono\s+\d+\s+0\s+R[^>]*?>>`)
	if !fontsRegex.Match(data) {
		t.Fatalf("expected page font resource entries")
	}
}

func TestWriter_XRefStream(t *testing.T) {
	b := builder.NewBuilder()
	b.NewPage(100, 100).DrawText("XRef", 5, 5, builder.TextOptions{FontSize: 9}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	cfg := Config{XRefStreams: true, Deterministic: true}
	if err := w.Write(staticCtx{}, doc, &buf, cfg); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	if !bytes.Contains(data, []byte("/Type /XRef")) {
		t.Fatalf("expected xref stream type")
	}
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	m := re.FindSubmatch(data)
	if len(m) != 2 {
		t.Fatalf("startxref not found")
	}
	startOff, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parse startxref: %v", err)
	}
	if startOff <= 0 || startOff >= len(data) {
		t.Fatalf("startxref out of bounds: %d", startOff)
	}

	res := xref.NewResolver(xref.ResolverConfig{})
	table, err := res.Resolve(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("resolve xref stream: %v", err)
	}
	if table.Type() != "xref-stream" {
		t.Fatalf("expected xref-stream table, got %s", table.Type())
	}
	if off, _, ok := table.Lookup(1); !ok || off == 0 {
		t.Fatalf("catalog entry missing in xref stream")
	}
}

func TestWriter_IncrementalAppend(t *testing.T) {
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()

	// First revision
	orig := builder.NewBuilder()
	orig.NewPage(50, 50).DrawText("v1", 1, 1, builder.TextOptions{}).Finish()
	doc1, _ := orig.Build()
	if err := w.Write(staticCtx{}, doc1, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write first: %v", err)
	}
	baseData := buf.Bytes()
	baseMax := maxObjNum(baseData)
	prevOffset := startXRef(baseData)
	firstLen := buf.Len()

	// Second revision (incremental) with new page
	rev := builder.NewBuilder()
	rev.NewPage(50, 50).DrawText("v1", 1, 1, builder.TextOptions{}).Finish()
	rev.NewPage(60, 60).DrawText("v2", 2, 2, builder.TextOptions{}).Finish()
	doc2, _ := rev.Build()
	cfg := Config{Incremental: true, Deterministic: true, XRefStreams: true}
	if err := w.Write(staticCtx{}, doc2, &buf, cfg); err != nil {
		t.Fatalf("write incremental: %v", err)
	}
	data := buf.Bytes()
	if len(data) <= firstLen {
		t.Fatalf("incremental write did not append data")
	}
	if !bytes.Contains(data[firstLen:], []byte("/Prev")) {
		t.Fatalf("expected Prev in incremental trailer")
	}
	if !bytes.Contains(data[firstLen:], []byte(strconv.FormatInt(prevOffset, 10))) {
		t.Fatalf("Prev does not reference prior xref offset")
	}
	if maxObjNum(data) <= baseMax {
		t.Fatalf("expected object numbers to increase for incremental update")
	}
	p := ir.NewDefault()
	if _, err := p.Parse(context.Background(), bytes.NewReader(data)); err != nil {
		t.Fatalf("parse incremental: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("raw parse incremental: %v", err)
	}
	if len(rawDoc.Objects) <= baseMax {
		t.Fatalf("expected raw object count to grow")
	}
}

func maxObjNum(data []byte) int {
	re := regexp.MustCompile(`\s(\d+)\s+0\s+obj`)
	matches := re.FindAllSubmatch(data, -1)
	max := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		n, _ := strconv.Atoi(string(m[1]))
		if n > max {
			max = n
		}
	}
	return max
}

func startXRef(data []byte) int64 {
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	m := re.FindSubmatch(data)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return v
}
