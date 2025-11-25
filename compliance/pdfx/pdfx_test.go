package pdfx_test

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/compliance/pdfx"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestPDFXEnforcement(t *testing.T) {
	e := pdfx.NewEnforcer()
	ctx := context.Background()

	doc := &semantic.Document{
		Encrypted: true,
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					ColorSpaces: map[string]semantic.ColorSpace{
						"CS1": &semantic.DeviceColorSpace{Name: "DeviceRGB"}, // Invalid for X-1a
					},
				},
			},
		},
	}

	// Validate X-1a (should fail)
	rep, _ := e.Validate(ctx, doc)
	if rep.Compliant {
		t.Fatal("Expected non-compliant document (Encryption, RGB, No OutputIntent, No Trapped)")
	}

	// Enforce X-1a
	if err := e.Enforce(ctx, doc, pdfx.PDFX1a); err != nil {
		t.Fatalf("Enforce failed: %v", err)
	}

	// Check Enforcement results
	if doc.Encrypted {
		t.Error("Encryption should be removed")
	}
	if len(doc.OutputIntents) == 0 {
		t.Error("OutputIntent should be added")
	}
	if doc.Info == nil || doc.Info.Trapped == "" {
		t.Error("Trapped key should be set")
	}
	if doc.Pages[0].TrimBox == (semantic.Rectangle{}) {
		t.Error("TrimBox should be set (defaulted to MediaBox)")
	}

	// Validate again (might still fail on RGB, as Enforce doesn't convert colors yet)
	rep, _ = e.Validate(ctx, doc)
	hasRGBViolation := false
	for _, v := range rep.Violations {
		if v.Code == "CLR001" {
			hasRGBViolation = true
		}
	}
	if !hasRGBViolation {
		t.Error("Expected RGB violation to persist (Enforce doesn't convert colors)")
	}
}

func TestPDFXEncryptedDocumentFailsValidation(t *testing.T) {
	e := pdfx.NewEnforcer()
	ctx := context.Background()

	doc := &semantic.Document{
		Encrypted: true,
		OutputIntents: []semantic.OutputIntent{{
			S: "GTS_PDFX",
		}},
		Info: &semantic.DocumentInfo{Trapped: "True"},
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				TrimBox:  semantic.Rectangle{URX: 10, URY: 10},
			},
		},
	}

	rep, err := e.Validate(ctx, doc)
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

func hasViolation(rep *compliance.Report, code string) bool {
	for _, v := range rep.Violations {
		if v.Code == code {
			return true
		}
	}
	return false
}
