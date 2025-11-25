package pdfa_test

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/compliance/pdfa"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/writer"
)

func TestPDFALevelSharedType(t *testing.T) {
	levels := []pdfa.Level{pdfa.PDFA1B, pdfa.PDFA3B}
	for _, level := range levels {
		cfg := writer.Config{PDFALevel: level}
		e := pdfa.NewEnforcer()
		doc := &semantic.Document{}
		if err := e.Enforce(context.Background(), doc, cfg.PDFALevel); err != nil {
			t.Fatalf("enforce level %v: %v", level, err)
		}
		rep, err := e.Validate(context.Background(), doc, level)
		if err != nil {
			t.Fatalf("validate level %v: %v", level, err)
		}
		if rep.Standard != level.String() {
			t.Fatalf("expected level %v got %v", level, rep.Standard)
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
	rep, err := e.Validate(context.Background(), doc, pdfa.PDFA1B)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.Compliant {
		t.Fatal("expected non-compliant document")
	}

	// Enforce
	if err := e.Enforce(context.Background(), doc, pdfa.PDFA1B); err != nil {
		t.Fatalf("enforce: %v", err)
	}

	// Verify compliance
	rep, err = e.Validate(context.Background(), doc, pdfa.PDFA1B)
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

func TestPDFAEncryptedDocumentFailsValidation(t *testing.T) {
	e := pdfa.NewEnforcer()
	ctx := context.Background()

	doc := &semantic.Document{
		Encrypted: true,
		OutputIntents: []semantic.OutputIntent{{
			S: "GTS_PDFA1",
		}},
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
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

	rep, err := e.Validate(ctx, doc, pdfa.PDFA1B)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.Compliant {
		t.Fatal("expected encrypted document to be non-compliant")
	}
	if !hasViolation(rep, "ENC001") {
		t.Fatalf("expected encryption violation, got %+v", rep.Violations)
	}
	if len(rep.Violations) != 1 {
		t.Fatalf("expected only encryption violation, got %+v", rep.Violations)
	}
}
