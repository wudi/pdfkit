package semantic

import (
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
)

// parseStructureTree parses the logical structure tree from the catalog.
func parseStructureTree(catalog *raw.DictObj, resolver rawResolver, pages []*Page) (*StructureTree, error) {
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

	pageMap := buildPageMap(pages)

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
		kids, err := parseStructureKids(kObj, nil, resolver, pageMap)
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

	// Parse ParentTree
	if ptObj, ok := rootDict.Get(raw.NameLiteral("ParentTree")); ok {
		pt, err := parseNumberTree(ptObj, resolver)
		if err == nil {
			tree.ParentTree = make(map[int][]interface{})
			for k, v := range pt {
				var items []interface{}
				if arr, ok := v.(*raw.ArrayObj); ok {
					for _, item := range arr.Items {
						items = append(items, item)
					}
				} else {
					items = append(items, v)
				}
				tree.ParentTree[k] = items
			}
		}
	}

	// Parse Namespaces (PDF 2.0)
	if nsObj, ok := rootDict.Get(raw.NameLiteral("Namespaces")); ok {
		if nsArr, ok := nsObj.(*raw.ArrayObj); ok {
			for _, item := range nsArr.Items {
				ns, err := parseNamespace(item, resolver)
				if err == nil {
					tree.Namespaces = append(tree.Namespaces, ns)
				}
			}
		}
	}

	// Parse IDTree
	// Instead of parsing the NameTree directly (which gives raw references),
	// we traverse the parsed structure tree to populate the ID map.
	// This ensures consistency with the in-memory tree.
	tree.IDTree = make(map[string]*StructureElement)
	var collectIDs func(elem *StructureElement)
	collectIDs = func(elem *StructureElement) {
		if elem == nil {
			return
		}
		if elem.ID != "" {
			tree.IDTree[elem.ID] = elem
		}
		for _, kid := range elem.K {
			if kid.Element != nil {
				collectIDs(kid.Element)
			}
		}
	}

	for _, kid := range tree.K {
		collectIDs(kid)
	}

	return tree, nil
}

func buildPageMap(pages []*Page) map[raw.ObjectRef]*Page {
	m := make(map[raw.ObjectRef]*Page)
	for _, p := range pages {
		if p.OriginalRef.Num != 0 {
			m[p.OriginalRef] = p
		}
	}
	return m
}

