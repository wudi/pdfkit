package extractor

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

// ExtractAcroForm extracts the AcroForm dictionary and its fields.
func (e *Extractor) ExtractAcroForm() (*semantic.AcroForm, error) {
	catalog := e.catalog
	if catalog == nil {
		return nil, fmt.Errorf("catalog not found")
	}

	acroFormObj := valueFromDict(catalog, "AcroForm")
	acroFormDict := derefDict(e.raw, acroFormObj)
	if acroFormDict == nil {
		return nil, nil // No AcroForm
	}

	form := &semantic.AcroForm{}
	if needApp, ok := boolFromDict(acroFormDict, "NeedAppearances"); ok {
		form.NeedAppearances = needApp
	}

	// Map to store extracted fields by their object reference for CO resolution
	fieldMap := make(map[raw.ObjectRef]semantic.FormField)

	// Extract Fields
	fieldsArr := derefArray(e.raw, valueFromDict(acroFormDict, "Fields"))
	if fieldsArr != nil {
		for _, item := range fieldsArr.Items {
			e.walkField(item, "", fieldMap, &form.Fields)
		}
	}

	// Extract Calculation Order (CO)
	coArr := derefArray(e.raw, valueFromDict(acroFormDict, "CO"))
	if coArr != nil {
		for _, item := range coArr.Items {
			refObj, ok := item.(raw.RefObj)
			if !ok {
				continue
			}
			if field, ok := fieldMap[refObj.Ref()]; ok {
				form.CalculationOrder = append(form.CalculationOrder, field)
			}
		}
	}

	return form, nil
}

func (e *Extractor) walkField(obj raw.Object, parentFT string, fieldMap map[raw.ObjectRef]semantic.FormField, list *[]semantic.FormField) {
	dict := derefDict(e.raw, obj)
	if dict == nil {
		return
	}

	// Resolve FT for this node
	ft, _ := nameFromDict(dict, "FT")
	if ft == "" {
		ft = parentFT
	}

	// Check if this is a field (has T) or a widget (no T, but might be a child widget)
	// If it has T, it's a field.
	// If it doesn't have T, it might be a widget node of a parent field, or an intermediate node.
	// For semantic extraction, we usually care about the "Field" concept.

	// However, in PDF, the hierarchy can be complex.
	// Let's try to extract it as a field if it looks like one.

	// Inheritable attributes need to be handled if we want to be correct,
	// but for now let's assume explicit attributes or simple structure.

	// If it has "Kids", we recurse.
	kidsArr := derefArray(e.raw, valueFromDict(dict, "Kids"))

	// If it has "T", it is a field.
	if _, hasT := dict.Get(raw.NameLiteral("T")); hasT {
		field, err := e.extractFieldNode(obj, dict, ft)
		if err == nil && field != nil {
			*list = append(*list, field)

			// Map reference
			if ref, ok := obj.(raw.RefObj); ok {
				fieldMap[ref.Ref()] = field
			}
		}
	}

	// Recurse into kids
	if kidsArr != nil {
		for _, kid := range kidsArr.Items {
			e.walkField(kid, ft, fieldMap, list)
		}
	}
}

func (e *Extractor) extractFieldNode(obj raw.Object, dict *raw.DictObj, inheritedFT string) (semantic.FormField, error) {
	// Determine field type
	ft, _ := nameFromDict(dict, "FT")
	if ft == "" {
		ft = inheritedFT
	}

	// If FT is missing, it might be inherited.
	// For this pass, if FT is missing, we might skip or treat as generic.
	// But wait, if we are walking down, we don't pass the parent's FT.
	// This is a limitation of this simple implementation.

	var field semantic.FormField
	base := semantic.BaseFormField{}

	// Common properties
	base.Name, _ = stringFromDict(dict, "T")
	base.Flags, _ = intFromObject(valueFromDict(dict, "Ff"))
	base.DefaultAppearance, _ = stringFromDict(dict, "DA")
	if q, ok := intFromObject(valueFromDict(dict, "Q")); ok {
		base.Quadding = q
	}

	// Capture reference if available
	if ref, ok := obj.(raw.RefObj); ok {
		base.Ref = ref.Ref()
	}

	// Rect
	if rectArr := derefArray(e.raw, valueFromDict(dict, "Rect")); rectArr != nil {
		base.Rect = rectFromRaw(rectArr)
	}

	switch ft {
	case "Tx":
		tx := &semantic.TextFormField{BaseFormField: base}
		tx.Value, _ = stringFromObject(valueFromDict(dict, "V"))
		tx.MaxLen, _ = intFromObject(valueFromDict(dict, "MaxLen"))
		field = tx
	case "Btn":
		btn := &semantic.ButtonFormField{BaseFormField: base}
		flags := base.Flags
		if flags&(1<<16) != 0 {
			btn.IsPush = true
		} else if flags&(1<<15) != 0 {
			btn.IsRadio = true
		} else {
			btn.IsCheck = true
		}
		if v, ok := nameFromObject(valueFromDict(dict, "V")); ok {
			if v != "Off" {
				btn.Checked = true
				btn.OnState = v
			}
		}

		// Extract Caption from MK
		mkObj := valueFromDict(dict, "MK")
		mkDict := derefDict(e.raw, mkObj)
		if mkDict != nil {
			if ca, ok := stringFromDict(mkDict, "CA"); ok {
				btn.Caption = ca
			}
		}

		field = btn
	case "Ch":
		ch := &semantic.ChoiceFormField{BaseFormField: base}
		flags := base.Flags
		if flags&(1<<17) != 0 {
			ch.IsCombo = true
		}
		if optArr := derefArray(e.raw, valueFromDict(dict, "Opt")); optArr != nil {
			for _, o := range optArr.Items {
				if s, ok := stringFromObject(o); ok {
					ch.Options = append(ch.Options, s)
				} else if arr, ok := o.(*raw.ArrayObj); ok && len(arr.Items) == 2 {
					if s, ok := stringFromObject(arr.Items[1]); ok {
						ch.Options = append(ch.Options, s)
					}
				}
			}
		}
		if v, ok := stringFromObject(valueFromDict(dict, "V")); ok {
			ch.Selected = []string{v}
		}
		field = ch
	case "Sig":
		sig := &semantic.SignatureFormField{BaseFormField: base}
		field = sig
	default:
		// If FT is missing but it has T, it might be a non-terminal node acting as a field group,
		// or a field inheriting FT.
		// For now, treat as generic if we can't determine.
		gen := &semantic.GenericFormField{BaseFormField: base}
		gen.Type = ft
		field = gen
	}

	return field, nil
}

func rectFromRaw(arr *raw.ArrayObj) semantic.Rectangle {
	r := semantic.Rectangle{}
	if len(arr.Items) >= 4 {
		r.LLX, _ = floatFromObject(arr.Items[0])
		r.LLY, _ = floatFromObject(arr.Items[1])
		r.URX, _ = floatFromObject(arr.Items[2])
		r.URY, _ = floatFromObject(arr.Items[3])
	}
	return r
}
