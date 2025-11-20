package pdfa_test

import (
	"testing"

	"pdflib/ir/semantic"
	"pdflib/pdfa"
	"pdflib/writer"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestPDFALevelSharedType(t *testing.T) {
	levels := []pdfa.Level{pdfa.PDFA1B, pdfa.PDFA3B}
	for _, level := range levels {
		cfg := writer.Config{PDFALevel: level}
		e := pdfa.NewEnforcer()
		doc := &semantic.Document{}
		if err := e.Enforce(staticCtx{}, doc, cfg.PDFALevel); err != nil {
			t.Fatalf("enforce level %v: %v", level, err)
		}
		rep, err := e.Validate(staticCtx{}, doc, level)
		if err != nil {
			t.Fatalf("validate level %v: %v", level, err)
		}
		if rep.Level != level {
			t.Fatalf("expected level %v got %v", level, rep.Level)
		}
	}
}

func TestEnforce(t *testing.T) {
	e := pdfa.NewEnforcer()

	// Create a non-compliant document
	doc := &semantic.Document{
		Encrypted: true,
		Pages: []*semantic.Page{
			{
				Annotations: []semantic.Annotation{
					&semantic.GenericAnnotation{BaseAnnotation: semantic.BaseAnnotation{Subtype: "Movie"}}, // Forbidden
					&semantic.GenericAnnotation{BaseAnnotation: semantic.BaseAnnotation{Subtype: "Text"}},  // Allowed
				},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": {
							Subtype:  "TrueType",
							BaseFont: "Arial",
							Descriptor: &semantic.FontDescriptor{
								FontFile: []byte("mock font data"), // Embedded
							},
						},
					},
				},
			},
		},
	}

	// Verify it fails validation initially
	rep, err := e.Validate(staticCtx{}, doc, pdfa.PDFA1B)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.Compliant {
		t.Fatal("expected non-compliant document")
	}

	// Enforce
	if err := e.Enforce(staticCtx{}, doc, pdfa.PDFA1B); err != nil {
		t.Fatalf("enforce: %v", err)
	}

	// Verify compliance
	rep, err = e.Validate(staticCtx{}, doc, pdfa.PDFA1B)
	if err != nil {
		t.Fatalf("validate after enforce: %v", err)
	}
	if !rep.Compliant {
		for _, v := range rep.Violations {
			t.Logf("Violation: %s (%s)", v.Description, v.Location)
		}
		t.Fatal("expected compliant document after enforcement")
	}

	// Check specific changes
	if doc.Encrypted {
		t.Error("Encryption should be removed")
	}
	if len(doc.OutputIntents) == 0 {
		t.Error("OutputIntent should be added")
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Errorf("Expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Base().Subtype != "Text" {
		t.Errorf("Expected Text annotation, got %s", doc.Pages[0].Annotations[0].Base().Subtype)
	}
}
