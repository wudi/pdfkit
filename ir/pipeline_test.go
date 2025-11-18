package ir

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"pdflib/observability"
)

func TestPipelineDecodeASCIIHexStream(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")
	hexData := "48656c6c6f20776f726c64"
	objOff := buf.Len()
	fmt.Fprintf(buf, "1 0 obj\n<< /Length %d /Filter /ASCIIHexDecode >>\nstream\n%s>\nendstream\nendobj\n", len(hexData), hexData)
	xrefOff := buf.Len()
	fmt.Fprintf(buf, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \n", objOff)
	buf.WriteString("trailer << /Size 2 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOff)
	pdf := buf.Bytes()

	p := NewDefault()
	doc, err := p.Parse(context.Background(), bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("pipeline parse failed: %v", err)
	}

	if doc.Decoded() == nil {
		t.Fatalf("decoded document missing")
	}
	stream := doc.Decoded().Streams
	if len(stream) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(stream))
	}

	for _, s := range stream {
		if string(s.Data()) != "Hello world" {
			t.Fatalf("unexpected decoded data: %q", s.Data())
		}
	}
}

type recordingTracer struct{ spans []*recordingSpan }

type recordingSpan struct {
	name     string
	tags     map[string]interface{}
	err      error
	finished bool
}

func (t *recordingTracer) StartSpan(ctx context.Context, name string) (context.Context, observability.Span) {
	span := &recordingSpan{name: name, tags: make(map[string]interface{})}
	t.spans = append(t.spans, span)
	return ctx, span
}

func (s *recordingSpan) SetTag(key string, value interface{}) {
	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	s.tags[key] = value
}

func (s *recordingSpan) SetError(err error) { s.err = err }

func (s *recordingSpan) Finish() { s.finished = true }

func (t *recordingTracer) spanByName(name string) *recordingSpan {
	for i := len(t.spans) - 1; i >= 0; i-- {
		if t.spans[i].name == name {
			return t.spans[i]
		}
	}
	return nil
}

func TestPipelineTracingSuccess(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")
	objOff := buf.Len()
	fmt.Fprintf(buf, "1 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n")
	xrefOff := buf.Len()
	fmt.Fprintf(buf, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \n", objOff)
	buf.WriteString("trailer << /Size 2 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOff)

	tracer := &recordingTracer{}
	p := NewDefault().WithTracer(tracer)
	if _, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("pipeline parse failed: %v", err)
	}
	if span := tracer.spanByName("pipeline.parse"); span == nil || !span.finished {
		t.Fatalf("pipeline span missing or not finished: %+v", span)
	} else if span.err != nil {
		t.Fatalf("unexpected error recorded: %v", span.err)
	} else if pages, ok := span.tags["pages"]; !ok || pages != 0 {
		t.Fatalf("pages tag missing or wrong: %+v", span.tags)
	}
	if rawSpan := tracer.spanByName("pipeline.raw_parse"); rawSpan == nil || rawSpan.err != nil || !rawSpan.finished {
		t.Fatalf("raw parse span missing or errored: %+v", rawSpan)
	}
	if decodeSpan := tracer.spanByName("pipeline.decode"); decodeSpan == nil || decodeSpan.err != nil || !decodeSpan.finished {
		t.Fatalf("decode span missing or errored: %+v", decodeSpan)
	}
	if buildSpan := tracer.spanByName("pipeline.semantic_build"); buildSpan == nil || buildSpan.err != nil || !buildSpan.finished {
		t.Fatalf("semantic build span missing or errored: %+v", buildSpan)
	}
}

func TestPipelineTracingError(t *testing.T) {
	tracer := &recordingTracer{}
	p := NewDefault().WithTracer(tracer)
	_, err := p.Parse(context.Background(), bytes.NewReader([]byte("not a pdf")))
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if span := tracer.spanByName("pipeline.raw_parse"); span == nil || span.err == nil {
		t.Fatalf("raw parse span missing error: %+v", span)
	}
	if span := tracer.spanByName("pipeline.parse"); span == nil || span.err == nil {
		t.Fatalf("pipeline span missing error: %+v", span)
	}
}
