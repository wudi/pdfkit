package pdfa_test

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/compliance/pdfa"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestPDFALevels(t *testing.T) {
	e := pdfa.NewEnforcer()
	ctx := context.Background()

	// Helper to create a basic compliant doc (embedded font, output intent)
	createBaseDoc := func() *semantic.Document {
		return &semantic.Document{
			OutputIntents: []semantic.OutputIntent{{
				S:                 "GTS_PDFA1",
				DestOutputProfile: []byte("mock profile"), // Mock, validation will fail on profile parsing but we ignore that for feature checks
			}},
			Pages: []*semantic.Page{
				{
					Resources: &semantic.Resources{
						Fonts: map[string]*semantic.Font{
							"F1": {
								Subtype:  "TrueType",
								BaseFont: "Arial",
								Descriptor: &semantic.FontDescriptor{
									FontFile: []byte("mock font data"),
								},
							},
						},
					},
				},
			},
		}
	}

	t.Run("Transparency", func(t *testing.T) {
		doc := createBaseDoc()
		alpha := 0.5
		doc.Pages[0].Resources.ExtGStates = map[string]semantic.ExtGState{
			"GS1": {FillAlpha: &alpha},
		}

		// A-1b: Forbidden
		rep, _ := e.Validate(ctx, doc, pdfa.PDFA1B)
		if !hasViolation(rep, "TRN001") {
			t.Error("Expected transparency violation in PDF/A-1b")
		}

		// A-2b: Allowed
		rep, _ = e.Validate(ctx, doc, pdfa.PDFA2B)
		if hasViolation(rep, "TRN001") {
			t.Error("Unexpected transparency violation in PDF/A-2b")
		}
	})

	t.Run("Layers", func(t *testing.T) {
		doc := createBaseDoc()
		doc.Pages[0].Resources.Properties = map[string]semantic.PropertyList{
			"OC1": &semantic.OptionalContentGroup{Name: "Layer1"},
		}

		// A-1b: Forbidden
		rep, _ := e.Validate(ctx, doc, pdfa.PDFA1B)
		if !hasViolation(rep, "LYR001") {
			t.Error("Expected layer violation in PDF/A-1b")
		}

		// A-2b: Allowed
		rep, _ = e.Validate(ctx, doc, pdfa.PDFA2B)
		if hasViolation(rep, "LYR001") {
			t.Error("Unexpected layer violation in PDF/A-2b")
		}
	})

	t.Run("Attachments", func(t *testing.T) {
		doc := createBaseDoc()
		doc.EmbeddedFiles = []semantic.EmbeddedFile{
			{Name: "test.txt", Data: []byte("hello")},
		}

		// A-1b: Forbidden
		rep, _ := e.Validate(ctx, doc, pdfa.PDFA1B)
		if !hasViolation(rep, "ATT001") {
			t.Error("Expected attachment violation in PDF/A-1b")
		}

		// A-3b: Allowed
		rep, _ = e.Validate(ctx, doc, pdfa.PDFA3B)
		if hasViolation(rep, "ATT001") {
			t.Error("Unexpected attachment violation in PDF/A-3b")
		}
	})

	t.Run("Annotations", func(t *testing.T) {
		doc := createBaseDoc()
		doc.Pages[0].Annotations = []semantic.Annotation{
			&semantic.MovieAnnotation{BaseAnnotation: semantic.BaseAnnotation{Subtype: "Movie"}},
		}

		// A-1b: Forbidden
		rep, _ := e.Validate(ctx, doc, pdfa.PDFA1B)
		if !hasViolation(rep, "ACT001") {
			t.Error("Expected Movie annotation violation in PDF/A-1b")
		}

		// A-2b: Forbidden (Movie is deprecated/forbidden in A-2 as well, use Screen)
		rep, _ = e.Validate(ctx, doc, pdfa.PDFA2B)
		if !hasViolation(rep, "ACT001") {
			t.Error("Expected Movie annotation violation in PDF/A-2b")
		}
	})
}

func hasViolation(rep *compliance.Report, code string) bool {
	for _, v := range rep.Violations {
		if v.Code == code {
			return true
		}
	}
	return false
}
