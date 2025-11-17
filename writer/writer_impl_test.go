package writer

import (
	"bytes"
	"context"
	"testing"

	"pdflib/builder"
	"pdflib/ir"
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
