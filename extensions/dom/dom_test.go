package dom

import (
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
)

func TestAdapter_GetField(t *testing.T) {
	doc := &semantic.Document{
		AcroForm: &semantic.AcroForm{
			Fields: []semantic.FormField{
				&semantic.TextFormField{
					BaseFormField: semantic.BaseFormField{Name: "TestField"},
					Value:         "InitialValue",
				},
			},
		},
	}

	adapter := New(doc)
	field, err := adapter.GetField("TestField")
	if err != nil {
		t.Fatalf("GetField failed: %v", err)
	}

	if val := field.GetValue(); val != "InitialValue" {
		t.Errorf("Expected value 'InitialValue', got %v", val)
	}

	field.SetValue("NewValue")
	if val := field.GetValue(); val != "NewValue" {
		t.Errorf("Expected value 'NewValue', got %v", val)
	}
}

func TestAdapter_GetPage(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{Index: 0},
			{Index: 1},
		},
	}

	adapter := New(doc)
	page, err := adapter.GetPage(1)
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}

	if idx := page.GetIndex(); idx != 1 {
		t.Errorf("Expected index 1, got %d", idx)
	}
}
