package main

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	// Load image
	imgFile, err := os.Open("../../testdata/marilyn-monroe.jpg")
	if err != nil {
		panic(err)
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	if err != nil {
		panic(err)
	}

	// Convert to raw RGB
	rgbData := imageToRGB(img)
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Create PDF
	b := builder.NewBuilder()

	// Create semantic image
	pdfImg := &semantic.Image{
		Width:            width,
		Height:           height,
		BitsPerComponent: 8,
		ColorSpace:       semantic.DeviceColorSpace{Name: "DeviceRGB"},
		Data:             rgbData,
	}

	// Add page and draw image
	// Scale image to fit on page (e.g., width 400)
	targetWidth := 400.0
	scale := targetWidth / float64(width)
	targetHeight := float64(height) * scale

	b.NewPage(595, 842).
		DrawText("Image Example", 50, 800, builder.TextOptions{FontSize: 24}).
		DrawImage(pdfImg, 50, 750-targetHeight, targetWidth, targetHeight, builder.ImageOptions{}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		panic(err)
	}

	f, err := os.Create("image.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		panic(err)
	}

	fmt.Println("Successfully created image.pdf")
}

func imageToRGB(img image.Image) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	data := make([]byte, w*h*3)
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA returns 0-65535, we want 0-255
			data[idx] = byte(r >> 8)
			data[idx+1] = byte(g >> 8)
			data[idx+2] = byte(b >> 8)
			idx += 3
		}
	}
	return data
}
