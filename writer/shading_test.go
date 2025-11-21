package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/parser"
)

func TestWriter_ShadingPattern(t *testing.T) {
	// Function Shading (Type 2)
	funcShading := &semantic.FunctionShading{
		BaseShading: semantic.BaseShading{
			Type:       2, // Axial
			ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceRGB"},
			BBox:       semantic.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
			AntiAlias:  true,
		},
		Coords: []float64{0, 0, 100, 100},
		Domain: []float64{0, 1},
		Extend: []bool{true, true},
		Function: []semantic.Function{
			&semantic.ExponentialFunction{
				BaseFunction: semantic.BaseFunction{
					Type:   2,
					Domain: []float64{0, 1},
					Range:  []float64{0, 1, 0, 1, 0, 1},
				},
				C0: []float64{1, 0, 0},
				C1: []float64{0, 0, 1},
				N:  1,
			},
		},
	}

	// Mesh Shading (Type 4)
	meshShading := &semantic.MeshShading{
		BaseShading: semantic.BaseShading{
			Type:       4, // Free-form Gouraud-shaded triangle mesh
			ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceRGB"},
			BBox:       semantic.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200},
			AntiAlias:  false,
		},
		BitsPerCoordinate: 8,
		BitsPerComponent:  8,
		BitsPerFlag:       8,
		Decode:            []float64{0, 200, 0, 200, 0, 1, 0, 1, 0, 1},
		Stream:            []byte{0x00, 0x00, 0xFF}, // Dummy data
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 500, URY: 500},
				Resources: &semantic.Resources{
					Shadings: map[string]semantic.Shading{
						"Sh1": funcShading,
						"Sh2": meshShading,
					},
				},
				Contents: []semantic.ContentStream{
					{RawBytes: []byte("/Sh1 sh /Sh2 sh")},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Verify output
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundSh1 := false
	foundSh2 := false

	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			if stream, ok := obj.(*raw.StreamObj); ok {
				dict = stream.Dict
			} else {
				continue
			}
		}

		// Check for ShadingType
		stObj, ok := dict.Get(raw.NameLiteral("ShadingType"))
		if !ok {
			continue
		}
		st, ok := stObj.(raw.NumberObj)
		if !ok {
			continue
		}

		if st.Int() == 2 {
			foundSh1 = true
			// Check Function Shading fields
			if bbox, ok := dict.Get(raw.NameLiteral("BBox")); !ok {
				t.Error("Sh1 missing BBox")
			} else if arr, ok := bbox.(*raw.ArrayObj); !ok || arr.Len() != 4 {
				t.Errorf("Sh1 BBox malformed: %v", bbox)
			}

			if aa, ok := dict.Get(raw.NameLiteral("AntiAlias")); !ok {
				t.Error("Sh1 missing AntiAlias")
			} else if b, ok := aa.(raw.BoolObj); !ok || !b.Value() {
				t.Errorf("Sh1 AntiAlias mismatch: %v", aa)
			}

			if coords, ok := dict.Get(raw.NameLiteral("Coords")); !ok {
				t.Error("Sh1 missing Coords")
			} else if arr, ok := coords.(*raw.ArrayObj); !ok || arr.Len() != 4 {
				t.Errorf("Sh1 Coords malformed: %v", coords)
			}
		} else if st.Int() == 4 {
			foundSh2 = true
			// Check Mesh Shading fields
			if bbox, ok := dict.Get(raw.NameLiteral("BBox")); !ok {
				t.Error("Sh2 missing BBox")
			} else if arr, ok := bbox.(*raw.ArrayObj); !ok || arr.Len() != 4 {
				t.Errorf("Sh2 BBox malformed: %v", bbox)
			}

			// AntiAlias should be absent or false
			if aa, ok := dict.Get(raw.NameLiteral("AntiAlias")); ok {
				if b, ok := aa.(raw.BoolObj); ok && b.Value() {
					t.Error("Sh2 AntiAlias should be false or absent")
				}
			}

			if bpc, ok := dict.Get(raw.NameLiteral("BitsPerCoordinate")); !ok {
				t.Error("Sh2 missing BitsPerCoordinate")
			} else if n, ok := bpc.(raw.NumberObj); !ok || n.Int() != 8 {
				t.Errorf("Sh2 BitsPerCoordinate mismatch: %v", bpc)
			}
		}
	}

	if !foundSh1 {
		t.Error("Shading Type 2 (Sh1) not found")
	}
	if !foundSh2 {
		t.Error("Shading Type 4 (Sh2) not found")
	}
}