func parseStructureKids(kObj raw.Object, parent *StructureElement, resolver rawResolver, pageMap map[raw.ObjectRef]*Page) ([]StructureItem, error) {
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
			return parseStructureItemFromDict(dict, ref.Ref(), parent, resolver, pageMap)
		}
	}

	switch v := kObj.(type) {
	case *raw.ArrayObj:
		for _, item := range v.Items {
			kids, err := parseStructureKids(item, parent, resolver, pageMap)
			if err != nil {
				return nil, err
			}
			items = append(items, kids...)
		}
	case *raw.DictObj:
		kids, err := parseStructureItemFromDict(v, raw.ObjectRef{}, parent, resolver, pageMap)
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

func parseStructureItemFromDict(dict *raw.DictObj, ref raw.ObjectRef, parent *StructureElement, resolver rawResolver, pageMap map[raw.ObjectRef]*Page) ([]StructureItem, error) {
	// Check Type
	typ := getName(dict, "Type")
	if typ == "MCR" {
		mcr := &MCR{MCID: int(getInt(dict, "MCID"))}
		// Resolve Page
		if pgObj, ok := dict.Get(raw.NameLiteral("Pg")); ok {
			if pgRef, ok := pgObj.(raw.RefObj); ok {
				if p, ok := pageMap[pgRef.Ref()]; ok {
					mcr.Pg = p
				}
			}
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

	// Resolve Page for Element
	if pgObj, ok := dict.Get(raw.NameLiteral("Pg")); ok {
		if pgRef, ok := pgObj.(raw.RefObj); ok {
			if p, ok := pageMap[pgRef.Ref()]; ok {
				elem.Pg = p
			}
		}
	}

	// Parse Attributes (A)
	if aObj, ok := dict.Get(raw.NameLiteral("A")); ok {
		attr, err := parseAttributeObject(aObj, resolver)
		if err == nil {
			elem.A = attr
		}
	}

	// Parse Namespace (PDF 2.0)
	if nsObj, ok := dict.Get(raw.NameLiteral("NS")); ok {
		ns, err := parseNamespace(nsObj, resolver)
		if err == nil {
			elem.Namespace = ns
		}
	}

	// Parse Kids (K)
	if kObj, ok := dict.Get(raw.NameLiteral("K")); ok {
		kids, err := parseStructureKids(kObj, elem, resolver, pageMap)
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
		// It might be an array of attributes
		if arr, ok := obj.(*raw.ArrayObj); ok {
			// Merge attributes? Or just take the first one?
			// For simplicity, we'll take the first dictionary we find.
			for _, item := range arr.Items {
				if d, ok := resolveDict(item, resolver); ok {
					dict = d
					break
				}
			}
		}
		if dict == nil {
			return nil, fmt.Errorf("attribute is not a dictionary or array of dictionaries")
		}
	}

	attr := &AttributeObject{
		Owner:       getName(dict, "O"),
		Attributes:  make(map[string]interface{}),
		OriginalRef: raw.ObjectRef{},
	}

	if ref, ok := obj.(raw.RefObj); ok {
		attr.OriginalRef = ref.Ref()
	}

	for k, v := range dict.KV {
		if k == "O" {
			continue
		}
		// Convert raw object to interface{}
		attr.Attributes[k] = convertRawToInterface(v)
	}
	return attr, nil
}

func parseNamespace(obj raw.Object, resolver rawResolver) (*Namespace, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("namespace is not a dictionary")
	}

	ns := &Namespace{
		Type:        "Namespace",
		NS:          getString(dict, "NS"),
		OriginalRef: raw.ObjectRef{},
	}

	if ref, ok := obj.(raw.RefObj); ok {
		ns.OriginalRef = ref.Ref()
	}

	// Parse RoleMapNS
	if rmObj, ok := dict.Get(raw.NameLiteral("RoleMapNS")); ok {
		if rmDict, ok := resolveDict(rmObj, resolver); ok {
			ns.RoleMapNS = make(RoleMap)
			for k, v := range rmDict.KV {
				if n, ok := v.(raw.NameObj); ok {
					ns.RoleMapNS[k] = n.Val
				}
			}
		}
	}

	// Parse Schema
	if schemaObj, ok := dict.Get(raw.NameLiteral("Schema")); ok {
		schema, err := parseSchema(schemaObj, resolver)
		if err == nil {
			ns.Schema = schema
		}
	}

	return ns, nil
}

func parseSchema(obj raw.Object, resolver rawResolver) (*Schema, error) {
	dict, ok := resolveDict(obj, resolver)
	if !ok {
		return nil, fmt.Errorf("schema is not a dictionary")
	}

	schema := &Schema{
		Type:        "Schema",
		O:           getString(dict, "O"),
		NS:          getString(dict, "NS"),
		OriginalRef: raw.ObjectRef{},
	}

	if ref, ok := obj.(raw.RefObj); ok {
		schema.OriginalRef = ref.Ref()
	}

	// Parse RoleMap
	if rmObj, ok := dict.Get(raw.NameLiteral("RoleMap")); ok {
		if rmDict, ok := resolveDict(rmObj, resolver); ok {
			schema.RoleMap = make(RoleMap)
			for k, v := range rmDict.KV {
				if n, ok := v.(raw.NameObj); ok {
					schema.RoleMap[k] = n.Val
				}
			}
		}
	}

	// Parse ClassMap
	if cmObj, ok := dict.Get(raw.NameLiteral("ClassMap")); ok {
		if cmDict, ok := resolveDict(cmObj, resolver); ok {
			schema.ClassMap = make(ClassMap)
			for k, v := range cmDict.KV {
				attr, err := parseAttributeObject(v, resolver)
				if err == nil {
					schema.ClassMap[k] = attr
				}
			}
		}
	}

	return schema, nil
}

func convertRawToInterface(obj raw.Object) interface{} {
	switch v := obj.(type) {
	case raw.NameObj:
		return v.Val
	case raw.StringObj:
		return string(v.Bytes)
	case raw.NumberObj:
		if v.IsInt {
			return v.Int()
		}
		return v.Float()
	case raw.BoolObj:
		return v.Value()
	case *raw.ArrayObj:
		var arr []interface{}
		for _, item := range v.Items {
			arr = append(arr, convertRawToInterface(item))
		}
		return arr
	case *raw.DictObj:
		m := make(map[string]interface{})
		for k, val := range v.KV {
			m[k] = convertRawToInterface(val)
		}
		return m
	default:
		return nil
	}
}

// NumberTree parsing
func parseNumberTree(root raw.Object, resolver rawResolver) (map[int]raw.Object, error) {
	result := make(map[int]raw.Object)

	var visit func(obj raw.Object) error
	visit = func(obj raw.Object) error {
		dict, ok := resolveDict(obj, resolver)
		if !ok {
			return nil
		}

		// Kids
		if kidsObj, ok := dict.Get(raw.NameLiteral("Kids")); ok {
			if kidsArr, ok := kidsObj.(*raw.ArrayObj); ok {
				for _, kid := range kidsArr.Items {
					if err := visit(kid); err != nil {
						return err
					}
				}
			}
		}

		// Nums
		if numsObj, ok := dict.Get(raw.NameLiteral("Nums")); ok {
			if numsArr, ok := numsObj.(*raw.ArrayObj); ok {
				for i := 0; i+1 < len(numsArr.Items); i += 2 {
					keyObj := numsArr.Items[i]
					valObj := numsArr.Items[i+1]

					var key int
					if n, ok := keyObj.(raw.NumberObj); ok {
						key = int(n.Int())
					} else {
						continue
					}

					result[key] = valObj
				}
			}
		}
		return nil
	}

	if err := visit(root); err != nil {
		return nil, err
	}
	return result, nil
}

// NameTree parsing
func parseNameTree(root raw.Object, resolver rawResolver) (map[string]raw.Object, error) {
	result := make(map[string]raw.Object)

	var visit func(obj raw.Object) error
	visit = func(obj raw.Object) error {
		dict, ok := resolveDict(obj, resolver)
		if !ok {
			return nil
		}

		// Kids
		if kidsObj, ok := dict.Get(raw.NameLiteral("Kids")); ok {
			if kidsArr, ok := kidsObj.(*raw.ArrayObj); ok {
				for _, kid := range kidsArr.Items {
					if err := visit(kid); err != nil {
						return err
					}
				}
			}
		}

		// Names
		if namesObj, ok := dict.Get(raw.NameLiteral("Names")); ok {
			if namesArr, ok := namesObj.(*raw.ArrayObj); ok {
				for i := 0; i+1 < len(namesArr.Items); i += 2 {
					keyObj := namesArr.Items[i]
					valObj := namesArr.Items[i+1]

					var key string
					if s, ok := keyObj.(raw.StringObj); ok {
						key = string(s.Bytes)
					} else if s, ok := keyObj.(raw.HexStringObj); ok {
						key = string(s.Value())
					} else {
						continue
					}

					result[key] = valObj
				}
			}
		}
		return nil
	}

	if err := visit(root); err != nil {
		return nil, err
	}
	return result, nil
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
