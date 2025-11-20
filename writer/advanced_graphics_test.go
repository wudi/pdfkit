package writer

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"testing"
)

func TestAdvancedGraphicsSerialization(t *testing.T) {
	// Setup
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					ExtGStates: map[string]semantic.ExtGState{
						"GS1": {
							BlendMode:     "Multiply",
							StrokeAlpha:   agFloat64Ptr(0.5),
							FillAlpha:     agFloat64Ptr(0.5),
							AlphaSource:   agBoolPtr(true),
							TextKnockout:  agBoolPtr(false),
							Overprint:     agBoolPtr(true),
							OverprintFill: agBoolPtr(false),
							OverprintMode: agIntPtr(1),
						},
						"GS2": {
							SoftMask: &semantic.SoftMaskDict{
								Subtype: "Alpha",
								Group: &semantic.XObject{
									Subtype: "Form",
									BBox:    semantic.Rectangle{URX: 100, URY: 100},
									Group: &semantic.TransparencyGroup{
										Isolated: true,
										Knockout: false,
									},
								},
								BackdropColor: []float64{1.0, 1.0, 1.0},
								Transfer:      "Identity",
							},
						},
					},
					XObjects: map[string]semantic.XObject{
						"Form1": {
							Subtype: "Form",
							BBox:    semantic.Rectangle{URX: 100, URY: 100},
							Group: &semantic.TransparencyGroup{
								Isolated: true,
								Knockout: true,
								CS:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
							},
							Data: []byte("..."),
						},
					},
				},
			},
		},
	}

	// Build
	builder := newObjectBuilder(doc, Config{}, 1, nil, nil, nil)
	objects, _, _, _, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify ExtGState GS1
	foundGS1 := false
	for _, obj := range objects {
		if dict, ok := obj.(*raw.DictObj); ok {
			// Look for ExtGState dictionary inside Resources (or the page object itself if flattened? No, Resources is a dict)
			// Actually, the builder creates separate objects for pages, but Resources are embedded in the Page dict usually,
			// unless they are indirect. In object_builder.go, Resources are built inline into the Page dict.
			// Wait, ExtGState entries are inline dictionaries inside the ExtGState resource dictionary.
			
			// Let's find the Page object first.
			if typeName, ok := dict.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typeName.(raw.NameObj); ok && name.Val == "Page" {
					res, ok := dict.Get(raw.NameLiteral("Resources"))
					if !ok {
						continue
					}
					resDict, ok := res.(*raw.DictObj)
					if !ok {
						continue
					}
					extG, ok := resDict.Get(raw.NameLiteral("ExtGState"))
					if !ok {
						continue
					}
					extGDict, ok := extG.(*raw.DictObj)
					if !ok {
						continue
					}
					
					// Check GS1
					if gs1, ok := extGDict.Get(raw.NameLiteral("GS1")); ok {
						gs1Dict := gs1.(*raw.DictObj)
						foundGS1 = true
						checkEntry(t, gs1Dict, "BM", "Multiply")
						checkEntry(t, gs1Dict, "CA", 0.5)
						checkEntry(t, gs1Dict, "ca", 0.5)
						checkEntry(t, gs1Dict, "AIS", true)
						checkEntry(t, gs1Dict, "TK", false)
						checkEntry(t, gs1Dict, "OP", true)
						checkEntry(t, gs1Dict, "op", false)
						checkEntry(t, gs1Dict, "OPM", 1)
					}

					// Check GS2 (SoftMask)
					if gs2, ok := extGDict.Get(raw.NameLiteral("GS2")); ok {
						gs2Dict := gs2.(*raw.DictObj)
						if sm, ok := gs2Dict.Get(raw.NameLiteral("SMask")); ok {
							smDict := sm.(*raw.DictObj)
							checkEntry(t, smDict, "S", "Alpha")
							checkEntry(t, smDict, "TR", "Identity")
							// Check Group reference
							if _, ok := smDict.Get(raw.NameLiteral("G")); !ok {
								t.Error("GS2 SMask missing Group reference")
							}
						} else {
							t.Error("GS2 missing SMask entry")
						}
					}
				}
			}
		}
	}
	if !foundGS1 {
		t.Error("GS1 ExtGState not found in Page Resources")
	}

	// Verify Form XObject Group
	foundForm := false
	for _, obj := range objects {
		if stream, ok := obj.(*raw.StreamObj); ok {
			dict := stream.Dict
			if subtype, ok := dict.Get(raw.NameLiteral("Subtype")); ok {
				if name, ok := subtype.(raw.NameObj); ok && name.Val == "Form" {
					// Check for Group
					if group, ok := dict.Get(raw.NameLiteral("Group")); ok {
						gDict := group.(*raw.DictObj)
						// We might have multiple Form XObjects (e.g. from SoftMask).
						// We are looking for the one with Knockout=true.
						if k, ok := gDict.Get(raw.NameLiteral("K")); ok {
							if b, ok := k.(raw.BoolObj); ok && b.V {
								foundForm = true
								checkEntry(t, gDict, "S", "Transparency")
								checkEntry(t, gDict, "I", true)
								checkEntry(t, gDict, "K", true)
								checkEntry(t, gDict, "CS", "DeviceRGB")
							}
						}
					}
				}
			}
		}
	}
	if !foundForm {
		t.Error("Form XObject with Group not found")
	}
}

func checkEntry(t *testing.T, dict *raw.DictObj, key string, expected interface{}) {
	val, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		t.Errorf("Missing key %s. Available keys: %v", key, dict.Keys())
		return
	}
	switch v := expected.(type) {
	case string:
		if name, ok := val.(raw.NameObj); ok {
			if name.Val != v {
				t.Errorf("Key %s: expected Name %s, got %s", key, v, name.Val)
			}
		} else if str, ok := val.(raw.StringObj); ok {
			if string(str.Bytes) != v {
				t.Errorf("Key %s: expected String %s, got %s", key, v, string(str.Bytes))
			}
		} else {
			t.Errorf("Key %s: expected string/name, got %T", key, val)
		}
	case float64:
		if num, ok := val.(raw.NumberObj); ok {
			if num.Float() != v {
				t.Errorf("Key %s: expected %f, got %f", key, v, num.Float())
			}
		} else {
			t.Errorf("Key %s: expected number, got %T", key, val)
		}
	case int:
		if num, ok := val.(raw.NumberObj); ok {
			if num.Int() != int64(v) {
				t.Errorf("Key %s: expected %d, got %d", key, v, num.Int())
			}
		} else {
			t.Errorf("Key %s: expected number, got %T", key, val)
		}
	case bool:
		if b, ok := val.(raw.BoolObj); ok {
			if b.V != v {
				t.Errorf("Key %s: expected %v, got %v", key, v, b.V)
			}
		} else {
			t.Errorf("Key %s: expected bool, got %T", key, val)
		}
	}
}

func agFloat64Ptr(v float64) *float64 { return &v }
func agBoolPtr(v bool) *bool          { return &v }
func agIntPtr(v int) *int             { return &v }
