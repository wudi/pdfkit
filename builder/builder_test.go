package builder

import (
	"testing"

	"github.com/wudi/pdfkit/contentstream"
	"github.com/wudi/pdfkit/fonts"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"

	"golang.org/x/image/font/gofont/goregular"
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
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
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

func TestBuilder_RegisterTrueTypeFont(t *testing.T) {
	b := NewBuilder()
	b.RegisterTrueTypeFont("Go", goregular.TTF)

	text := "hé"
	b.NewPage(50, 50).DrawText(text, 5, 5, TextOptions{Font: "Go", FontSize: 9}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	page := doc.Pages[0]
	font := page.Resources.Fonts["Go"]
	if font == nil || font.Subtype != "Type0" || font.Encoding != "Identity-H" {
		t.Fatalf("expected embedded Type0 Identity-H font, got %+v", font)
	}
	if len(font.ToUnicode) == 0 {
		t.Fatalf("expected ToUnicode mapping on font")
	}
	if len(page.Contents) != 1 {
		t.Fatalf("content stream missing")
	}
	ops := page.Contents[0].Operations
	if len(ops) == 0 {
		t.Fatalf("no operations found")
	}
	tjOp := ops[len(ops)-2] // Tj before ET
	if tjOp.Operator != "Tj" {
		t.Fatalf("expected Tj operator, got %s", tjOp.Operator)
	}
	encoded := tjOp.Operands[0].(semantic.StringOperand).Value
	refFont, err := fonts.LoadTrueType("Go", goregular.TTF)
	if err != nil {
		t.Fatalf("load reference font: %v", err)
	}
	expected := encodeWithMap(text, runeToCID(refFont))
	if string(encoded) == text {
		t.Fatalf("text was not encoded as CID string")
	}
	if string(encoded) != string(expected) {
		t.Fatalf("encoded text mismatch, got %x want %x", encoded, expected)
	}
}

func encodeWithMap(text string, cmap map[rune]int) []byte {
	if len(cmap) == 0 {
		return []byte(text)
	}
	buf := make([]byte, 0, len(text)*2)
	for _, r := range text {
		cid := cmap[r]
		buf = append(buf, byte(cid>>8), byte(cid))
	}
	return buf
}

func TestBuilder_PageBoxesAnnotations(t *testing.T) {
	ann := &semantic.LinkAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{Subtype: "Link"},
		URI:            "https://example.com",
	}
	b := NewBuilder()
	box := semantic.Rectangle{LLX: 0, LLY: 0, URX: 25, URY: 30}
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
	if len(page.Annotations) != 1 {
		t.Fatalf("annotation not added correctly: %+v", page.Annotations)
	}
	link, ok := page.Annotations[0].(*semantic.LinkAnnotation)
	if !ok || link.URI != "https://example.com" {
		t.Fatalf("annotation not added correctly: %+v", page.Annotations)
	}
}

func TestBuilder_MetadataHelpers(t *testing.T) {
	b := NewBuilder().
		SetLanguage("en-US").
		SetMarked(true).
		AddPageLabel(0, "i.").
		AddPageLabel(1, "A.")
	b.NewPage(10, 10).Finish()
	b.NewPage(10, 10).Finish()

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if doc.Lang != "en-US" {
		t.Fatalf("lang not propagated: %s", doc.Lang)
	}
	if !doc.Marked {
		t.Fatalf("marked flag not set")
	}
	if doc.PageLabels == nil || doc.PageLabels[0] != "i." || doc.PageLabels[1] != "A." {
		t.Fatalf("page labels not set correctly: %+v", doc.PageLabels)
	}
}

func TestBuilder_Outlines(t *testing.T) {
	b := NewBuilder()
	p1 := b.NewPage(100, 100).(*pageBuilderImpl).page
	b.NewPage(100, 100).Finish()
	x := 10.0
	y := 20.0
	zoom := 1.5
	b.AddOutline(Outline{
		Title: "Parent",
		Page:  p1,
		X:     &x,
		Y:     &y,
		Zoom:  &zoom,
		Children: []Outline{
			{Title: "Child", PageIndex: 1},
		},
	})
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if len(doc.Outlines) != 1 {
		t.Fatalf("expected one outline")
	}
	parent := doc.Outlines[0]
	if parent.PageIndex != 0 {
		t.Fatalf("parent page index not resolved: %d", parent.PageIndex)
	}
	if parent.Dest == nil || parent.Dest.X == nil || parent.Dest.Y == nil || parent.Dest.Zoom == nil {
		t.Fatalf("dest not populated: %+v", parent.Dest)
	}
	if *parent.Dest.X != x || *parent.Dest.Y != y || *parent.Dest.Zoom != zoom {
		t.Fatalf("dest values mismatch: %+v", parent.Dest)
	}
	if len(parent.Children) != 1 || parent.Children[0].PageIndex != 1 {
		t.Fatalf("child outline not preserved: %+v", parent.Children)
	}
}

func TestBuilder_SetEncryption(t *testing.T) {
	perms := raw.Permissions{Print: true, Modify: true}
	b := NewBuilder().
		SetEncryption("owner", "user", perms, true)
	b.NewPage(5, 5).Finish()

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if !doc.Encrypted {
		t.Fatalf("encrypted flag not set")
	}
	if doc.OwnerPassword != "owner" || doc.UserPassword != "user" {
		t.Fatalf("passwords not set: %q %q", doc.OwnerPassword, doc.UserPassword)
	}
	if !doc.Permissions.Print || !doc.Permissions.Modify {
		t.Fatalf("permissions not copied: %+v", doc.Permissions)
	}
	if !doc.MetadataEncrypted {
		t.Fatalf("metadata encryption flag not set")
	}
}
