package builder

import (
	"bytes"
	"context"
	"testing"

	"pdflib/contentstream"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/security"
	"pdflib/writer"
)

func TestBuilder_DrawTextPopulatesResourcesAndOps(t *testing.T) {
	b := NewBuilder()
	font := &semantic.Font{BaseFont: "Helvetica-Bold"}
	b.RegisterFont("Body", font)

	b.NewPage(200, 200).
		DrawText("Hello", 10, 20, TextOptions{
			Font:         "Body",
			FontSize:     16,
			Color:        Color{R: 0.1, G: 0.2, B: 0.3},
			RenderMode:   contentstream.TextFillStroke,
			CharSpacing:  0.5,
			WordSpacing:  1,
			HorizScaling: 110,
			Rise:         2,
		}).
		Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("expected one page, got %d", len(doc.Pages))
	}
	page := doc.Pages[0]
	if got := page.Index; got != 0 {
		t.Fatalf("unexpected page index: %d", got)
	}
	if page.Resources == nil || page.Resources.Fonts["Body"] != font {
		t.Fatalf("font not registered on page resources")
	}
	if len(page.Contents) != 1 {
		t.Fatalf("expected one content stream")
	}
	ops := page.Contents[0].Operations
	expectOperators := []string{"BT", "Tf", "Tc", "Tw", "Tz", "Ts", "Tr", "Tm", "rg", "RG", "Tj", "ET"}
	if len(ops) < len(expectOperators) {
		t.Fatalf("missing operations, got %d want >=%d", len(ops), len(expectOperators))
	}
	for i, op := range expectOperators {
		if ops[i].Operator != op {
			t.Fatalf("operation %d = %s, want %s", i, ops[i].Operator, op)
		}
	}
	if nameOp, ok := ops[1].Operands[0].(semantic.NameOperand); !ok || nameOp.Value != "Body" {
		t.Fatalf("Tf not set to Body font")
	}
	if tm := ops[7].Operands; len(tm) == 6 {
		if tm[4].(semantic.NumberOperand).Value != 10 || tm[5].(semantic.NumberOperand).Value != 20 {
			t.Fatalf("Tm coordinates not set: %+v", tm)
		}
	}
	if tj := ops[10].Operands[0].(semantic.StringOperand); string(tj.Value) != "Hello" {
		t.Fatalf("Tj text mismatch: %q", tj.Value)
	}
}

