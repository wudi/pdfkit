package pdfua_test

import (
	"context"
	"testing"

	"pdflib/compliance/pdfua"
	"pdflib/ir/semantic"
)

func TestValidate(t *testing.T) {
	e := pdfua.NewEnforcer()

	// Case 1: Empty document (should fail)
	doc := &semantic.Document{}
	rep, err := e.Validate(context.Background(), doc)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.Compliant {
		t.Fatal("expected non-compliant document")
	}

	// Check for specific violations
	hasTagError := false
	hasTitleError := false
	hasLangError := false

	for _, v := range rep.Violations {
		switch v.Code {
		case "UA001":
			hasTagError = true
		case "UA002":
			hasTitleError = true
		case "UA003":
			hasLangError = true
		}
	}

	if !hasTagError {
		t.Error("expected UA001 (StructTree missing)")
	}
	if !hasTitleError {
		t.Error("expected UA002 (Title missing)")
	}
	if !hasLangError {
		t.Error("expected UA003 (Language missing)")
	}

	// Case 2: Compliant document
	doc = &semantic.Document{
		StructTree: &semantic.StructureTree{},
		Info:       &semantic.DocumentInfo{Title: "Test Document"},
		Lang:       "en-US",
	}

	rep, err = e.Validate(context.Background(), doc)
	if err != nil {
		t.Fatalf("validate compliant: %v", err)
	}
	if !rep.Compliant {
		for _, v := range rep.Violations {
			t.Logf("Violation: %s", v.Description)
		}
		t.Fatal("expected compliant document")
	}
}
