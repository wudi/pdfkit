package parser

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestParseHintStreamHeader(t *testing.T) {
	// Construct a fake hint stream
	// Header is 11 fields.
	// 32, 32, 16, 32, 16, 16, 16, 16, 16, 16, 16
	// Total bits: 64 + 16 + 32 + 16*7 = 112 + 112 = 224 bits = 28 bytes.

	data := make([]byte, 100)
	// Set S (shared offset) to 50
	dict := raw.Dict()
	dict.Set(raw.NameObj{Val: "S"}, raw.NumberObj{I: 50, IsInt: true})

	// We leave data as zeros, so all values read as 0.
	// This is enough to test that it doesn't crash and reads the header.

	ht, err := ParseHintStream(data, dict, 1)
	if err != nil {
		t.Fatalf("ParseHintStream failed: %v", err)
	}

	if len(ht.PageOffsets) == 0 {
		t.Errorf("expected at least one page offset hint")
	}
}
