package ir

import (
	"bytes"
	"context"
	"testing"
)

func TestPipelineDecodeASCIIHexStream(t *testing.T) {
	pdf := "" +
		"%PDF-1.7\n" +
		"1 0 obj\n<< /Length 11 /Filter /ASCIIHexDecode >>\nstream\n48656c6c6f20776f726c64>\nendstream\nendobj\n" +
		"xref\n0 2\n0000000000 65535 f \n0000000010 00000 n \n" +
		"trailer << /Size 2 /Root 1 0 R >>\nstartxref\n99\n%%EOF\n"

	p := NewDefault()
	doc, err := p.Parse(context.Background(), bytes.NewReader([]byte(pdf)))
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
