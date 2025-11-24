package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/optimize"
	"github.com/wudi/pdfkit/parser"
)

func TestWriter_Optimization(t *testing.T) {
	// Create two identical images
	data := []byte{1, 2, 3, 4}
	img1 := semantic.XObject{
		Subtype:          "Image",
		Width:            2,
		Height:           2,
		BitsPerComponent: 8,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
		Data:             data,
	}
	img2 := semantic.XObject{
		Subtype:          "Image",
		Width:            2,
		Height:           2,
		BitsPerComponent: 8,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
		Data:             data,
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": img1,
						"Im2": img2,
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	opt := optimize.New(optimize.Config{
		CombineIdenticalIndirectObjects: true,
	})
	cfg := Config{
		Deterministic: true,
		Optimizer:     opt,
	}

	if err := w.Write(staticCtx{}, doc, &buf, cfg); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse and check if images are shared
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	// Find the page resources
	var resDict *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if tval, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := tval.(raw.NameObj); ok && n.Value() == "Page" {
					if res, ok := d.Get(raw.NameLiteral("Resources")); ok {
						resDict = res.(*raw.DictObj)
					}
				}
			}
		}
	}

	if resDict == nil {
		t.Fatalf("resources not found")
	}

	xobjDict := resDict.KV["XObject"].(*raw.DictObj)
	ref1 := xobjDict.KV["Im1"].(raw.RefObj)
	ref2 := xobjDict.KV["Im2"].(raw.RefObj)

	if ref1.Ref().Num != ref2.Ref().Num {
		t.Errorf("expected identical images to be combined, got %v and %v", ref1, ref2)
	}
}
