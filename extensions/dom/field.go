package dom

import (
	"github.com/wudi/pdfkit/ir/semantic"
)

type FieldProxy struct {
	field semantic.FormField
}

func NewFieldProxy(f semantic.FormField) *FieldProxy {
	return &FieldProxy{field: f}
}

func (p *FieldProxy) GetValue() interface{} {
	switch f := p.field.(type) {
	case *semantic.TextFormField:
		return f.Value
	case *semantic.ButtonFormField:
		if f.IsCheck || f.IsRadio {
			return f.Checked
		}
		return f.OnState // Or caption?
	case *semantic.ChoiceFormField:
		if f.IsMultiSelect {
			return f.Selected
		}
		if len(f.Selected) > 0 {
			return f.Selected[0]
		}
		return ""
	default:
		return nil
	}
}

func (p *FieldProxy) SetValue(val interface{}) {
	p.field.SetDirty(true)
	switch f := p.field.(type) {
	case *semantic.TextFormField:
		if s, ok := val.(string); ok {
			f.Value = s
		}
	case *semantic.ButtonFormField:
		if b, ok := val.(bool); ok {
			f.Checked = b
		}
	case *semantic.ChoiceFormField:
		if s, ok := val.(string); ok {
			f.Selected = []string{s}
		} else if strs, ok := val.([]string); ok {
			f.Selected = strs
		}
	}
}
