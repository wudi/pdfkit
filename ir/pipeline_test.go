package ir

import (
	"bytes"
	"context"
	"fmt"
	"testing"
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
