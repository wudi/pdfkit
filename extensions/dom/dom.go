package dom

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/scripting"
)

type Adapter struct {
	doc *semantic.Document
}

func New(doc *semantic.Document) *Adapter {
	return &Adapter{doc: doc}
}

func (a *Adapter) GetField(name string) (scripting.FormFieldProxy, error) {
	if a.doc.AcroForm == nil {
		return nil, fmt.Errorf("no form found")
	}
	// Simple linear search for now. In a real implementation, we might want a map.
	for _, f := range a.doc.AcroForm.Fields {
		if f.FieldName() == name {
			return NewFieldProxy(f), nil
		}
	}
	return nil, fmt.Errorf("field not found: %s", name)
}

func (a *Adapter) GetPage(index int) (scripting.PageProxy, error) {
	if index < 0 || index >= len(a.doc.Pages) {
		return nil, fmt.Errorf("page index out of range")
	}
	return NewPageProxy(a.doc.Pages[index]), nil
}

func (a *Adapter) Alert(message string) {
	fmt.Printf("JS Alert: %s\n", message)
}
