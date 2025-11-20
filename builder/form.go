package builder

import (
	"pdflib/ir/semantic"
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
		if fb.form.Fields[i].Name == name {
			fb.form.Fields[i].Value = value
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.Name == name {
			fb.parent.pendingFields[i].field.Value = value
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) SetCheckbox(name string, checked bool) FormBuilder {
	val := "Off"
	if checked {
		val = "Yes" // Default export value
	}

	for i := range fb.form.Fields {
		if fb.form.Fields[i].Name == name {
			fb.form.Fields[i].Value = val
			if checked {
				fb.form.Fields[i].AppearanceState = "Yes"
			} else {
				fb.form.Fields[i].AppearanceState = "Off"
			}
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.Name == name {
			fb.parent.pendingFields[i].field.Value = val
			if checked {
				fb.parent.pendingFields[i].field.AppearanceState = "Yes"
			} else {
				fb.parent.pendingFields[i].field.AppearanceState = "Off"
			}
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) SetChoice(name, value string) FormBuilder {
	for i := range fb.form.Fields {
		if fb.form.Fields[i].Name == name {
			fb.form.Fields[i].Value = value
			return fb
		}
	}
	for i := range fb.parent.pendingFields {
		if fb.parent.pendingFields[i].field.Name == name {
			fb.parent.pendingFields[i].field.Value = value
			return fb
		}
	}
	return fb
}

func (fb *formBuilderImpl) Finish() PDFBuilder {
	return fb.parent
}
