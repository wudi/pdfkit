package writer

import (
	"bytes"
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestSerializeInlineImage(t *testing.T) {
	// Create an inline image operand
	dict := semantic.DictOperand{Values: map[string]semantic.Operand{
		"W":   semantic.NumberOperand{Value: 10},
		"H":   semantic.NumberOperand{Value: 10},
		"BPC": semantic.NumberOperand{Value: 8},
		"CS":  semantic.NameOperand{Value: "RGB"},
	}}
	data := []byte{0, 1, 2, 3}
	iiOp := semantic.InlineImageOperand{
		Image: dict,
		Data:  data,
	}

	// Create operation
	op := semantic.Operation{
		Operator: "INLINE_IMAGE",
		Operands: []semantic.Operand{iiOp},
	}

	// Create content stream
	cs := semantic.ContentStream{
		Operations: []semantic.Operation{op},
	}

	// Serialize
	serialized := serializeContentStream(cs)

	// Check output
	// Expected: BI\n/BPC 8\n/CS /RGB\n/H 10\n/W 10\nID \x00\x01\x02\x03\nEI\n
	// Note: keys are sorted. BPC, CS, H, W.

	expectedStart := "BI\n"
	expectedEnd := "\nEI\n"

	if !bytes.HasPrefix(serialized, []byte(expectedStart)) {
		t.Errorf("expected prefix %q, got %q", expectedStart, serialized)
	}
	if !bytes.HasSuffix(serialized, []byte(expectedEnd)) {
		t.Errorf("expected suffix %q, got %q", expectedEnd, serialized)
	}

	// Check for ID and data
	idIndex := bytes.Index(serialized, []byte("ID "))
	if idIndex == -1 {
		t.Fatalf("ID marker not found")
	}

	// Check data
	dataStart := idIndex + 3
	dataEnd := len(serialized) - len(expectedEnd)
	actualData := serialized[dataStart:dataEnd]

	if !bytes.Equal(actualData, data) {
		t.Errorf("expected data %v, got %v", data, actualData)
	}

	// Check dict keys
	s := string(serialized)
	if !strings.Contains(s, "/W 10") {
		t.Errorf("missing /W 10")
	}
	if !strings.Contains(s, "/H 10") {
		t.Errorf("missing /H 10")
	}
	if !strings.Contains(s, "/BPC 8") {
		t.Errorf("missing /BPC 8")
	}
	if !strings.Contains(s, "/CS /RGB") {
		t.Errorf("missing /CS /RGB")
	}
}
