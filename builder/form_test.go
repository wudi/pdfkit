package builder

import (
	"testing"

	"pdflib/ir/semantic"
)

func TestBuilder_AddFormField(t *testing.T) {
	b := NewBuilder()
	field := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "TestField",
			Rect: semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 30},
		},
		Value: "Initial Value",
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
	if doc.AcroForm.Fields[0].FieldName() != "TestField" {
		t.Errorf("expected field name 'TestField', got %q", doc.AcroForm.Fields[0].FieldName())
	}

	if len(doc.Pages) != 1 {
		t.Fatal("expected 1 page")
	}
	// Note: WidgetAnnotation is not added to Page.Annotations by builder,
	// but handled by writer via AcroForm.Fields.PageIndex.
	// So we check if the field has the correct PageIndex.
	if doc.AcroForm.Fields[0].FieldPageIndex() != 0 {
		t.Errorf("expected field PageIndex 0, got %d", doc.AcroForm.Fields[0].FieldPageIndex())
	}
}

func TestBuilder_FormFill(t *testing.T) {
	// 1. Create a document with a field
	b := NewBuilder()
	field := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "TextField",
			Rect: semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 30},
		},
	}
	cbField := &semantic.ButtonFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "Checkbox",
			Rect: semantic.Rectangle{LLX: 10, LLY: 50, URX: 30, URY: 70},
		},
		IsCheck: true,
	}
	chField := &semantic.ChoiceFormField{
		BaseFormField: semantic.BaseFormField{
			Name: "Choice",
			Rect: semantic.Rectangle{LLX: 10, LLY: 80, URX: 100, URY: 100},
		},
		Selected: []string{"Option1"},
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

	var tf *semantic.TextFormField
	var cf *semantic.ButtonFormField
	var chf *semantic.ChoiceFormField

	for _, f := range doc.AcroForm.Fields {
		if f.FieldName() == "TextField" {
			tf = f.(*semantic.TextFormField)
		} else if f.FieldName() == "Checkbox" {
			cf = f.(*semantic.ButtonFormField)
		} else if f.FieldName() == "Choice" {
			chf = f.(*semantic.ChoiceFormField)
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
	if !cf.Checked {
		t.Errorf("expected Checkbox checked")
	}
	if cf.GetAppearanceState() != "Yes" {
		t.Errorf("expected Checkbox appearance 'Yes', got %q", cf.GetAppearanceState())
	}

	if chf == nil {
		t.Fatal("Choice field not found")
	}
	if len(chf.Selected) == 0 || chf.Selected[0] != "Option2" {
		t.Errorf("expected Choice value 'Option2', got %v", chf.Selected)
	}
}
