package builder

import (
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestGenerateTextAppearance(t *testing.T) {
	form := &semantic.AcroForm{
		DefaultResources: &semantic.Resources{},
	}
	generator := NewAppearanceGenerator(form)

	field := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Rect:              semantic.Rectangle{LLX: 100, LLY: 100, URX: 200, URY: 120},
			DefaultAppearance: "/Helv 12 Tf 0 g",
		},
		Value: "Hello World",
	}

	xobj, err := generator.Generate(field)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if xobj == nil {
		t.Fatal("Expected XObject, got nil")
	}

	if xobj.Subtype != "Form" {
		t.Errorf("Expected Subtype Form, got %s", xobj.Subtype)
	}

	content := string(xobj.Data)
	if !strings.Contains(content, "/Helv 12 Tf") {
		t.Errorf("Expected font setting, got %s", content)
	}
	if !strings.Contains(content, "(Hello World) Tj") {
		t.Errorf("Expected text drawing, got %s", content)
	}
}

func TestGenerateCheckboxAppearance(t *testing.T) {
	form := &semantic.AcroForm{
		DefaultResources: &semantic.Resources{},
	}
	generator := NewAppearanceGenerator(form)

	field := &semantic.ButtonFormField{
		BaseFormField: semantic.BaseFormField{
			Rect: semantic.Rectangle{LLX: 100, LLY: 100, URX: 120, URY: 120},
		},
		IsCheck: true,
		Checked: true,
	}

	xobj, err := generator.Generate(field)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if xobj == nil {
		t.Fatal("Expected XObject, got nil")
	}

	content := string(xobj.Data)
	// Check for cross drawing (lines)
	if !strings.Contains(content, "m") || !strings.Contains(content, "l") || !strings.Contains(content, "S") {
		t.Errorf("Expected drawing commands, got %s", content)
	}
}
