package semantic

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
)

func TestParsePages(t *testing.T) {
	// Construct a mock raw document
	// Root -> Pages -> [Page1]
	// Page1 has Viewports

	page1 := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "Page"},
			"VP": &raw.ArrayObj{
				Items: []raw.Object{
					&raw.DictObj{
						KV: map[string]raw.Object{
							"Type": raw.NameObj{Val: "Viewport"},
							"BBox": &raw.ArrayObj{
								Items: []raw.Object{
									raw.NumberObj{I: 0, IsInt: true},
									raw.NumberObj{I: 0, IsInt: true},
									raw.NumberObj{I: 100, IsInt: true},
									raw.NumberObj{I: 100, IsInt: true},
								},
							},
							"Name": raw.StringObj{Bytes: []byte("MyMap")},
							"Measure": &raw.DictObj{
								KV: map[string]raw.Object{
									"Type":    raw.NameObj{Val: "Measure"},
									"Subtype": raw.NameObj{Val: "GEO"},
									"GCS": &raw.DictObj{
										KV: map[string]raw.Object{
											"Type": raw.NameObj{Val: "GEOGCS"},
											"EPSG": raw.NumberObj{I: 4326, IsInt: true},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pages := &raw.DictObj{
		KV: map[string]raw.Object{
			"Type": raw.NameObj{Val: "Pages"},
			"Kids": &raw.ArrayObj{
				Items: []raw.Object{page1},
			},
			"Count": raw.NumberObj{I: 1, IsInt: true},
		},
	}

	resolver := &mockResolver{}

	parsedPages, err := parsePages(pages, resolver, inheritedPageProps{})
	if err != nil {
		t.Fatalf("parsePages failed: %v", err)
	}

	if len(parsedPages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(parsedPages))
	}

	p := parsedPages[0]
	if len(p.Viewports) != 1 {
		t.Fatalf("expected 1 viewport, got %d", len(p.Viewports))
	}

	vp := p.Viewports[0]
	if vp.Name != "MyMap" {
		t.Errorf("expected viewport name 'MyMap', got '%s'", vp.Name)
	}

	if vp.Measure == nil {
		t.Fatal("expected Measure dict")
	}

	if vp.Measure.Subtype != "GEO" {
		t.Errorf("expected Measure subtype 'GEO', got '%s'", vp.Measure.Subtype)
	}

	if vp.Measure.GCS == nil {
		t.Fatal("expected GCS dict")
	}

	if vp.Measure.GCS.EPSG != 4326 {
		t.Errorf("expected EPSG 4326, got %d", vp.Measure.GCS.EPSG)
	}
}

type mockResolver struct{}

func (r *mockResolver) Resolve(ref raw.ObjectRef) (raw.Object, error) {
	return nil, nil
}
