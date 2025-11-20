package writer

import (
	"bytes"
	"context"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/ir/raw"
	"testing"
	"strings"
)

func TestWriter_Type3FontAndOCG(t *testing.T) {
	// Define a Type 3 font
	type3Font := &semantic.Font{
		Subtype:    "Type3",
		BaseFont:   "MyType3",
		FontBBox:   semantic.Rectangle{LLX: 0, LLY: 0, URX: 1000, URY: 1000},
		FontMatrix: []float64{0.001, 0, 0, 0.001, 0, 0},
		CharProcs: map[string][]byte{
			"a": []byte("100 0 d0 0 0 1000 1000 re f"),
		},
		Encoding: "WinAnsiEncoding",
		Widths:   map[int]int{97: 1000}, // 'a'
		Resources: &semantic.Resources{}, // Empty resources for the font itself
	}

	// Define OCGs
	ocg1 := &semantic.OptionalContentGroup{
		Name: "Layer 1",
	}
	ocg2 := &semantic.OptionalContentGroup{
		Name: "Layer 2",
	}

	// Define OCMD
	ocmd := &semantic.OptionalContentMembership{
		OCGs:   []*semantic.OptionalContentGroup{ocg1, ocg2},
		Policy: "AnyOn",
	}

	// Page using the font and OCGs
	page := &semantic.Page{
		MediaBox: semantic.Rectangle{URX: 200, URY: 200},
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"T3": type3Font,
			},
			Properties: map[string]semantic.PropertyList{
				"OC1": ocg1,
				"OC2": ocg2,
				"MC1": ocmd,
			},
		},
		Contents: []semantic.ContentStream{
			{RawBytes: []byte("/OC1 BDC /T3 12 Tf (a) Tj EMC")},
		},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{page},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Verify output
	data := buf.Bytes()
	
	// Check for Type 3 font
	if !strings.Contains(string(data), "/Subtype /Type3") {
		t.Fatalf("Type 3 subtype missing")
	}
	// FontMatrix might be serialized with trailing zeros (e.g. 0.001000)
	// Just check for the presence of FontMatrix and the key values
	sData := string(data)
	if !strings.Contains(sData, "/FontMatrix") {
		t.Fatalf("FontMatrix key missing")
	}
	// We expect [0.001 0 0 0.001 0 0] but maybe with extra zeros
	if !strings.Contains(sData, "0.001") {
		t.Fatalf("FontMatrix values missing (expected 0.001)")
	}
	if !strings.Contains(string(data), "/CharProcs") {
		t.Fatalf("CharProcs missing")
	}
	// Check for OCGs
	if !strings.Contains(string(data), "/Type /OCG") {
		t.Fatalf("OCG type missing")
	}
	if !strings.Contains(string(data), "/Name (Layer 1)") {
		t.Fatalf("OCG Layer 1 missing")
	}
	if !strings.Contains(string(data), "/Name (Layer 2)") {
		t.Fatalf("OCG Layer 2 missing")
	}
	// Check for OCMD
	if !strings.Contains(string(data), "/Type /OCMD") {
		t.Fatalf("OCMD type missing")
	}
	if !strings.Contains(string(data), "/P /AnyOn") {
		t.Fatalf("OCMD policy missing")
	}
	if !strings.Contains(string(data), "/OCGs [") {
		t.Fatalf("OCMD OCGs array missing")
	}

	// Parse back to verify structure
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	// Find the font dict
	foundFont := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if tval, ok := dict.Get(raw.NameLiteral("Type")); ok {
			if n, ok := tval.(raw.NameObj); ok && n.Value() == "Font" {
				if s, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
					if n, ok := s.(raw.NameObj); ok && n.Value() == "Type3" {
						foundFont = true
						// Check CharProcs
						if cp, ok := dict.Get(raw.NameLiteral("CharProcs")); ok {
							if cpDict, ok := cp.(*raw.DictObj); ok {
								if _, ok := cpDict.Get(raw.NameLiteral("a")); !ok {
									t.Fatalf("CharProc 'a' missing")
								}
							} else {
								t.Fatalf("CharProcs not a dict")
							}
						} else {
							t.Fatalf("CharProcs missing in font dict")
						}
					}
				}
			}
		}
	}
	if !foundFont {
		t.Fatalf("Type 3 font not found in parsed objects")
	}
}
