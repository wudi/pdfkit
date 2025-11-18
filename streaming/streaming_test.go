package streaming

import (
	"bytes"
	"testing"

	"pdflib/builder"
	"pdflib/writer"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestStreamingDocumentStartEnd(t *testing.T) {
	// Build a small PDF.
	b := builder.NewBuilder()
	b.NewPage(50, 50).DrawText("hi", 5, 5, builder.TextOptions{FontSize: 10}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, writer.Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Stream the document.
	p := NewParser()
	ds, err := p.Stream(staticCtx{}, bytes.NewReader(buf.Bytes()), StreamConfig{BufferSize: 2})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer ds.Close()

	events := []Event{}
	for ev := range ds.Events() {
		events = append(events, ev)
	}
	select {
	case err := <-ds.Errors():
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}
	if len(events) != 2 {
		t.Fatalf("expected two events, got %d", len(events))
	}
	start, ok := events[0].(DocumentStartEvent)
	if !ok {
		t.Fatalf("first event not DocumentStart: %#v", events[0])
	}
	if start.Version == "" {
		t.Fatalf("missing version in start event")
	}
	if events[1].Type() != EventDocumentEnd {
		t.Fatalf("last event not DocumentEnd")
	}
}
