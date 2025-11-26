package builder

import (
	"strings"
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
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

func TestGenerateButtonAppearanceWithCaption(t *testing.T) {
	form := &semantic.AcroForm{
		DefaultResources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"Helv": {
					BaseFont: "Helvetica",
					Widths:   map[int]int{'O': 600, 'K': 600}, // Mock widths
				},
			},
		},
	}
	generator := NewAppearanceGenerator(form)

	field := &semantic.ButtonFormField{
		BaseFormField: semantic.BaseFormField{
			Rect: semantic.Rectangle{LLX: 100, LLY: 100, URX: 200, URY: 150},
			Name: "SubmitBtn",
		},
		Caption: "OK",
	}

	xobj, err := generator.Generate(field)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if xobj == nil {
		t.Fatal("Expected XObject, got nil")
	}

	content := string(xobj.Data)
	if !strings.Contains(content, "(OK) Tj") {
		t.Errorf("Expected caption 'OK', got %s", content)
	}

	// Test fallback to Name
	field.Caption = ""
	xobj, err = generator.Generate(field)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	content = string(xobj.Data)
	if !strings.Contains(content, "(SubmitBtn) Tj") {
		t.Errorf("Expected fallback to Name 'SubmitBtn', got %s", content)
	}
}

func TestMeasureText(t *testing.T) {
	font := &semantic.Font{
		BaseFont: "Helvetica",
		Widths:   map[int]int{int('A'): 1000}, // 1000/1000 = 1.0 em
	}
	form := &semantic.AcroForm{
		DefaultResources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"Helv": font,
			},
		},
	}
	generator := NewAppearanceGenerator(form)

	// Test with known width
	width := generator.measureText("AAA", "Helv", 10.0)
	// 3 chars * 1.0 em * 10.0 size = 30.0
	if width != 30.0 {
		t.Errorf("Expected width 30.0, got %f", width)
	}

	// Test with missing font (fallback)
	width = generator.measureText("AAA", "Unknown", 10.0)
	// 3 chars * 0.5 em * 10.0 size = 15.0
	if width != 15.0 {
		t.Errorf("Expected fallback width 15.0, got %f", width)
	}
}
