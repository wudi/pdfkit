package writer

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"testing"
)

func TestMoreAnnotationsSerialization(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Annotations: []semantic.Annotation{
					&semantic.StampAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Stamp",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Name: "Approved",
					},
					&semantic.InkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Ink",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						InkList: [][]float64{{10, 10, 20, 20}, {30, 30, 40, 40}},
					},
					&semantic.FileAttachmentAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "FileAttachment",
							RectVal: semantic.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50},
						},
						Name: "Paperclip",
						File: semantic.EmbeddedFile{
							Name: "test.txt",
							Data: []byte("Hello World"),
						},
					},
				},
			},
		},
	}

	builder := newObjectBuilder(doc, Config{}, 1, nil, nil, nil)
	objects, _, _, _, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	foundStamp := false
	foundInk := false
	foundFile := false

	for _, obj := range objects {
		if dict, ok := obj.(*raw.DictObj); ok {
			if typeName, ok := dict.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typeName.(raw.NameObj); ok && name.Val == "Annot" {
					subtype, _ := dict.Get(raw.NameLiteral("Subtype"))
					subName, _ := subtype.(raw.NameObj)

					if subName.Val == "Stamp" {
						foundStamp = true
						checkEntry(t, dict, "Name", "Approved")
					} else if subName.Val == "Ink" {
						foundInk = true
						if inkList, ok := dict.Get(raw.NameLiteral("InkList")); ok {
							arr := inkList.(*raw.ArrayObj)
							if arr.Len() != 2 {
								t.Errorf("InkList length mismatch: got %d, want 2", arr.Len())
							}
						} else {
							t.Error("Missing InkList")
						}
					} else if subName.Val == "FileAttachment" {
						foundFile = true
						checkEntry(t, dict, "Name", "Paperclip")
						if fs, ok := dict.Get(raw.NameLiteral("FS")); ok {
							fsDict := fs.(*raw.DictObj)
							checkEntry(t, fsDict, "Type", "Filespec")
							checkEntry(t, fsDict, "F", "test.txt")
						} else {
							t.Error("Missing FS")
						}
					}
				}
			}
		}
	}

	if !foundStamp {
		t.Error("Stamp annotation not found")
	}
	if !foundInk {
		t.Error("Ink annotation not found")
	}
	if !foundFile {
		t.Error("FileAttachment annotation not found")
	}
}
