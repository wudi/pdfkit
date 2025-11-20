package writer

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"testing"
)

func TestRemainingAnnotationsSerialization(t *testing.T) {
	// Create a parent annotation for Popup
	parentAnnot := &semantic.TextAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype: "Text",
			RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
		},
	}
	// We need to manually set the ref for the parent since we are mocking the build process
	// In a real build, the builder assigns refs.
	// Here we can't easily set the ref on the struct because it's private or handled by the builder.
	// However, the serializer calls t.Parent.Reference().
	// Let's see if we can mock it or if we need to rely on the builder to set it.
	// The builder sets the ref on the annotation struct before calling Serialize?
	// No, Serialize returns the ref.
	// But for Popup, we need the parent's ref.
	// Let's skip Popup parent check for now or try to hack it if possible.
	// Actually, semantic.BaseAnnotation has SetReference.

	parentRef := raw.ObjectRef{Num: 999, Gen: 0}
	parentAnnot.SetReference(parentRef)

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Annotations: []semantic.Annotation{
					parentAnnot,
					&semantic.PopupAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Popup",
							RectVal: semantic.Rectangle{LLX: 60, LLY: 60, URX: 100, URY: 100},
						},
						Parent: parentAnnot,
						Open:   true,
					},
					&semantic.SoundAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Sound",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Name: "Speaker",
						Sound: semantic.EmbeddedFile{
							Data: []byte("sound data"),
						},
					},
					&semantic.MovieAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Movie",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Title: "My Movie",
						Movie: semantic.EmbeddedFile{
							Name: "movie.mp4",
							Data: []byte("movie data"),
						},
					},
					&semantic.ScreenAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Screen",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Title: "My Screen",
						Action: &semantic.JavaScriptAction{
							JS: "app.alert('Hello');",
						},
					},
					&semantic.PrinterMarkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "PrinterMark",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
					},
					&semantic.TrapNetAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "TrapNet",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						LastModified: "D:20230101000000Z",
						Version:      []int{1, 0},
					},
					&semantic.WatermarkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Watermark",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						FixedPrint: true,
					},
					&semantic.ThreeDAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "3D",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						ThreeD: semantic.EmbeddedFile{
							Data: []byte("3d data"),
						},
						View: "DefaultView",
					},
					&semantic.RedactAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Redact",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						OverlayText: "REDACTED",
						Repeat:      []float64{1, 2},
					},
					&semantic.ProjectionAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Projection",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						ProjectionType: "P",
					},
				},
			},
		},
	}

	builder := newObjectBuilder(doc, Config{}, 1, nil, nil, nil, nil)
	objects, _, _, _, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	foundPopup := false
	foundSound := false
	foundMovie := false
	foundScreen := false
	foundPrinterMark := false
	foundTrapNet := false
	foundWatermark := false
	found3D := false
	foundRedact := false
	foundProjection := false

	for _, obj := range objects {
		if dict, ok := obj.(*raw.DictObj); ok {
			if typeName, ok := dict.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typeName.(raw.NameObj); ok && name.Val == "Annot" {
					subtype, _ := dict.Get(raw.NameLiteral("Subtype"))
					subName, _ := subtype.(raw.NameObj)

					switch subName.Val {
					case "Popup":
						foundPopup = true
						checkEntry(t, dict, "Open", true)
						if parent, ok := dict.Get(raw.NameLiteral("Parent")); ok {
							if ref, ok := parent.(raw.RefObj); ok {
								if ref.Ref().Num != 999 {
									t.Errorf("Popup Parent ref mismatch: got %d, want 999", ref.Ref().Num)
								}
							} else {
								t.Error("Popup Parent is not a reference")
							}
						} else {
							t.Error("Popup missing Parent")
						}
					case "Sound":
						foundSound = true
						checkEntry(t, dict, "Name", "Speaker")
						if _, ok := dict.Get(raw.NameLiteral("Sound")); !ok {
							t.Error("Sound missing Sound stream ref")
						}
					case "Movie":
						foundMovie = true
						checkEntry(t, dict, "T", "My Movie")
						if _, ok := dict.Get(raw.NameLiteral("Movie")); !ok {
							t.Error("Movie missing Movie dict")
						}
					case "Screen":
						foundScreen = true
						checkEntry(t, dict, "T", "My Screen")
						if _, ok := dict.Get(raw.NameLiteral("A")); !ok {
							t.Error("Screen missing Action")
						}
					case "PrinterMark":
						foundPrinterMark = true
					case "TrapNet":
						foundTrapNet = true
						checkEntry(t, dict, "LastModified", "D:20230101000000Z")
						if ver, ok := dict.Get(raw.NameLiteral("Version")); ok {
							arr := ver.(*raw.ArrayObj)
							if arr.Len() != 2 {
								t.Errorf("TrapNet Version length mismatch")
							}
						} else {
							t.Error("TrapNet missing Version")
						}
					case "Watermark":
						foundWatermark = true
						checkEntry(t, dict, "FixedPrint", true)
					case "3D":
						found3D = true
						if _, ok := dict.Get(raw.NameLiteral("3DD")); !ok {
							t.Error("3D missing 3DD stream ref")
						}
						if view, ok := dict.Get(raw.NameLiteral("3DV")); ok {
							vDict := view.(*raw.DictObj)
							checkEntry(t, vDict, "XN", "DefaultView")
						} else {
							t.Error("3D missing 3DV")
						}
					case "Redact":
						foundRedact = true
						checkEntry(t, dict, "OverlayText", "REDACTED")
						if rep, ok := dict.Get(raw.NameLiteral("Repeat")); ok {
							arr := rep.(*raw.ArrayObj)
							if arr.Len() != 2 {
								t.Errorf("Redact Repeat length mismatch")
							}
						} else {
							t.Error("Redact missing Repeat")
						}
					case "Projection":
						foundProjection = true
						checkEntry(t, dict, "ProjectionType", "P")
					}
				}
			}
		}
	}

	if !foundPopup {
		t.Error("Popup annotation not found")
	}
	if !foundSound {
		t.Error("Sound annotation not found")
	}
	if !foundMovie {
		t.Error("Movie annotation not found")
	}
	if !foundScreen {
		t.Error("Screen annotation not found")
	}
	if !foundPrinterMark {
		t.Error("PrinterMark annotation not found")
	}
	if !foundTrapNet {
		t.Error("TrapNet annotation not found")
	}
	if !foundWatermark {
		t.Error("Watermark annotation not found")
	}
	if !found3D {
		t.Error("3D annotation not found")
	}
	if !foundRedact {
		t.Error("Redact annotation not found")
	}
	if !foundProjection {
		t.Error("Projection annotation not found")
	}
}
