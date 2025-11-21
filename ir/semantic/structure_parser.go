package semantic

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
)

// parseStructureTree parses the logical structure tree from the catalog.
func parseStructureTree(catalog *raw.DictObj, resolver rawResolver) (*StructureTree, error) {
	if catalog == nil {
		return nil, nil
	}
	stRootObj, ok := catalog.Get(raw.NameLiteral("StructTreeRoot"))
	if !ok {
		return nil, nil
	}

	stRootRef := raw.ObjectRef{}
	if ref, ok := stRootObj.(raw.RefObj); ok {
		stRootRef = ref.Ref()
		obj, err := resolver.Resolve(stRootRef)
		if err != nil {
			return nil, fmt.Errorf("resolve StructTreeRoot: %w", err)
		}
		stRootObj = obj
	}

	rootDict, ok := stRootObj.(*raw.DictObj)
	if !ok {
		return nil, fmt.Errorf("StructTreeRoot is not a dictionary")
	}

	tree := &StructureTree{
		Type:        "StructTreeRoot",
		OriginalRef: stRootRef,
	}

	// Parse RoleMap
	if rmObj, ok := rootDict.Get(raw.NameLiteral("RoleMap")); ok {
		if rmDict, ok := resolveDict(rmObj, resolver); ok {
			tree.RoleMap = make(RoleMap)
			for k, v := range rmDict.KV {
				if n, ok := v.(raw.NameObj); ok {
					tree.RoleMap[k] = n.Val
				}
			}
		}
	}

	// Parse ClassMap
	if cmObj, ok := rootDict.Get(raw.NameLiteral("ClassMap")); ok {
		if cmDict, ok := resolveDict(cmObj, resolver); ok {
			tree.ClassMap = make(ClassMap)
			for k, v := range cmDict.KV {
				attr, err := parseAttributeObject(v, resolver)
				if err == nil {
					tree.ClassMap[k] = attr
				}
			}
		}
	}

	// Parse K (Kids)
	if kObj, ok := rootDict.Get(raw.NameLiteral("K")); ok {
		kids, err := parseStructureKids(kObj, nil, resolver)
		if err != nil {
			return nil, err
		}
		// K at root must be StructureElements (not MCIDs)
		for _, item := range kids {
			if item.Element != nil {
				tree.K = append(tree.K, item.Element)
			}
		}
	}

	// ParentTree and IDTree are complex number trees / name trees.
	// We'll skip full parsing for now or implement basic support if needed.
	// The TODO implies "Full StructTree support", so we should probably try.
	// But NumberTree/NameTree parsing is generic.

	return tree, nil
}

func parseStructureKids(kObj raw.Object, parent *StructureElement, resolver rawResolver) ([]StructureItem, error) {
	var items []StructureItem

	// K can be a dictionary (single kid), an array (multiple kids), or integer (MCID - only for elements)

	// Resolve indirect reference first
	if ref, ok := kObj.(raw.RefObj); ok {
		obj, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, err
		}
		kObj = obj
		// If it's a dictionary, it's a child element or OBJR or MCR
		if dict, ok := kObj.(*raw.DictObj); ok {
			return parseStructureItemFromDict(dict, ref.Ref(), parent, resolver)
		}
	}

	switch v := kObj.(type) {
	case *raw.ArrayObj:
		for _, item := range v.Items {
			kids, err := parseStructureKids(item, parent, resolver)
			if err != nil {
				return nil, err
			}
			items = append(items, kids...)
		}
	case *raw.DictObj:
		// It's a direct dictionary (unlikely for K array elements, but possible)
		// We need a reference for it if we want to track it properly, but if it's direct, ref is empty.
		kids, err := parseStructureItemFromDict(v, raw.ObjectRef{}, parent, resolver)
		if err != nil {
			return nil, err
		}
		items = append(items, kids...)
	case raw.NumberObj:
		if v.IsInt {
			items = append(items, StructureItem{MCID: int(v.Int())})
		}
	}
	return items, nil
}

