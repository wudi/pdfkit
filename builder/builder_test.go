package builder

import (
	"testing"

	"pdflib/contentstream"
	"pdflib/ir/semantic"
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
