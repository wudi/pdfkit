package editor_test

import (
	"context"
	"os"
	"testing"

	"pdflib/contentstream/editor"
	"pdflib/ir/semantic"
)

func TestReplaceText(t *testing.T) {
	// Load font
	fontData, err := os.ReadFile("../../testdata/Rubik-Regular.ttf")
	if err != nil {
		t.Skip("skipping test: font file not found")
	}

	font := &semantic.Font{
		Subtype:  "TrueType",
		BaseFont: "Rubik-Regular",
		Descriptor: &semantic.FontDescriptor{
			FontFile: fontData,
		},
		Widths: make(map[int]int), // Mock widths
	}

	// Create page with Tj operation
	page := &semantic.Page{
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"F1": font,
			},
		},
		Contents: []semantic.ContentStream{
			{
				Operations: []semantic.Operation{
					{Operator: "Tf", Operands: []semantic.Operand{semantic.NameOperand{Value: "F1"}, semantic.NumberOperand{Value: 12}}},
					{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte("Hello")}}},
				},
			},
		},
	}

	ed := editor.NewEditor()
	err = ed.ReplaceText(context.Background(), page, "Hello", "World")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}

	// Verify operation changed to TJ
	ops := page.Contents[0].Operations
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}
	if ops[1].Operator != "TJ" {
		t.Errorf("expected TJ operator, got %s", ops[1].Operator)
	}

	// Check operands of TJ
	tjOp := ops[1]
	if len(tjOp.Operands) != 1 {
		t.Fatalf("expected 1 operand for TJ, got %d", len(tjOp.Operands))
	}
	arr, ok := tjOp.Operands[0].(semantic.ArrayOperand)
	if !ok {
		t.Fatal("expected ArrayOperand for TJ")
	}
	
	// "World" has 5 characters.
	if len(arr.Values) < 5 {
		t.Errorf("expected at least 5 elements in TJ array, got %d", len(arr.Values))
	}
}
