package writer

import (
	"bytes"
	"context"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"testing"
)

func TestWriter_TilingPattern(t *testing.T) {
	// Create a Tiling Pattern with Resources
	pattern := &semantic.TilingPattern{
		BasePattern: semantic.BasePattern{
			Type: 1,
		},
		PaintType:  1, // Colored
		TilingType: 1, // Constant spacing
		BBox:       semantic.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		XStep:      100,
		YStep:      100,
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"F1": {
					Subtype:  "Type1",
					BaseFont: "Helvetica",
				},
			},
		},
		Content: []byte("BT /F1 12 Tf (Pattern) Tj ET"),
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 500, URY: 500},
				Resources: &semantic.Resources{
					Patterns: map[string]semantic.Pattern{
						"P1": pattern,
					},
					ColorSpaces: map[string]semantic.ColorSpace{
						"PatternCS": &semantic.PatternColorSpace{},
					},
				},
				Contents: []semantic.ContentStream{
					{RawBytes: []byte("/PatternCS cs /P1 scn 0 0 500 500 re f")},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Verify output
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	// Find the pattern object
	var patternDict *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if stream, ok := obj.(*raw.StreamObj); ok {
			if typ, ok := stream.Dict.Get(raw.NameLiteral("Type")); ok {
				if n, ok := typ.(raw.NameObj); ok && n.Value() == "Pattern" {
					patternDict = stream.Dict
					break
				}
			}
		}
	}

	if patternDict == nil {
		t.Fatal("Pattern object not found")
	}

	// Check Pattern fields
	if pt, ok := patternDict.Get(raw.NameLiteral("PatternType")); !ok {
		t.Error("Missing PatternType")
	} else if n, ok := pt.(raw.NumberObj); !ok || n.Int() != 1 {
		t.Errorf("PatternType mismatch: %v", pt)
	}

	if pt, ok := patternDict.Get(raw.NameLiteral("PaintType")); !ok {
		t.Error("Missing PaintType")
	} else if n, ok := pt.(raw.NumberObj); !ok || n.Int() != 1 {
		t.Errorf("PaintType mismatch: %v", pt)
	}

	if tt, ok := patternDict.Get(raw.NameLiteral("TilingType")); !ok {
		t.Error("Missing TilingType")
	} else if n, ok := tt.(raw.NumberObj); !ok || n.Int() != 1 {
		t.Errorf("TilingType mismatch: %v", tt)
	}

	if bbox, ok := patternDict.Get(raw.NameLiteral("BBox")); !ok {
		t.Error("Missing BBox")
	} else if arr, ok := bbox.(*raw.ArrayObj); !ok || arr.Len() != 4 {
		t.Errorf("BBox malformed: %v", bbox)
	}

	if xs, ok := patternDict.Get(raw.NameLiteral("XStep")); !ok {
		t.Error("Missing XStep")
	} else if n, ok := xs.(raw.NumberObj); !ok || n.Float() != 100 {
		t.Errorf("XStep mismatch: %v", xs)
	}

	if ys, ok := patternDict.Get(raw.NameLiteral("YStep")); !ok {
		t.Error("Missing YStep")
	} else if n, ok := ys.(raw.NumberObj); !ok || n.Float() != 100 {
		t.Errorf("YStep mismatch: %v", ys)
	}

	// Check Resources
	if res, ok := patternDict.Get(raw.NameLiteral("Resources")); !ok {
		t.Error("Missing Resources in Pattern")
	} else if resDict, ok := res.(*raw.DictObj); !ok {
		t.Errorf("Resources not a dict: %T", res)
	} else {
		if fonts, ok := resDict.Get(raw.NameLiteral("Font")); !ok {
			t.Error("Missing Font in Resources")
		} else if fontDict, ok := fonts.(*raw.DictObj); !ok {
			t.Errorf("Font not a dict: %T", fonts)
		} else {
			if _, ok := fontDict.Get(raw.NameLiteral("F1")); !ok {
				t.Error("Missing F1 in Font resources")
			}
		}
	}
}
