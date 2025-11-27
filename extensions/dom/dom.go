package dom

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/scripting"
)

type Adapter struct {
	doc      *semantic.Document
	fieldMap map[string]scripting.FormFieldProxy
}

func New(doc *semantic.Document) *Adapter {
	a := &Adapter{doc: doc}
	a.buildFieldMap()
	return a
}

func (a *Adapter) buildFieldMap() {
	a.fieldMap = make(map[string]scripting.FormFieldProxy)
	if a.doc.AcroForm == nil {
		return
	}
	for _, f := range a.doc.AcroForm.Fields {
		a.fieldMap[f.FieldName()] = NewFieldProxy(f)
	}
}

func (a *Adapter) GetField(name string) (scripting.FormFieldProxy, error) {
	if val, ok := a.fieldMap[name]; ok {
		return val, nil
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
