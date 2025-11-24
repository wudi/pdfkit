package xfa

import (
	"strings"
)

// Binder handles the binding of data from Datasets to the Template.
type Binder struct {
	Form *Form
}

// NewBinder creates a new Binder for the given form.
func NewBinder(form *Form) *Binder {
	return &Binder{Form: form}
}

// Bind performs the data binding process.
// It traverses the Template and populates Field values from the Datasets.
func (b *Binder) Bind() {
	if b.Form.Template == nil || b.Form.Datasets == nil || b.Form.Datasets.Data == nil {
		return
	}

	// The root of data is usually the first child of <xfa:data>
	// In a real XFA processor, we'd match the top-level subform name to the data root.
	var dataRoot *Node
	if len(b.Form.Datasets.Data.Nodes) > 0 {
		dataRoot = b.Form.Datasets.Data.Nodes[0]
	}

	if dataRoot != nil {
		// If the top-level subform name matches the data root, we start there.
		// Otherwise, we might need to look inside.
		if b.Form.Template.Subform != nil {
			if b.Form.Template.Subform.Name == dataRoot.XMLName.Local {
				b.bindSubform(b.Form.Template.Subform, dataRoot)
			} else {
				// Try to find the subform name inside the data root
				match := b.findDataNode(dataRoot, b.Form.Template.Subform.Name)
				if match != nil {
					b.bindSubform(b.Form.Template.Subform, match)
				} else {
					// Fallback: bind assuming dataRoot IS the context for the subform
					b.bindSubform(b.Form.Template.Subform, dataRoot)
				}
			}
		}
	}
}

func (b *Binder) bindSubform(subform *Subform, dataNode *Node) {
	if subform == nil {
		return
	}

	for _, item := range subform.Items {
		switch v := item.(type) {
		case *Field:
			b.bindField(v, dataNode)
		case *Subform:
			// Determine binding for the child subform
			var childDataNode *Node

			bindingName := v.Name
			if v.Bind != nil && v.Bind.Match == "dataRef" && v.Bind.Ref != "" {
				bindingName = b.resolveRef(v.Bind.Ref)
			}

			if v.Bind != nil && v.Bind.Match == "none" {
				childDataNode = nil // No binding
			} else {
				childDataNode = b.findDataNode(dataNode, bindingName)
			}

			if childDataNode != nil {
				b.bindSubform(v, childDataNode)
			} else {
				b.bindSubform(v, nil)
			}
		}
	}
}

func (b *Binder) bindField(field *Field, dataNode *Node) {
	if dataNode == nil {
		return
	}

	if field.Bind != nil && field.Bind.Match == "none" {
		return
	}

	// Determine binding path
	bindingName := field.Name
	if field.Bind != nil && field.Bind.Match == "dataRef" && field.Bind.Ref != "" {
		bindingName = b.resolveRef(field.Bind.Ref)
	}

	// Find data node
	targetNode := b.findDataNode(dataNode, bindingName)
	if targetNode != nil {
		// Update Field Value
		if field.Value == nil {
			field.Value = &Value{}
		}

		val := strings.TrimSpace(targetNode.Content)

		// Populate the correct field in Value based on what's currently there or UI type
		// For now, we populate Text as a generic container, and specific ones if they exist.
		if field.Value.Integer != "" {
			field.Value.Integer = val
		} else if field.Value.Decimal != "" {
			field.Value.Decimal = val
		} else if field.Value.Float != "" {
			field.Value.Float = val
		} else if field.Value.Boolean != "" {
			field.Value.Boolean = val
		} else if field.Value.Date != "" {
			field.Value.Date = val
		} else {
			// Default to Text
			field.Value.Text = val
		}
	}
}

func (b *Binder) findDataNode(parent *Node, name string) *Node {
	if parent == nil {
		return nil
	}
	// Handle simple name matching
	for _, child := range parent.Children {
		if child.XMLName.Local == name {
			return child
		}
	}
	return nil
}

func (b *Binder) resolveRef(ref string) string {
	// Simplified SOM expression handling
	// e.g. "$record.Address" -> "Address"
	// e.g. "Address" -> "Address"
	parts := strings.Split(ref, ".")
	return parts[len(parts)-1]
}
