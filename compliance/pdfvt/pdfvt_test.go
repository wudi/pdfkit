package pdfvt_test

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/compliance/pdfvt"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestPDFVTEnforcement(t *testing.T) {
	e := pdfvt.NewEnforcer()
	ctx := context.Background()

	doc := &semantic.Document{
		Encrypted: true,
	}

	// Validate (should fail)
	rep, _ := e.Validate(ctx, doc)
	if rep.Compliant {
		t.Fatal("Expected non-compliant document")
	}

	// Enforce
	if err := e.Enforce(ctx, doc, pdfvt.PDFVT1); err != nil {
		t.Fatalf("Enforce failed: %v", err)
	}

	// Check Enforcement
	if doc.Encrypted {
		t.Error("Encryption should be removed")
	}
	if len(doc.OutputIntents) == 0 {
		t.Error("OutputIntent should be added")
	}
	if doc.DPartRoot == nil {
		t.Error("DPartRoot should be created")
	}

	// Validate again
	rep, _ = e.Validate(ctx, doc)
	if !rep.Compliant {
		for _, v := range rep.Violations {
			t.Logf("Violation: %s", v.Description)
		}
		t.Fatal("Expected compliant document")
	}
}

func TestPDFVTEncryptedDocumentFailsValidation(t *testing.T) {
	e := pdfvt.NewEnforcer()
	ctx := context.Background()

	doc := &semantic.Document{
		Encrypted: true,
		OutputIntents: []semantic.OutputIntent{{
			S: "GTS_PDFVT",
		}},
		DPartRoot: &semantic.DPartRoot{},
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
