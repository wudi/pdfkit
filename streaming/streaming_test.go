package streaming

import (
	"bytes"
	"testing"
	"time"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/writer"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestStreamingDocumentStartEnd(t *testing.T) {
	pdfBytes := buildSamplePDF(t)

	// Stream the document.
	p := NewParser()
	ds, err := p.Stream(staticCtx{}, bytes.NewReader(pdfBytes), StreamConfig{BufferSize: 2})
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
	if _, ok := events[len(events)-1].(DocumentEndEvent); !ok {
		t.Fatalf("last event not DocumentEnd: %#v", events[len(events)-1])
	}
	pageStarts := 0
	contentOps := 0
	pageEnds := 0
	resourceRefs := 0
	for _, ev := range events {
		switch e := ev.(type) {
		case PageStartEvent:
			pageStarts++
			if e.Index < 0 || e.MediaBox.URX == 0 {
				t.Fatalf("invalid page index: %d", e.Index)
			}
		case ContentOperationEvent:
			contentOps++
		case ResourceRefEvent:
			resourceRefs++
		case PageEndEvent:
			pageEnds++
		}
	}
	if pageStarts != 2 || pageEnds != 2 {
		t.Fatalf("unexpected page counts: starts=%d ends=%d", pageStarts, pageEnds)
	}
	if contentOps == 0 {
		t.Fatalf("expected content operations, got %d", contentOps)
	}
	if resourceRefs == 0 {
		t.Fatalf("expected resource refs, got %d", resourceRefs)
	}
}

func TestStreamingBackpressure(t *testing.T) {
	pdfBytes := buildSamplePDF(t)
	p := NewParser()
	ds, err := p.Stream(staticCtx{}, bytes.NewReader(pdfBytes), StreamConfig{BufferSize: 1})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer ds.Close()

	events := []Event{}
	for ev := range ds.Events() {
		events = append(events, ev)
		time.Sleep(2 * time.Millisecond) // simulate slow consumer/backpressure
	}
	select {
	case err := <-ds.Errors():
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}
	if len(events) == 0 {
		t.Fatalf("no events observed")
	}
}

func buildSamplePDF(t *testing.T) []byte {
	t.Helper()
	b := builder.NewBuilder()
	img := &semantic.Image{
		Width:            1,
		Height:           1,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
		BitsPerComponent: 8,
		Data:             []byte{0xFF},
	}
	b.NewPage(50, 50).
		DrawText("hi there", 5, 5, builder.TextOptions{FontSize: 10}).
		DrawImage(img, 10, 10, 1, 1, builder.ImageOptions{}).
		Finish()
	b.NewPage(30, 30).DrawText("p2", 2, 2, builder.TextOptions{FontSize: 8}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := writer.NewWriter()
	if err := w.Write(staticCtx{}, doc, &buf, writer.Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	return buf.Bytes()
}
