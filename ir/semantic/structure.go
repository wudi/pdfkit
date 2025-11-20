package semantic

import "pdflib/ir/raw"

// StructureTree is the root of the logical structure.
type StructureTree struct {
	Type              string // /StructTreeRoot
	K                 []*StructureElement
	IDTree            map[string]*StructureElement
	ParentTree        map[int][]interface{} // PageIndex -> []StructureItem
	ParentTreeNextKey int
	RoleMap           RoleMap
	ClassMap          ClassMap
	Namespaces        []*Namespace // PDF 2.0
	OriginalRef       raw.ObjectRef
	Dirty             bool
}

// StructureElement represents a node in the structure tree.
type StructureElement struct {
	Type            string            // /StructElem
	S               string            // Structure type (e.g., P, H1)
	P               *StructureElement // Parent
	ID              string
	Pg              *Page            // Page containing the content
	K               []StructureItem  // Children
	A               *AttributeObject // Attributes
	C               *ClassMap        // Classes
	R               int              // Revision number
	Title           string
	Lang            string
	Alt             string
	Expanded        string
	ActualText      string
	AssociatedFiles []EmbeddedFile // PDF 2.0
	Namespace       *Namespace     // PDF 2.0
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// StructureItem represents a child of a structure element.
// It can be another StructureElement, a Marked Content ID (MCID), or an Object Reference (MCR).
type StructureItem struct {
	Element *StructureElement
	MCID    int           // -1 if not an MCID
	MCR     *MCR          // Marked Content Reference
	ObjRef  raw.ObjectRef // For OBJR
}

// MCR (Marked Content Reference)
type MCR struct {
	Pg   *Page
	MCID int
	Stm  raw.ObjectRef // Optional stream containing the marked content
}

// RoleMap maps structure types to standard types.
type RoleMap map[string]string

// ClassMap maps class names to attribute objects.
type ClassMap map[string]*AttributeObject

// Namespace represents a PDF 2.0 Namespace dictionary.
type Namespace struct {
	Type        string // /Namespace
	NS          string // URI
	RoleMapNS   RoleMap
	Schema      *Schema
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// Schema represents a PDF 2.0 Structure Element Schema.
type Schema struct {
	// Simplified for now
	OriginalRef raw.ObjectRef
}

// AttributeObject represents Layout or PrintField attributes.
type AttributeObject struct {
	Owner       string // /O (e.g., /Layout, /List, /Table)
	Attributes  map[string]interface{}
	OriginalRef raw.ObjectRef
	Dirty       bool
}
