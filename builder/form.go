package builder

import (
	"github.com/wudi/pdfkit/ir/semantic"
)

// FormBuilder provides a fluent API for form filling.
type FormBuilder interface {
	// SetText sets the value of a text field.
	SetText(name, value string) FormBuilder
	// SetCheckbox sets the state of a checkbox.
	SetCheckbox(name string, checked bool) FormBuilder
	// SetChoice sets the selected value of a choice field (combo box or list box).
	SetChoice(name, value string) FormBuilder
	// Finish returns to the PDFBuilder.
	Finish() PDFBuilder
}

type formBuilderImpl struct {
	parent *builderImpl
	form   *semantic.AcroForm
}

func (b *builderImpl) Form() FormBuilder {
	if b.acroForm == nil {
		b.acroForm = &semantic.AcroForm{NeedAppearances: true}
	}
	return &formBuilderImpl{parent: b, form: b.acroForm}
}

func (fb *formBuilderImpl) SetText(name, value string) FormBuilder {
	for i := range fb.form.Fields {
		if fb.form.Fields[i].FieldName() == name {
			if tf, ok := fb.form.Fields[i].(*semantic.TextFormField); ok {
				tf.Value = value
			} else if gf, ok := fb.form.Fields[i].(*semantic.GenericFormField); ok {
				gf.Value = value
			}
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.FieldName() == name {
			if tf, ok := fb.parent.pendingFields[i].field.(*semantic.TextFormField); ok {
				tf.Value = value
			} else if gf, ok := fb.parent.pendingFields[i].field.(*semantic.GenericFormField); ok {
				gf.Value = value
			}
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) SetCheckbox(name string, checked bool) FormBuilder {
	for i := range fb.form.Fields {
		if fb.form.Fields[i].FieldName() == name {
			if bf, ok := fb.form.Fields[i].(*semantic.ButtonFormField); ok {
				bf.Checked = checked
				if checked {
					state := bf.OnState
					if state == "" {
						state = "Yes"
					}
					bf.AppearanceState = state
				} else {
					bf.AppearanceState = "Off"
				}
			} else if gf, ok := fb.form.Fields[i].(*semantic.GenericFormField); ok {
				if checked {
					gf.Value = "Yes"
					gf.AppearanceState = "Yes"
				} else {
					gf.Value = "Off"
					gf.AppearanceState = "Off"
				}
			}
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.FieldName() == name {
			if bf, ok := fb.parent.pendingFields[i].field.(*semantic.ButtonFormField); ok {
				bf.Checked = checked
				if checked {
					state := bf.OnState
					if state == "" {
						state = "Yes"
					}
					bf.AppearanceState = state
				} else {
					bf.AppearanceState = "Off"
				}
			} else if gf, ok := fb.parent.pendingFields[i].field.(*semantic.GenericFormField); ok {
				if checked {
					gf.Value = "Yes"
					gf.AppearanceState = "Yes"
				} else {
					gf.Value = "Off"
					gf.AppearanceState = "Off"
				}
			}
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) SetChoice(name, value string) FormBuilder {
	for i := range fb.form.Fields {
		if fb.form.Fields[i].FieldName() == name {
			if cf, ok := fb.form.Fields[i].(*semantic.ChoiceFormField); ok {
				cf.Selected = []string{value}
			} else if gf, ok := fb.form.Fields[i].(*semantic.GenericFormField); ok {
				gf.Value = value
			}
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.FieldName() == name {
			if cf, ok := fb.parent.pendingFields[i].field.(*semantic.ChoiceFormField); ok {
				cf.Selected = []string{value}
			} else if gf, ok := fb.parent.pendingFields[i].field.(*semantic.GenericFormField); ok {
				gf.Value = value
			}
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) Finish() PDFBuilder {
	return fb.parent
}