func parseStructureItemFromDict(dict *raw.DictObj, ref raw.ObjectRef, parent *StructureElement, resolver rawResolver) ([]StructureItem, error) {
	// Check Type
	typ := getName(dict, "Type")
	if typ == "MCR" {
		mcr := &MCR{MCID: int(getInt(dict, "MCID"))}
		// Resolve Page
		if _, ok := dict.Get(raw.NameLiteral("Pg")); ok {
			// We need to map page object to Page struct.
			// This requires access to the document's page map.
			// For now, we'll store the raw ref or skip.
			// Ideally, resolver should help or we pass a page mapper.
		}
		return []StructureItem{{MCR: mcr, MCID: -1}}, nil
	}
	if typ == "OBJR" {
		objRef := raw.ObjectRef{}
		if obj, ok := dict.Get(raw.NameLiteral("Obj")); ok {
			if r, ok := obj.(raw.RefObj); ok {
				objRef = r.Ref()
			}
		}
		return []StructureItem{{ObjRef: objRef, MCID: -1}}, nil
	}

	// Otherwise, it's a StructureElement
	elem := &StructureElement{
		Type:        "StructElem",
		S:           getName(dict, "S"),
		P:           parent,
		ID:          getString(dict, "ID"),
		Title:       getString(dict, "T"),
		Lang:        getString(dict, "Lang"),
		Alt:         getString(dict, "Alt"),
		ActualText:  getString(dict, "ActualText"),
		Expanded:    getString(dict, "E"),
		OriginalRef: ref,
	}

	// Parse Attributes (A)
	if aObj, ok := dict.Get(raw.NameLiteral("A")); ok {
		attr, err := parseAttributeObject(aObj, resolver)
		if err == nil {
			elem.A = attr
		}
	}

	// Parse Classes (C)
	// C can be name or array of names.
	// We need to look up in ClassMap.
	// For now, we just store the names? StructureElement has `C *ClassMap`.
	// This seems to imply it stores the resolved attributes.

	// Parse Kids (K)
	if kObj, ok := dict.Get(raw.NameLiteral("K")); ok {
		kids, err := parseStructureKids(kObj, elem, resolver)
		if err != nil {
			return nil, err
		}
		elem.K = kids
	}

	return []StructureItem{{Element: elem, MCID: -1}}, nil
}

func parseAttributeObject(obj raw.Object, resolver rawResolver) (*AttributeObject, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("attribute is not a dictionary")
	}

	attr := &AttributeObject{
		Owner:       getName(dict, "O"),
		Attributes:  make(map[string]interface{}),
		OriginalRef: raw.ObjectRef{}, // We might need to pass ref if resolved
	}

	for k, v := range dict.KV {
		if k == "O" {
			continue
		}
		// Convert raw object to interface{}
		attr.Attributes[k] = v // Simplified
	}
	return attr, nil
}

// Helpers

type rawResolver interface {
	Resolve(ref raw.ObjectRef) (raw.Object, error)
}

func resolveDict(obj raw.Object, resolver rawResolver) (*raw.DictObj, bool) {
	if ref, ok := obj.(raw.RefObj); ok {
		o, err := resolver.Resolve(ref.Ref())
		if err != nil {
			return nil, false
		}
		obj = o
	}
	d, ok := obj.(*raw.DictObj)
	return d, ok
}

func getName(d *raw.DictObj, key string) string {
	if v, ok := d.Get(raw.NameLiteral(key)); ok {
		if n, ok := v.(raw.NameObj); ok {
			return n.Val
		}
	}
	return ""
}

func getString(d *raw.DictObj, key string) string {
	if v, ok := d.Get(raw.NameLiteral(key)); ok {
		if s, ok := v.(raw.StringObj); ok {
			return string(s.Value())
		}
		if s, ok := v.(raw.HexStringObj); ok {
			return string(s.Value())
		}
	}
	return ""
}

func getInt(d *raw.DictObj, key string) int64 {
	if v, ok := d.Get(raw.NameLiteral(key)); ok {
		if n, ok := v.(raw.NumberObj); ok {
			return n.Int()
		}
	}
	return 0
}
