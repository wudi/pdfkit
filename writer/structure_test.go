package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/parser"
)

func TestStructureTree(t *testing.T) {
	// Create a simple document with structure
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Index:    0,
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
			},
		},
	}

	// Create structure tree
	tree := &semantic.StructureTree{
		Type: "StructTreeRoot",
		RoleMap: semantic.RoleMap{
			"MyPara": "P",
		},
	}

	// Create elements
	p1 := &semantic.StructureElement{
		Type:  "StructElem",
		S:     "MyPara",
		Title: "Paragraph 1",
		Pg:    doc.Pages[0],
	}

	// Add content item (MCID 0)
	p1.K = []semantic.StructureItem{
		{MCID: 0},
	}

	tree.K = []*semantic.StructureElement{p1}
	doc.StructTree = tree

	// Write
	var buf bytes.Buffer
	wb := &WriterBuilder{}
	w := wb.Build()
	err := w.Write(context.Background(), doc, &buf, Config{Version: PDF17})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Parse back
	p := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Decode
	dec := decoded.NewDecoder(nil)
	decDoc, err := dec.Decode(context.Background(), rawDoc)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Build semantic
	sb := semantic.NewBuilder()
	parsedDoc, err := sb.Build(context.Background(), decDoc)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify structure
	if parsedDoc.StructTree == nil {
		t.Fatal("StructTree is nil after parsing")
	}

	if len(parsedDoc.StructTree.RoleMap) != 1 {
		t.Errorf("Expected 1 RoleMap entry, got %d", len(parsedDoc.StructTree.RoleMap))
	}
	if v, ok := parsedDoc.StructTree.RoleMap["MyPara"]; !ok || v != "P" {
		t.Errorf("RoleMap mismatch: expected MyPara->P, got %v->%v", "MyPara", v)
	}

	if len(parsedDoc.StructTree.K) != 1 {
		t.Fatalf("Expected 1 root kid, got %d", len(parsedDoc.StructTree.K))
	}

	elem := parsedDoc.StructTree.K[0]
	if elem.S != "MyPara" {
		t.Errorf("Expected element type MyPara, got %s", elem.S)
	}
	if elem.Title != "Paragraph 1" {
		t.Errorf("Expected title 'Paragraph 1', got '%s'", elem.Title)
	}

	if len(elem.K) != 1 {
		t.Fatalf("Expected 1 kid in element, got %d", len(elem.K))
	}

	// Check MCID
	kid := elem.K[0]
	// writer/helpers.go writes MCR dictionary for MCID.
	// parser/structure_parser.go parses MCR dictionary into StructureItem{MCR: ...}
	if kid.MCR == nil {
		t.Error("Expected MCR kid, got nil")
	} else {
		if kid.MCR.MCID != 0 {
			t.Errorf("Expected MCID 0, got %d", kid.MCR.MCID)
		}
	}
}
