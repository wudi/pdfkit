package extractor

import (
	"testing"

	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestExtractor_AcroForm(t *testing.T) {
	dec := buildAcroFormDoc(t)
	ext, err := New(dec)
	if err != nil {
		t.Fatalf("new extractor: %v", err)
	}

	form, err := ext.ExtractAcroForm()
	if err != nil {
		t.Fatalf("extract acroform: %v", err)
	}
	if form == nil {
		t.Fatal("expected acroform")
	}

	if !form.NeedAppearances {
		t.Error("expected NeedAppearances to be true")
	}

	if len(form.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(form.Fields))
	}

	// Check fields
	f1 := form.Fields[0]
	if f1.FieldName() != "Field1" {
		t.Errorf("expected Field1, got %s", f1.FieldName())
	}
	if f1.FieldType() != "Tx" {
		t.Errorf("expected Tx type, got %s", f1.FieldType())
	}

	f2 := form.Fields[1]
	if f2.FieldName() != "Field2" {
		t.Errorf("expected Field2, got %s", f2.FieldName())
	}
	if f2.FieldType() != "Btn" {
		t.Errorf("expected Btn type, got %s", f2.FieldType())
	}

	// Check Calculation Order
	if len(form.CalculationOrder) != 2 {
		t.Fatalf("expected 2 fields in calculation order, got %d", len(form.CalculationOrder))
	}
	// Order should be Field2, Field1
	if form.CalculationOrder[0].FieldName() != "Field2" {
		t.Errorf("expected first CO field to be Field2, got %s", form.CalculationOrder[0].FieldName())
	}
	if form.CalculationOrder[1].FieldName() != "Field1" {
		t.Errorf("expected second CO field to be Field1, got %s", form.CalculationOrder[1].FieldName())
	}
}

func buildAcroFormDoc(t *testing.T) *decoded.DecodedDocument {
	t.Helper()

	root := raw.Dict()
	pages := raw.Dict()
	page := raw.Dict()

	// Fields
	field1 := raw.Dict()
	field1.Set(raw.NameLiteral("FT"), raw.NameLiteral("Tx"))
	field1.Set(raw.NameLiteral("T"), raw.Str([]byte("Field1")))
	field1.Set(raw.NameLiteral("V"), raw.Str([]byte("Value1")))
	field1.Set(raw.NameLiteral("Rect"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(100), raw.NumberInt(20)))

	field2 := raw.Dict()
	field2.Set(raw.NameLiteral("FT"), raw.NameLiteral("Btn"))
	field2.Set(raw.NameLiteral("T"), raw.Str([]byte("Field2")))
	field2.Set(raw.NameLiteral("V"), raw.NameLiteral("Yes"))
	field2.Set(raw.NameLiteral("Rect"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(30), raw.NumberInt(20), raw.NumberInt(50)))

	// AcroForm
	acroForm := raw.Dict()
	acroForm.Set(raw.NameLiteral("NeedAppearances"), raw.Bool(true))
	acroForm.Set(raw.NameLiteral("Fields"), raw.NewArray(raw.Ref(5, 0), raw.Ref(6, 0)))
	// Calculation Order: Field2 then Field1
	acroForm.Set(raw.NameLiteral("CO"), raw.NewArray(raw.Ref(6, 0), raw.Ref(5, 0)))

	root.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	root.Set(raw.NameLiteral("Pages"), raw.Ref(2, 0))
	root.Set(raw.NameLiteral("AcroForm"), raw.Ref(4, 0))

	pages.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pages.Set(raw.NameLiteral("Kids"), raw.NewArray(raw.Ref(3, 0)))
	pages.Set(raw.NameLiteral("Count"), raw.NumberInt(1))

	page.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
	page.Set(raw.NameLiteral("Parent"), raw.Ref(2, 0))
	page.Set(raw.NameLiteral("Annots"), raw.NewArray(raw.Ref(5, 0), raw.Ref(6, 0)))

	doc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}: root,
			{Num: 2, Gen: 0}: pages,
			{Num: 3, Gen: 0}: page,
			{Num: 4, Gen: 0}: acroForm,
			{Num: 5, Gen: 0}: field1,
			{Num: 6, Gen: 0}: field2,
		},
		Trailer: raw.Dict(),
	}
	doc.Trailer.Set(raw.NameLiteral("Root"), raw.Ref(1, 0))

	return &decoded.DecodedDocument{
		Raw:     doc,
		Streams: map[raw.ObjectRef]decoded.Stream{},
	}
}

func TestExtractor_AcroForm_InheritedFT(t *testing.T) {
	// Build a doc where FT is inherited from parent field
	root := raw.Dict()
	pages := raw.Dict()
	page := raw.Dict()

	// Parent Field (defines FT=Tx)
	parentField := raw.Dict()
	parentField.Set(raw.NameLiteral("FT"), raw.NameLiteral("Tx"))
	parentField.Set(raw.NameLiteral("T"), raw.Str([]byte("Parent")))
	parentField.Set(raw.NameLiteral("Kids"), raw.NewArray(raw.Ref(6, 0)))

	// Child Field (inherits FT)
	childField := raw.Dict()
	childField.Set(raw.NameLiteral("T"), raw.Str([]byte("Child")))
	childField.Set(raw.NameLiteral("V"), raw.Str([]byte("Value")))
	childField.Set(raw.NameLiteral("Parent"), raw.Ref(5, 0))

	// AcroForm
	acroForm := raw.Dict()
	acroForm.Set(raw.NameLiteral("Fields"), raw.NewArray(raw.Ref(5, 0)))

	root.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	root.Set(raw.NameLiteral("Pages"), raw.Ref(2, 0))
	root.Set(raw.NameLiteral("AcroForm"), raw.Ref(4, 0))

	pages.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pages.Set(raw.NameLiteral("Kids"), raw.NewArray(raw.Ref(3, 0)))
	pages.Set(raw.NameLiteral("Count"), raw.NumberInt(1))

	page.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
	page.Set(raw.NameLiteral("Parent"), raw.Ref(2, 0))

	doc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}: root,
			{Num: 2, Gen: 0}: pages,
			{Num: 3, Gen: 0}: page,
			{Num: 4, Gen: 0}: acroForm,
			{Num: 5, Gen: 0}: parentField,
			{Num: 6, Gen: 0}: childField,
		},
		Trailer: raw.Dict(),
	}
	doc.Trailer.Set(raw.NameLiteral("Root"), raw.Ref(1, 0))

	dec := &decoded.DecodedDocument{
		Raw:     doc,
		Streams: map[raw.ObjectRef]decoded.Stream{},
	}

	ext, err := New(dec)
	if err != nil {
		t.Fatalf("new extractor: %v", err)
	}

	form, err := ext.ExtractAcroForm()
	if err != nil {
		t.Fatalf("extract acroform: %v", err)
	}

	// Should find Parent and Child
	if len(form.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(form.Fields))
	}

	// Check Child
	var child semantic.FormField
	for _, f := range form.Fields {
		if f.FieldName() == "Child" {
			child = f
			break
		}
	}
	if child == nil {
		t.Fatal("Child field not found")
	}

	// Child should be TextFormField (inherited FT=Tx)
	if _, ok := child.(*semantic.TextFormField); !ok {
		t.Errorf("expected Child to be TextFormField, got %T", child)
	}
}
