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
	b.NewPage(30, 30).DrawText("p2", 2, 2, builder.TextOptions{FontSize: 8}).Finish()
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
	if len(events) == 0 {
		t.Fatalf("no events emitted")
	}
	if _, ok := events[0].(DocumentStartEvent); !ok {
		t.Fatalf("first event not DocumentStart: %#v", events[0])
	}
	end, ok := events[len(events)-1].(DocumentEndEvent)
	_ = end
	if !ok {
		t.Fatalf("last event not DocumentEnd: %#v", events[len(events)-1])
	}
	pageStarts := 0
	contentEvents := 0
	pageEnds := 0
	for _, ev := range events {
		switch e := ev.(type) {
		case PageStartEvent:
			pageStarts++
			if e.Index < 0 {
				t.Fatalf("invalid page index: %d", e.Index)
			}
		case ContentStreamEvent:
			contentEvents++
			if len(e.Operations) == 0 {
				t.Fatalf("empty operations in content event")
			}
			if e.Resources == nil || len(e.Resources.Fonts) == 0 {
				t.Fatalf("resources missing fonts")
			}
		case PageEndEvent:
			pageEnds++
		}
	}
	if pageStarts != 2 || pageEnds != 2 || contentEvents != 2 {
		t.Fatalf("unexpected page/content counts: starts=%d ends=%d contents=%d", pageStarts, pageEnds, contentEvents)
	}
}
