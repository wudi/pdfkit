package writer

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"pdflib/builder"
	"pdflib/ir"
	"pdflib/ir/semantic"
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
