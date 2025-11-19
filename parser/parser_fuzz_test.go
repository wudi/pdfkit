package parser

import (
	"bytes"
	"context"
	"testing"

	"pdflib/recovery"
)

func FuzzDocumentParser(f *testing.F) {
	f.Add([]byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n..."))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		cfg := Config{
			Recovery: recovery.NewStrictStrategy(),
		}
		p := NewDocumentParser(cfg)
		_, _ = p.Parse(context.Background(), r)
	})
}
