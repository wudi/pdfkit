package writer

import (
	"bytes"
	"context"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"testing"
)

func TestWriter_Actions(t *testing.T) {
	jsAction := semantic.JavaScriptAction{
		JS: "app.alert('Hello World');",
	}

	namedAction := semantic.NamedAction{
		Name: "NextPage",
	}

	launchAction := semantic.LaunchAction{
		File:      "external.pdf",
		NewWindow: boolPtr(true),
	}

	submitFormAction := semantic.SubmitFormAction{
		URL:   "http://example.com/submit",
		Flags: 4, // IncludeNoValueFields
	}

	resetFormAction := semantic.ResetFormAction{
		Fields: []string{"Field1", "Field2"},
		Flags:  0,
	}

	importDataAction := semantic.ImportDataAction{
		File: "data.fdf",
	}

	// Create annotations that use these actions
	annots := []semantic.Annotation{
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 20},
			},
			Action: jsAction,
		},
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 30, URX: 100, URY: 40},
			},
			Action: namedAction,
		},
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 50, URX: 100, URY: 60},
			},
			Action: launchAction,
		},
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 70, URX: 100, URY: 80},
			},
			Action: submitFormAction,
		},
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 90, URX: 100, URY: 100},
			},
			Action: resetFormAction,
		},
		&semantic.LinkAnnotation{
			BaseAnnotation: semantic.BaseAnnotation{
				Subtype: "Link",
				RectVal: semantic.Rectangle{LLX: 10, LLY: 110, URX: 100, URY: 120},
			},
			Action: importDataAction,
		},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox:    semantic.Rectangle{URX: 500, URY: 500},
				Contents:    []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
				Annotations: annots,
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

	foundJS := false
	foundNamed := false
	foundLaunch := false
	foundSubmit := false
	foundReset := false
	foundImport := false

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

		aObj, ok := dict.Get(raw.NameLiteral("A"))
		if !ok {
			continue
		}
		actionDict, ok := aObj.(*raw.DictObj)
		if !ok {
			continue
		}
		sObj, ok := actionDict.Get(raw.NameLiteral("S"))
		if !ok {
			continue
		}
		sName, ok := sObj.(raw.NameObj)
		if !ok {
			continue
		}

		switch sName.Value() {
		case "JavaScript":
			foundJS = true
			if js, ok := actionDict.Get(raw.NameLiteral("JS")); !ok {
				t.Errorf("JavaScript action missing JS")
			} else if s, ok := js.(raw.StringObj); !ok || string(s.Value()) != "app.alert('Hello World');" {
				t.Errorf("JavaScript action JS mismatch: %v", js)
			}
		case "Named":
			foundNamed = true
			if n, ok := actionDict.Get(raw.NameLiteral("N")); !ok {
				t.Errorf("Named action missing N")
			} else if name, ok := n.(raw.NameObj); !ok || name.Value() != "NextPage" {
				t.Errorf("Named action N mismatch: %v", n)
			}
		case "Launch":
			foundLaunch = true
			if f, ok := actionDict.Get(raw.NameLiteral("F")); !ok {
				t.Errorf("Launch action missing F")
			} else if s, ok := f.(raw.StringObj); !ok || string(s.Value()) != "external.pdf" {
				t.Errorf("Launch action F mismatch: %v", f)
			}
			if nw, ok := actionDict.Get(raw.NameLiteral("NewWindow")); !ok {
				t.Errorf("Launch action missing NewWindow")
			} else if b, ok := nw.(raw.BoolObj); !ok || !b.Value() {
				t.Errorf("Launch action NewWindow mismatch: %v", nw)
			}
		case "SubmitForm":
			foundSubmit = true
			if f, ok := actionDict.Get(raw.NameLiteral("F")); !ok {
				t.Errorf("SubmitForm action missing F")
			} else if fDict, ok := f.(*raw.DictObj); !ok {
				t.Errorf("SubmitForm action F is not a dict")
			} else {
				if fs, ok := fDict.Get(raw.NameLiteral("FS")); !ok {
					t.Errorf("SubmitForm action F missing FS")
				} else if n, ok := fs.(raw.NameObj); !ok || n.Value() != "URL" {
					t.Errorf("SubmitForm action F/FS mismatch: %v", fs)
				}
				if url, ok := fDict.Get(raw.NameLiteral("F")); !ok {
					t.Errorf("SubmitForm action F missing F")
				} else if s, ok := url.(raw.StringObj); !ok || string(s.Value()) != "http://example.com/submit" {
					t.Errorf("SubmitForm action F/F mismatch: %v", url)
				}
			}
			if flags, ok := actionDict.Get(raw.NameLiteral("Flags")); !ok {
				t.Errorf("SubmitForm action missing Flags")
			} else if n, ok := flags.(raw.NumberObj); !ok || n.Int() != 4 {
				t.Errorf("SubmitForm action Flags mismatch: %v", flags)
			}
		case "ResetForm":
			foundReset = true
			if fields, ok := actionDict.Get(raw.NameLiteral("Fields")); !ok {
				t.Errorf("ResetForm action missing Fields")
			} else if arr, ok := fields.(*raw.ArrayObj); !ok || arr.Len() != 2 {
				t.Errorf("ResetForm action Fields malformed: %v", fields)
			}
		case "ImportData":
			foundImport = true
			if f, ok := actionDict.Get(raw.NameLiteral("F")); !ok {
				t.Errorf("ImportData action missing F")
			} else if s, ok := f.(raw.StringObj); !ok || string(s.Value()) != "data.fdf" {
				t.Errorf("ImportData action F mismatch: %v", f)
			}
		}
	}

	if !foundJS {
		t.Error("JavaScript action not found")
	}
	if !foundNamed {
		t.Error("Named action not found")
	}
	if !foundLaunch {
		t.Error("Launch action not found")
	}
	if !foundSubmit {
		t.Error("SubmitForm action not found")
	}
	if !foundReset {
		t.Error("ResetForm action not found")
	}
	if !foundImport {
		t.Error("ImportData action not found")
	}
}

