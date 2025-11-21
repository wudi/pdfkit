package writer

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestSerializeSMask(t *testing.T) {
	// Create a document with an ExtGState containing an SMask
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Resources: &semantic.Resources{
					ExtGStates: map[string]semantic.ExtGState{
						"GS1": {
							SoftMask: &semantic.SoftMaskDict{
								Subtype: "Alpha",
								Group: &semantic.XObject{
									Subtype: "Form",
									BBox:    semantic.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
									Data:    []byte("..."),
								},
								BackdropColor: []float64{0, 0, 0},
								Transfer:      "Identity",
							},
						},
					},
				},
			},
		},
	}

	// Build
	b := newObjectBuilder(doc, Config{}, 1, nil, nil, nil, nil)
	objects, _, _, _, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Find the ExtGState dictionary
	var gsDict raw.Dictionary
	for _, obj := range objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if t, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := t.(raw.NameObj); ok && n.Value() == "Page" {
					// Found page
					if res, ok := d.Get(raw.NameLiteral("Resources")); ok {
						if resDict, ok := res.(*raw.DictObj); ok {
							if ext, ok := resDict.Get(raw.NameLiteral("ExtGState")); ok {
								if extDict, ok := ext.(*raw.DictObj); ok {
									if gs1, ok := extDict.Get(raw.NameLiteral("GS1")); ok {
										if gs1Dict, ok := gs1.(*raw.DictObj); ok {
											if sm, ok := gs1Dict.Get(raw.NameLiteral("SMask")); ok {
												if smDict, ok := sm.(*raw.DictObj); ok {
													gsDict = smDict
													break
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if gsDict == nil {
		t.Fatalf("SMask dictionary not found in objects")
	}

	// Verify SMask fields
	if s, ok := gsDict.Get(raw.NameLiteral("S")); !ok {
		t.Errorf("missing S entry")
	} else if name, ok := s.(raw.NameObj); !ok || name.Value() != "Alpha" {
		t.Errorf("expected S=Alpha, got %v", s)
	}

	if g, ok := gsDict.Get(raw.NameLiteral("G")); !ok {
		t.Errorf("missing G entry")
	} else if _, ok := g.(raw.RefObj); !ok {
		t.Errorf("expected G to be a reference, got %T", g)
	}

	if bc, ok := gsDict.Get(raw.NameLiteral("BC")); !ok {
		t.Errorf("missing BC entry")
	} else if arr, ok := bc.(*raw.ArrayObj); !ok || arr.Len() != 3 {
		t.Errorf("expected BC to be array of 3, got %v", bc)
	}

	if tr, ok := gsDict.Get(raw.NameLiteral("TR")); !ok {
		t.Errorf("missing TR entry")
	} else if name, ok := tr.(raw.NameObj); !ok || name.Value() != "Identity" {
		t.Errorf("expected TR=Identity, got %v", tr)
	}
}
