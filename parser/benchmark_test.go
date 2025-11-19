package parser

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func BenchmarkParseFCD14492(b *testing.B) {
	data, err := os.ReadFile("../testdata/fcd14492.pdf")
	if err != nil {
		b.Fatalf("failed to read test file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := NewDocumentParser(Config{})
		_, err := p.Parse(context.Background(), bytes.NewReader(data))
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
	}
}
