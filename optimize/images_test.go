package optimize

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestOptimizeImages(t *testing.T) {
	// Create a simple 10x10 RGB image
	width, height := 10, 10
	data := make([]byte, width*height*3)
	for i := 0; i < len(data); i += 3 {
		// Red color
		data[i] = 255 // R
		data[i+1] = 0 // G
		data[i+2] = 0 // B
	}

	originalXO := semantic.XObject{
		Subtype:          "Image",
		Width:            width,
		Height:           height,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
		BitsPerComponent: 8,
		Data:             data,
		Filter:           "", // No filter initially
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": originalXO,
					},
				},
			},
		},
	}

	// Configure optimizer to compress images
	config := Config{
		ImageQuality: 75, // Enable JPEG compression
	}
	opt := New(config)

	err := opt.Optimize(context.Background(), doc)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	optimizedXO := doc.Pages[0].Resources.XObjects["Im1"]

	// Check if Filter is set to DCTDecode
	if optimizedXO.Filter != "DCTDecode" {
		t.Errorf("Expected Filter to be DCTDecode, got %q", optimizedXO.Filter)
	}

	// Check if Data size is smaller (JPEG compression should reduce size for this simple image,
	// but for very small images header overhead might make it larger.
	// However, 10x10x3 = 300 bytes. JPEG header is usually larger.
	// Let's just check that Data is different and not empty.
	if len(optimizedXO.Data) == 0 {
		t.Error("Optimized data is empty")
	}
	if string(optimizedXO.Data) == string(originalXO.Data) {
		t.Error("Data was not modified")
	}

	// Verify we can decode it back to an image (sanity check)
	_, err = toImage(optimizedXO)
	// toImage currently only supports raw buffers for DeviceGray/RGB/CMYK,
	// it doesn't support decoding DCTDecode yet in the test helper `toImage` inside `images.go`.
	// Wait, `toImage` in `images.go` takes `xo.Data` and interprets it based on ColorSpace.
	// If `xo.Filter` is set, `toImage` as written in `images.go` does NOT decode the filter.
	// It assumes `xo.Data` is raw pixels if it tries to read it as Gray/RGB/CMYK.

	// Actually, `processImage` sets `xo.Filter = "DCTDecode"`.
	// If we run `toImage` on the *result*, it will try to read JPEG data as raw RGB bytes, which will fail or produce garbage.
	// So we can't use `toImage` on the output unless `toImage` handles filters.
	// The current `toImage` implementation in `images.go` does NOT handle filters.
	// It just looks at ColorSpace and BPC.

	// So for this test, we just verify the metadata changes.
}

func TestOptimizeImages_Gray(t *testing.T) {
	// Create a simple 10x10 Gray image
	width, height := 10, 10
	data := make([]byte, width*height)
	for i := 0; i < len(data); i++ {
		data[i] = 128 // Gray
	}

	originalXO := semantic.XObject{
		Subtype:          "Image",
		Width:            width,
		Height:           height,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
		BitsPerComponent: 8,
		Data:             data,
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": originalXO,
					},
				},
			},
		},
	}

	config := Config{
		ImageQuality: 75,
	}
	opt := New(config)

	err := opt.Optimize(context.Background(), doc)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	optimizedXO := doc.Pages[0].Resources.XObjects["Im1"]

	if optimizedXO.Filter != "DCTDecode" {
		t.Errorf("Expected Filter to be DCTDecode, got %q", optimizedXO.Filter)
	}

	if optimizedXO.ColorSpace.ColorSpaceName() != "DeviceGray" {
		t.Errorf("Expected ColorSpace to be DeviceGray, got %v", optimizedXO.ColorSpace)
	}
}

func TestOptimizeImages_Downsample(t *testing.T) {
	// Create a 100x100 Gray image
	width, height := 100, 100
	data := make([]byte, width*height)
	for i := 0; i < len(data); i++ {
		data[i] = 128
	}

	ref := raw.ObjectRef{Num: 1, Gen: 0}
	originalXO := semantic.XObject{
		Subtype:          "Image",
		Width:            width,
		Height:           height,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
		BitsPerComponent: 8,
		Data:             data,
		OriginalRef:      ref,
	}

	// Draw it in a 10x10 point box.
	// PPI = 100 / (10/72) = 720 PPI.
	// If we set limit to 72 PPI.
	// Target PPI = 72.
	// Target Pixels = 72 * (10/72) = 10 pixels.
	// So it should downsample to 10x10.

	// Content stream: q 10 0 0 10 0 0 cm /Im1 Do Q
	ops := []semantic.Operation{
		{Operator: "q", Operands: []semantic.Operand{}},
		{Operator: "cm", Operands: []semantic.Operand{
			semantic.NumberOperand{Value: 10},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 10},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 0},
		}},
		{Operator: "Do", Operands: []semantic.Operand{
			semantic.NameOperand{Value: "Im1"},
		}},
		{Operator: "Q", Operands: []semantic.Operand{}},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": originalXO,
					},
				},
				Contents: []semantic.ContentStream{
					{Operations: ops},
				},
			},
		},
	}

	config := Config{
		ImageUpperPPI: 72,
		ImageQuality:  100,
	}
	opt := New(config)

	err := opt.Optimize(context.Background(), doc)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	optimizedXO := doc.Pages[0].Resources.XObjects["Im1"]

	// Check dimensions
	if optimizedXO.Width > 12 || optimizedXO.Height > 12 { // Allow some rounding error/buffer
		t.Errorf("Expected dimensions ~10x10, got %dx%d", optimizedXO.Width, optimizedXO.Height)
	}
	if optimizedXO.Width < 8 || optimizedXO.Height < 8 {
		t.Errorf("Expected dimensions ~10x10, got %dx%d", optimizedXO.Width, optimizedXO.Height)
	}
}

// Helper to create a solid color image
func createSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}
