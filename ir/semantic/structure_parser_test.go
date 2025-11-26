package semantic

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestParseStructureTree_IDTreeResolution(t *testing.T) {
	// Construct a StructTreeRoot with nested elements having IDs
	// Root -> Elem1 (ID="id1") -> Elem2 (ID="id2")

	elem2 := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "StructElem"},
			"S":    raw.NameObj{Val: "P"},
			"ID":   raw.StringObj{Bytes: []byte("id2")},
		},
	}

	elem1 := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "StructElem"},
			"S":    raw.NameObj{Val: "Div"},
			"ID":   raw.StringObj{Bytes: []byte("id1")},
			"K":    &raw.ArrayObj{Items: []raw.Object{elem2}},
		},
	}

	root := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type":           raw.NameObj{Val: "StructTreeRoot"},
			"StructTreeRoot": raw.NameObj{Val: "StructTreeRoot"}, // Self-reference simulation or just needed for check?
			// The parser checks catalog.Get("StructTreeRoot").
			// Here we are passing the catalog to parseStructureTree?
			// No, parseStructureTree takes the catalog.
		},
	}
	// Actually parseStructureTree takes the catalog.
	catalog := &raw.DictObj{
		KV: map[string]raw.Object{
			"StructTreeRoot": root,
		},
	}

	// Add K to root
	root.KV["K"] = &raw.ArrayObj{Items: []raw.Object{elem1}}

	resolver := &mockResolver{}

	tree, err := parseStructureTree(catalog, resolver, nil)
	if err != nil {
		t.Fatalf("parseStructureTree failed: %v", err)
	}

	if tree == nil {
		t.Fatal("expected structure tree")
	}

	if len(tree.IDTree) != 2 {
		t.Fatalf("expected 2 IDs in IDTree, got %d", len(tree.IDTree))
	}

	if e1, ok := tree.IDTree["id1"]; !ok {
		t.Error("id1 not found")
	} else if e1.S != "Div" {
		t.Errorf("expected id1 to be Div, got %s", e1.S)
	}

	if e2, ok := tree.IDTree["id2"]; !ok {
		t.Error("id2 not found")
	} else if e2.S != "P" {
		t.Errorf("expected id2 to be P, got %s", e2.S)
	}
}

func TestParseStructureTree_Namespaces(t *testing.T) {
	// Construct a StructTreeRoot with Namespaces

	schemaDict := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "Schema"},
			"O":    raw.StringObj{Bytes: []byte("Owner")},
			"NS":   raw.StringObj{Bytes: []byte("http://example.com/ns")},
		},
	}

	nsDict := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type":   raw.NameObj{Val: "Namespace"},
			"NS":     raw.StringObj{Bytes: []byte("http://example.com/ns")},
			"Schema": schemaDict,
		},
	}

	root := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type":       raw.NameObj{Val: "StructTreeRoot"},
			"Namespaces": &raw.ArrayObj{Items: []raw.Object{nsDict}},
		},
	}

	catalog := &raw.DictObj{
		KV: map[string]raw.Object{
			"StructTreeRoot": root,
		},
	}

	resolver := &mockResolver{}

	tree, err := parseStructureTree(catalog, resolver, nil)
	if err != nil {
		t.Fatalf("parseStructureTree failed: %v", err)
	}

	if len(tree.Namespaces) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(tree.Namespaces))
	}

	ns := tree.Namespaces[0]
	if ns.NS != "http://example.com/ns" {
		t.Errorf("expected NS to be http://example.com/ns, got %s", ns.NS)
	}

	if ns.Schema == nil {
		t.Fatal("expected Schema to be present")
	}

	if ns.Schema.O != "Owner" {
		t.Errorf("expected Schema Owner to be Owner, got %s", ns.Schema.O)
	}
}