func TestBuilder_DrawShapesAndImages(t *testing.T) {
	b := NewBuilder()
	p := contentstream.Path{
		Subpaths: []contentstream.Subpath{
			{
				Points: []contentstream.PathPoint{
					{Type: contentstream.PathMoveTo, X: 0, Y: 0},
					{Type: contentstream.PathLineTo, X: 10, Y: 0},
					{Type: contentstream.PathLineTo, X: 10, Y: 10},
				},
				Closed: true,
			},
		},
	}
	img := &semantic.Image{
		Width:            2,
		Height:           3,
		ColorSpace:       semantic.ColorSpace{Name: "DeviceGray"},
		BitsPerComponent: 8,
		Data:             []byte{0x00, 0xFF},
	}
	b.NewPage(100, 100).
		DrawRectangle(10, 20, 30, 40, RectOptions{Fill: true, Stroke: true, FillColor: Color{R: 1}, StrokeColor: Color{B: 1}, LineWidth: 2}).
		DrawLine(0, 0, 5, 5, LineOptions{StrokeColor: Color{G: 1}, LineWidth: 1.5, LineCap: contentstream.LineCapRound, DashPattern: []float64{3, 1}}).
		DrawPath(&p, PathOptions{Stroke: true, StrokeColor: Color{R: 0.5}, LineJoin: contentstream.LineJoinRound}).
		DrawImage(img, 5, 5, 0, 0, ImageOptions{Interpolate: true}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	page := doc.Pages[0]
	if len(page.Contents) == 0 {
		t.Fatalf("content stream missing")
	}
	foundDo := false
	foundRect := false
	for _, op := range page.Contents[0].Operations {
		switch op.Operator {
		case "re":
			foundRect = true
		case "Do":
			foundDo = true
		}
	}
	if !foundRect {
		t.Fatalf("rectangle operation not found")
	}
	if !foundDo {
		t.Fatalf("image Do operator missing")
	}
	if page.Resources == nil || len(page.Resources.XObjects) != 1 {
		t.Fatalf("expected image registered in resources, got %+v", page.Resources)
	}
	for name, xo := range page.Resources.XObjects {
		if xo.Subtype != "Image" || !xo.Interpolate {
			t.Fatalf("xobject %s missing expected image attributes", name)
		}
	}
}

func TestBuilder_PageBoxesAnnotations(t *testing.T) {
	ann := &semantic.Annotation{Subtype: "Link", URI: "https://example.com"}
	b := NewBuilder()
	box := semantic.Rectangle{0, 0, 25, 30}
	b.NewPage(50, 60).
		SetMediaBox(box).
		SetCropBox(box).
		SetRotation(450).
		AddAnnotation(ann).
		Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	page := doc.Pages[0]
	if page.MediaBox != box || page.CropBox != box {
		t.Fatalf("boxes not set on page: %+v %+v", page.MediaBox, page.CropBox)
	}
	if page.Rotate != 90 {
		t.Fatalf("rotation not normalized: %d", page.Rotate)
	}
	if len(page.Annotations) != 1 || page.Annotations[0].URI != "https://example.com" {
		t.Fatalf("annotation not added correctly: %+v", page.Annotations)
	}
}

func TestBuilder_InvoiceA4(t *testing.T) {
	b := NewBuilder()
	b.NewPage(595, 842).
		DrawText("Invoice #1234", 50, 800, TextOptions{FontSize: 24, Color: Color{R: 0.1, G: 0.1, B: 0.1}}).
		DrawText("Bill To:", 50, 770, TextOptions{FontSize: 12}).
		DrawLine(50, 760, 545, 760, LineOptions{StrokeColor: Color{B: 0.5}, LineWidth: 1}).
		DrawRectangle(50, 600, 495, 120, RectOptions{Stroke: true, Fill: false, LineWidth: 1}).
		DrawText("Item", 60, 700, TextOptions{FontSize: 10}).
		DrawText("Qty", 300, 700, TextOptions{FontSize: 10}).
		DrawText("Amount", 450, 700, TextOptions{FontSize: 10}).
		DrawText("Subtotal: $100.00", 400, 620, TextOptions{FontSize: 12}).
		DrawText("Total: $100.00", 400, 600, TextOptions{FontSize: 14, RenderMode: contentstream.TextFillStroke}).
		Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("expected single page invoice")
	}
	if doc.Pages[0].MediaBox != (semantic.Rectangle{0, 0, 595, 842}) {
		t.Fatalf("unexpected media box: %+v", doc.Pages[0].MediaBox)
	}
	// Serialize with writer to validate output.
	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	cfg := writer.Config{Deterministic: true, ContentFilter: writer.FilterNone}
	if err := w.Write(context.Background(), doc, &buf, cfg); err != nil {
		t.Fatalf("write invoice: %v", err)
	}
	data := buf.Bytes()
	if !bytes.Contains(data, []byte("Invoice #1234")) {
		t.Fatalf("invoice text missing in output")
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse invoice: %v", err)
	}
	foundMediaBox := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if tObj, ok := dict.Get(raw.NameLiteral("Type")); ok {
			if name, ok := tObj.(raw.NameObj); ok && name.Value() == "Page" {
				mbObj, ok := dict.Get(raw.NameLiteral("MediaBox"))
				if !ok {
					continue
				}
				arr, ok := mbObj.(*raw.ArrayObj)
				if !ok || arr.Len() != 4 {
					t.Fatalf("mediabox not array: %T", mbObj)
				}
				vals := make([]float64, 0, 4)
				for i := 0; i < 4; i++ {
					val, ok := numberValue(arr.Items[i])
					if ok {
						vals = append(vals, val)
					}
				}
				if len(vals) == 4 && vals[2] == 595 && vals[3] == 842 {
					foundMediaBox = true
					break
				}
			}
		}
	}
	if !foundMediaBox {
		t.Fatalf("A4 media box not found in raw output")
	}
}

func numberValue(obj raw.Object) (float64, bool) {
	switch v := obj.(type) {
	case raw.NumberObj:
		return v.Float(), true
	}
	return 0, false
}
