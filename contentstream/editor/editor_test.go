package editor_test

import (
	"context"
	"os"
	"testing"

	"github.com/wudi/pdfkit/contentstream/editor"
	"github.com/wudi/pdfkit/ir/semantic"
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

func TestRepairStructTree(t *testing.T) {
	// Setup Page with MCIDs 1 and 2
	page := &semantic.Page{
		Contents: []semantic.ContentStream{
			{
				Operations: []semantic.Operation{
					// /Tag <</MCID 1>> BDC
					{
						Operator: "BDC",
						Operands: []semantic.Operand{
							semantic.NameOperand{Value: "P"},
							semantic.DictOperand{
								Values: map[string]semantic.Operand{
									"MCID": semantic.NumberOperand{Value: 1},
								},
							},
						},
					},
					// EMC
					{Operator: "EMC"},
					// /Tag <</MCID 2>> BDC
					{
						Operator: "BDC",
						Operands: []semantic.Operand{
							semantic.NameOperand{Value: "P"},
							semantic.DictOperand{
								Values: map[string]semantic.Operand{
									"MCID": semantic.NumberOperand{Value: 2},
								},
							},
						},
					},
					// EMC
					{Operator: "EMC"},
				},
			},
		},
	}

	// Setup StructTree referencing MCID 1, 2, and 3 (3 is missing from page)
	structTree := &semantic.StructureTree{
		K: []*semantic.StructureElement{
			{
				Type: "StructElem",
				S:    "P",
				Pg:   page,
				K: []semantic.StructureItem{
					{MCID: 1}, // Exists
				},
			},
			{
				Type: "StructElem",
				S:    "P",
				Pg:   page,
				K: []semantic.StructureItem{
					{MCID: 3}, // Missing! Should be removed
				},
			},
			{
				Type: "StructElem",
				S:    "Div",
				K: []semantic.StructureItem{
					{
						Element: &semantic.StructureElement{
							Type: "StructElem",
							S:    "Span",
							Pg:   page,
							K: []semantic.StructureItem{
								{MCID: 2}, // Exists
							},
						},
					},
					{
						Element: &semantic.StructureElement{
							Type: "StructElem",
							S:    "Span",
							Pg:   page,
							K: []semantic.StructureItem{
								{MCID: 4}, // Missing! Should be removed
							},
						},
					},
				},
			},
		},
	}

	ed := editor.NewEditor()
	ed.RepairStructTree(page, structTree)

	// Verify results
	if len(structTree.K) != 2 {
		t.Errorf("Expected 2 root elements, got %d", len(structTree.K))
	}

	// First element should be MCID 1
	if len(structTree.K[0].K) != 1 || structTree.K[0].K[0].MCID != 1 {
		t.Error("First element should contain MCID 1")
	}

	// Second element should be the Div containing MCID 2
	div := structTree.K[1]
	if div.S != "Div" {
		t.Errorf("Expected second element to be Div, got %s", div.S)
	}
	if len(div.K) != 1 {
		t.Errorf("Expected Div to have 1 child (Span with MCID 2), got %d", len(div.K))
	}
	span := div.K[0].Element
	if span == nil || span.S != "Span" {
		t.Error("Expected child to be Span")
	}
	if len(span.K) != 1 || span.K[0].MCID != 2 {
		t.Error("Expected Span to contain MCID 2")
	}
}
