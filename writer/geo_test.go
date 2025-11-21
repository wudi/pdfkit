package writer

import (
	"bytes"
	"context"
	"testing"

	"pdflib/geo"
	"pdflib/ir/decoded"
	"pdflib/ir/semantic"
	"pdflib/parser"
)

func TestGeoPDF(t *testing.T) {
	// Create a document with GeoPDF metadata
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200},
				Viewports: []geo.Viewport{
					{
						Name: "Map1",
						BBox: []float64{0, 0, 200, 200},
						Measure: &geo.Measure{
							Subtype: "GEO",
							GCS: &geo.CoordinateSystem{
								Type: "GEOGCS",
								EPSG: 4326,
								WKT:  "GEOGCS[\"WGS 84\",...]",
							},
							GPTS: []float64{0, 0, 0, 1, 1, 1, 1, 0},
							LPTS: []float64{0, 0, 0, 200, 200, 200, 200, 0},
						},
					},
				},
			},
		},
	}

	// Write to buffer
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Parse back
	rawDoc, err := parser.NewDocumentParser(parser.Config{}).Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	decDoc := &decoded.DecodedDocument{
		Raw: rawDoc,
	}

	// Build semantic model
	builder := semantic.NewBuilder()
	semDoc, err := builder.Build(context.Background(), decDoc)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(semDoc.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(semDoc.Pages))
	}

	p := semDoc.Pages[0]
	if len(p.Viewports) != 1 {
		t.Fatalf("expected 1 viewport, got %d", len(p.Viewports))
	}

	vp := p.Viewports[0]
	if vp.Name != "Map1" {
		t.Errorf("expected viewport name 'Map1', got '%s'", vp.Name)
	}

	if vp.Measure == nil {
		t.Fatal("expected Measure")
	}

	if vp.Measure.Subtype != "GEO" {
		t.Errorf("expected subtype GEO, got %s", vp.Measure.Subtype)
	}

	if vp.Measure.GCS.EPSG != 4326 {
		t.Errorf("expected EPSG 4326, got %d", vp.Measure.GCS.EPSG)
	}
}