func TestWriter_AdvancedActions(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Annotations: []semantic.Annotation{
					&semantic.LinkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Link",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Action: semantic.GoToRAction{
							File:      "external.pdf",
							DestName:  "Chapter1",
							NewWindow: boolPtr(true),
						},
					},
					&semantic.LinkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Link",
							RectVal: semantic.Rectangle{LLX: 60, LLY: 10, URX: 100, URY: 50},
						},
						Action: semantic.HideAction{
							TargetName: "MyField",
							Hide:       true,
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse back
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundGoToR := false
	foundHide := false

	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Annot" {
				if a, ok := d.Get(raw.NameLiteral("A")); ok {
					actionDict, _ := a.(*raw.DictObj)
					if s, ok := actionDict.Get(raw.NameLiteral("S")); ok {
						if name, ok := s.(raw.NameObj); ok {
							if name.Value() == "GoToR" {
								foundGoToR = true
								checkActionString(t, actionDict, "F", "external.pdf")
								checkActionString(t, actionDict, "D", "Chapter1")
								if nw, ok := actionDict.Get(raw.NameLiteral("NewWindow")); ok {
									if b, ok := nw.(raw.BoolObj); ok && b.Value() == true {
										// ok
									} else {
										t.Errorf("GoToR NewWindow mismatch")
									}
								}
							} else if name.Value() == "Hide" {
								foundHide = true
								checkActionString(t, actionDict, "T", "MyField")
								if h, ok := actionDict.Get(raw.NameLiteral("H")); ok {
									if b, ok := h.(raw.BoolObj); ok && b.Value() == true {
										// ok
									} else {
										t.Errorf("Hide H mismatch")
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if !foundGoToR {
		t.Errorf("GoToR action not found")
	}
	if !foundHide {
		t.Errorf("Hide action not found")
	}
}

func checkActionString(t *testing.T, d *raw.DictObj, key, expected string) {
	if val, ok := d.Get(raw.NameLiteral(key)); ok {
		if s, ok := val.(raw.StringObj); ok {
			if string(s.Value()) != expected {
				t.Errorf("Expected %s=%s, got %s", key, expected, s.Value())
			}
		} else if n, ok := val.(raw.NameObj); ok {
			if n.Value() != expected {
				t.Errorf("Expected %s=%s, got %s", key, expected, n.Value())
			}
		} else {
			t.Errorf("Expected %s=%s, got wrong type", key, expected)
		}
	} else {
		t.Errorf("Missing %s", key)
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestWriter_PageTransitionAndMoreActions(t *testing.T) {
	duration := 1.5
	scale := 1.0
	base := true
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Trans: &semantic.Transition{
					Style:     "Split",
					Duration:  &duration,
					Dimension: "H",
					Motion:    "I",
					Direction: 0,
					Scale:     &scale,
					Base:      &base,
				},
				Annotations: []semantic.Annotation{
					&semantic.LinkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Link",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Action: semantic.ThreadAction{
							File: "external.pdf",
							// Thread and Bead refs would be set if we had objects,
							// but here we just test serialization of fields we can set.
						},
					},
					&semantic.ScreenAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Screen",
							RectVal: semantic.Rectangle{LLX: 60, LLY: 10, URX: 100, URY: 50},
						},
						Action: semantic.RichMediaExecuteAction{
							// Command ref would be set
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse back
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundTrans := false
	foundThread := false
	foundRichMedia := false

	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Page" {
				if trans, ok := d.Get(raw.NameLiteral("Trans")); ok {
					foundTrans = true
					transDict, _ := trans.(*raw.DictObj)
					checkActionString(t, transDict, "S", "Split")
					checkActionString(t, transDict, "Dm", "H")
					checkActionString(t, transDict, "M", "I")
					if dur, ok := transDict.Get(raw.NameLiteral("D")); ok {
						if n, ok := dur.(raw.NumberObj); ok && n.Float() == 1.5 {
							// ok
						} else {
							t.Errorf("Trans Duration mismatch")
						}
					}
				}
			} else if n, ok := typ.(raw.NameObj); ok && n.Value() == "Annot" {
				if a, ok := d.Get(raw.NameLiteral("A")); ok {
					actionDict, _ := a.(*raw.DictObj)
					if s, ok := actionDict.Get(raw.NameLiteral("S")); ok {
						if name, ok := s.(raw.NameObj); ok {
							if name.Value() == "Thread" {
								foundThread = true
								checkActionString(t, actionDict, "F", "external.pdf")
							} else if name.Value() == "RichMediaExecute" {
								foundRichMedia = true
							}
						}
					}
				}
			}
		}
	}

	if !foundTrans {
		t.Errorf("Page Transition not found")
	}
	if !foundThread {
		t.Errorf("Thread action not found")
	}
	if !foundRichMedia {
		t.Errorf("RichMediaExecute action not found")
	}
}
