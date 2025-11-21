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
	hasMarkedError := false
	hasTagError := false
	hasTitleError := false
	hasLangError := false

	for _, v := range rep.Violations {
		switch v.Code {
		case "UA001":
			hasMarkedError = true
		case "UA002":
			hasTagError = true
		case "UA003":
			hasTitleError = true
		case "UA004":
			hasLangError = true
		}
	}

	if !hasMarkedError {
		t.Error("expected UA001 (Marked missing)")
	}
	if !hasTagError {
		t.Error("expected UA002 (StructTree missing)")
	}
	if !hasTitleError {
		t.Error("expected UA003 (Title missing)")
	}
	if !hasLangError {
		t.Error("expected UA004 (Language missing)")
	}

	// Case 2: Compliant document
	doc = &semantic.Document{
		Marked:     true,
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

func TestEnforce(t *testing.T) {
	e := pdfua.NewEnforcer()
	doc := &semantic.Document{}

	if err := e.Enforce(context.Background(), doc, pdfua.PDFUA1); err != nil {
		t.Fatalf("enforce: %v", err)
	}

	if !doc.Marked {
		t.Error("Marked flag not set")
	}
	if doc.StructTree == nil {
		t.Error("StructTree not created")
	}
	if doc.Info == nil || doc.Info.Title == "" {
		t.Error("Title not set")
	}
	if doc.Lang == "" {
		t.Error("Lang not set")
	}
}

func TestAltText(t *testing.T) {
	e := pdfua.NewEnforcer()
	doc := &semantic.Document{
		Marked: true,
		Info:   &semantic.DocumentInfo{Title: "Test"},
		Lang:   "en",
		StructTree: &semantic.StructureTree{
			K: []*semantic.StructureElement{
				{
					S:   "Figure",
					Alt: "", // Missing Alt
				},
			},
		},
	}

	rep, _ := e.Validate(context.Background(), doc)
	if rep.Compliant {
		t.Fatal("Expected violation for missing Alt text")
	}

	found := false
	for _, v := range rep.Violations {
		if v.Code == "UA006" {
			found = true
		}
	}
	if !found {
		t.Error("Expected UA006 violation")
	}
}
