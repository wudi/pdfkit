package compliance_test

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/compliance/pdfua"
	"github.com/wudi/pdfkit/compliance/pdfx"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestPDFUA_StructureValidation(t *testing.T) {
	enforcer := pdfua.NewEnforcer()

	// Invalid Table
	doc := &semantic.Document{
		Marked: true,
		Info:   &semantic.DocumentInfo{Title: "Test"},
		Lang:   "en",
		StructTree: &semantic.StructureTree{
			K: []*semantic.StructureElement{
				{
					Type: "StructElem",
					S:    "Table",
					K: []semantic.StructureItem{
						{Element: &semantic.StructureElement{Type: "StructElem", S: "P"}}, // Invalid child
					},
				},
			},
		},
	}

	report, err := enforcer.Validate(context.Background(), doc)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if report.Compliant {
		t.Error("Expected non-compliant document (Invalid Table)")
	}

	found := false
	for _, v := range report.Violations {
		if v.Code == "UA007" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected UA007 violation")
	}
}

func TestPDFX_ColorSpaceValidation(t *testing.T) {
	enforcer := pdfx.NewEnforcer()

	// Invalid ColorSpace (RGB)
	doc := &semantic.Document{
		Info: &semantic.DocumentInfo{Trapped: "False"},
		OutputIntents: []semantic.OutputIntent{
			{S: "GTS_PDFX"},
		},
		Pages: []*semantic.Page{
			{
				TrimBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					ColorSpaces: map[string]semantic.ColorSpace{
						"CS1": &semantic.DeviceColorSpace{Name: "DeviceRGB"},
					},
				},
			},
		},
	}

	report, err := enforcer.Validate(context.Background(), doc)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if report.Compliant {
		t.Error("Expected non-compliant document (RGB ColorSpace)")
	}

	found := false
	for _, v := range report.Violations {
		if v.Code == "CLR001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected CLR001 violation")
	}
}
