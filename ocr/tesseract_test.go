package ocr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os/exec"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/extractor"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ensureTesseractAvailable checks that the tesseract binary is reachable.
func ensureTesseractAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract not installed in PATH")
	}
}

func TestTesseractEngineRecognize(t *testing.T) {
	ensureTesseractAvailable(t)

	img := image.NewRGBA(image.Rect(0, 0, 200, 80))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  image.Black,
		Face: basicfont.Face7x13,
		Dot:  fixed.P(10, 50),
	}
	target := "Hello PDF"
	d.DrawString(target)

	var buf bytes.Buffer
	enc := png.Encoder{}
	if err := enc.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	asset := extractor.ImageAsset{
		Page:         0,
		ResourceName: "Im1",
		Width:        img.Bounds().Dx(),
		Height:       img.Bounds().Dy(),
		Data:         img.Pix,
	}

	results, err := DefaultRecognizeAssets(context.Background(), []extractor.ImageAsset{asset}, WithLanguages("eng"), WithDPI(300))
	if err != nil {
		t.Fatalf("DefaultRecognizeAssets() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	got := strings.ToLower(res.PlainText)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "pdf") {
		t.Fatalf("unexpected OCR output: %q", res.PlainText)
	}
	if len(res.Blocks) == 0 || len(res.Blocks[0].Lines) == 0 {
		t.Fatalf("expected structured blocks")
	}
	if res.InputID != "page-0-Im1" {
		t.Fatalf("unexpected input id: %s", res.InputID)
	}
}
