package builder

import (
	"testing"

	"pdflib/ir/semantic"
)

func TestBuilder_AddFormField(t *testing.T) {
	b := NewBuilder()
	field := semantic.FormField{
		Name:  "TestField",
		Value: "Initial Value",
		Type:  "Tx",
		Rect:  semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 30},
	}

	b.NewPage(200, 200).
		AddFormField(field).
		Finish()

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	if doc.AcroForm == nil {
		t.Fatal("AcroForm is nil")
	}
	if len(doc.AcroForm.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(doc.AcroForm.Fields))
	}
	if doc.AcroForm.Fields[0].Name != "TestField" {
		t.Errorf("expected field name 'TestField', got %q", doc.AcroForm.Fields[0].Name)
	}

	if len(doc.Pages) != 1 {
		t.Fatal("expected 1 page")
	}
	// Note: WidgetAnnotation is not added to Page.Annotations by builder,
	// but handled by writer via AcroForm.Fields.PageIndex.
	// So we check if the field has the correct PageIndex.
	if doc.AcroForm.Fields[0].PageIndex != 0 {
		t.Errorf("expected field PageIndex 0, got %d", doc.AcroForm.Fields[0].PageIndex)
	}
}

func TestBuilder_FormFill(t *testing.T) {
	// 1. Create a document with a field
	b := NewBuilder()
	field := semantic.FormField{
		Name:  "TextField",
		Value: "",
		Type:  "Tx",
		Rect:  semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 30},
	}
	cbField := semantic.FormField{
		Name:  "Checkbox",
		Value: "Off",
		Type:  "Btn",
		Rect:  semantic.Rectangle{LLX: 10, LLY: 50, URX: 30, URY: 70},
	}
	chField := semantic.FormField{
		Name:  "Choice",
		Value: "Option1",
		Type:  "Ch",
		Rect:  semantic.Rectangle{LLX: 10, LLY: 80, URX: 100, URY: 100},
	}

	b.NewPage(200, 200).
		AddFormField(field).
		AddFormField(cbField).
		AddFormField(chField).
		Finish()

	// 2. Use FormBuilder to fill it
	b.Form().
		SetText("TextField", "Filled Value").
		SetCheckbox("Checkbox", true).
		SetChoice("Choice", "Option2")

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	// 3. Verify values
	if len(doc.AcroForm.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(doc.AcroForm.Fields))
	}

	var tf, cf, chf *semantic.FormField
	for i := range doc.AcroForm.Fields {
		f := &doc.AcroForm.Fields[i]
		if f.Name == "TextField" {
			tf = f
		} else if f.Name == "Checkbox" {
			cf = f
		} else if f.Name == "Choice" {
			chf = f
		}
	}

	if tf == nil {
		t.Fatal("TextField not found")
	}
	if tf.Value != "Filled Value" {
		t.Errorf("expected TextField value 'Filled Value', got %q", tf.Value)
	}

	if cf == nil {
		t.Fatal("Checkbox not found")
	}
	if cf.Value != "Yes" { // Assuming SetCheckbox sets "Yes" for true
		t.Errorf("expected Checkbox value 'Yes', got %q", cf.Value)
	}

	if chf == nil {
		t.Fatal("Choice field not found")
	}
	if chf.Value != "Option2" {
		t.Errorf("expected Choice value 'Option2', got %q", chf.Value)
	}
}
