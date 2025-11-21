package pdfvt_test

import (
	"context"
	"testing"

	"pdflib/compliance/pdfvt"
	"pdflib/ir/semantic"
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
