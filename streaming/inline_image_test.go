package streaming

import (
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
)

func TestParseInlineImage(t *testing.T) {
	// BI /W 10 /H 10 /BPC 8 /CS /RGB ID ...data... EI
	data := []byte("q\nBI\n/W 10\n/H 10\n/BPC 8\n/CS /RGB\nID \x00\x01\x02\x03\nEI\nQ")
	ops := parseOperations(data)

	if len(ops) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(ops))
	}

	if ops[0].Operator != "q" {
		t.Errorf("expected first op 'q', got '%s'", ops[0].Operator)
	}

	iiOp := ops[1]
	if iiOp.Operator != "INLINE_IMAGE" {
		t.Errorf("expected second op 'INLINE_IMAGE', got '%s'", iiOp.Operator)
	}

	if len(iiOp.Operands) != 1 {
		t.Fatalf("expected 1 operand for INLINE_IMAGE, got %d", len(iiOp.Operands))
	}

	ii, ok := iiOp.Operands[0].(semantic.InlineImageOperand)
	if !ok {
		t.Fatalf("operand is not InlineImageOperand")
	}

	if len(ii.Data) != 4 { // \x00\x01\x02\x03
		t.Errorf("expected 4 bytes of data, got %d", len(ii.Data))
	}
	if string(ii.Data) != "\x00\x01\x02\x03" {
		t.Errorf("data mismatch")
	}

	if len(ii.Image.Values) != 4 {
		t.Errorf("expected 4 dict entries, got %d", len(ii.Image.Values))
	}

	if ops[2].Operator != "Q" {
		t.Errorf("expected third op 'Q', got '%s'", ops[2].Operator)
	}
}
