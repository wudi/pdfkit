package writer

import (
	"bytes"
	"context"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"testing"
)

func TestWriter_NewAnnotations(t *testing.T) {
	textAnnot := &semantic.TextAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype:  "Text",
			RectVal:  semantic.Rectangle{LLX: 100, LLY: 100, URX: 120, URY: 120},
			Contents: "This is a sticky note",
			Color:    []float64{1, 1, 0},
		},
		Open: true,
		Icon: "Comment",
	}

	highlightAnnot := &semantic.HighlightAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype:  "Highlight",
			RectVal:  semantic.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 70},
			Contents: "Highlighted text",
			Color:    []float64{1, 0, 0},
		},
		QuadPoints: []float64{50, 70, 200, 70, 50, 50, 200, 50},
	}

	underlineAnnot := &semantic.UnderlineAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype: "Underline",
			RectVal: semantic.Rectangle{LLX: 50, LLY: 30, URX: 200, URY: 40},
			Color:   []float64{0, 1, 0},
		},
		QuadPoints: []float64{50, 40, 200, 40, 50, 30, 200, 30},
	}

	freeTextAnnot := &semantic.FreeTextAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype:  "FreeText",
			RectVal:  semantic.Rectangle{LLX: 300, LLY: 300, URX: 400, URY: 350},
			Contents: "Free Text",
		},
		DA: "0 g /Helv 12 Tf",
		Q:  1,
	}

	lineAnnot := &semantic.LineAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype: "Line",
			RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 100},
		},
		L:  []float64{10, 10, 100, 100},
		LE: []string{"Square", "OpenArrow"},
		IC: []float64{0, 0, 1},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox:    semantic.Rectangle{URX: 500, URY: 500},
				Contents:    []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
				Annotations: []semantic.Annotation{textAnnot, highlightAnnot, underlineAnnot, freeTextAnnot, lineAnnot},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundText := false
	foundHighlight := false

	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := dict.Get(raw.NameLiteral("Type")); !ok {
			continue
		} else if n, ok := typ.(raw.NameObj); !ok || n.Value() != "Annot" {
			continue
		}

		subtypeObj, ok := dict.Get(raw.NameLiteral("Subtype"))
		if !ok {
			continue
		}
		subtype, ok := subtypeObj.(raw.NameObj)
		if !ok {
			continue
		}

		if subtype.Value() == "Text" {
			foundText = true
			if open, ok := dict.Get(raw.NameLiteral("Open")); !ok {
				t.Errorf("Text annotation missing Open")
			} else if b, ok := open.(raw.BoolObj); !ok || !b.Value() {
				t.Errorf("Text annotation Open not true")
			}
			if name, ok := dict.Get(raw.NameLiteral("Name")); !ok {
				t.Errorf("Text annotation missing Name (Icon)")
			} else if n, ok := name.(raw.NameObj); !ok || n.Value() != "Comment" {
				t.Errorf("Text annotation Name mismatch: %v", name)
			}
			if contents, ok := dict.Get(raw.NameLiteral("Contents")); !ok {
				t.Errorf("Text annotation missing Contents")
			} else if s, ok := contents.(raw.StringObj); !ok || string(s.Value()) != "This is a sticky note" {
				t.Errorf("Text annotation Contents mismatch: %v", contents)
			}
		} else if subtype.Value() == "Highlight" {
			foundHighlight = true
			if qp, ok := dict.Get(raw.NameLiteral("QuadPoints")); !ok {
				t.Errorf("Highlight annotation missing QuadPoints")
			} else if arr, ok := qp.(*raw.ArrayObj); !ok || arr.Len() != 8 {
				t.Errorf("Highlight annotation QuadPoints malformed: %v", qp)
			}
			if contents, ok := dict.Get(raw.NameLiteral("Contents")); !ok {
				t.Errorf("Highlight annotation missing Contents")
			} else if s, ok := contents.(raw.StringObj); !ok || string(s.Value()) != "Highlighted text" {
				t.Errorf("Highlight annotation Contents mismatch: %v", contents)
			}
		} else if subtype.Value() == "Underline" {
			if qp, ok := dict.Get(raw.NameLiteral("QuadPoints")); !ok {
				t.Errorf("Underline annotation missing QuadPoints")
			} else if arr, ok := qp.(*raw.ArrayObj); !ok || arr.Len() != 8 {
				t.Errorf("Underline annotation QuadPoints malformed: %v", qp)
			}
		} else if subtype.Value() == "FreeText" {
			if da, ok := dict.Get(raw.NameLiteral("DA")); !ok {
				t.Errorf("FreeText annotation missing DA")
			} else if s, ok := da.(raw.StringObj); !ok || string(s.Value()) != "0 g /Helv 12 Tf" {
				t.Errorf("FreeText annotation DA mismatch: %v", da)
			}
			if q, ok := dict.Get(raw.NameLiteral("Q")); !ok {
				t.Errorf("FreeText annotation missing Q")
			} else if n, ok := q.(raw.NumberObj); !ok || n.Int() != 1 {
				t.Errorf("FreeText annotation Q mismatch: %v", q)
			}
		} else if subtype.Value() == "Line" {
			if l, ok := dict.Get(raw.NameLiteral("L")); !ok {
				t.Errorf("Line annotation missing L")
			} else if arr, ok := l.(*raw.ArrayObj); !ok || arr.Len() != 4 {
				t.Errorf("Line annotation L malformed: %v", l)
			}
			if le, ok := dict.Get(raw.NameLiteral("LE")); !ok {
				t.Errorf("Line annotation missing LE")
			} else if arr, ok := le.(*raw.ArrayObj); !ok || arr.Len() != 2 {
				t.Errorf("Line annotation LE malformed: %v", le)
			}
			if ic, ok := dict.Get(raw.NameLiteral("IC")); !ok {
				t.Errorf("Line annotation missing IC")
			} else if arr, ok := ic.(*raw.ArrayObj); !ok || arr.Len() != 3 {
				t.Errorf("Line annotation IC malformed: %v", ic)
			}
		}
	}

	if !foundText {
		t.Error("Text annotation not found in output")
	}
	if !foundHighlight {
		t.Error("Highlight annotation not found in output")
	}
}
