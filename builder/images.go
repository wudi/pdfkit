package builder

import (
	"image"
	"image/draw"
	_ "image/jpeg" // Register decoders
	_ "image/png"
	"os"

	"pdflib/ir/semantic"
)

// ImageFromFile loads an image from a file path and converts it to *semantic.Image.
func ImageFromFile(path string) (*semantic.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return FromImage(img), nil
}

// ImageFromGoImage converts a standard Go image.Image to *semantic.Image.
// It handles RGB color and creates a Soft Mask (SMask) for transparency if needed.
func FromImage(src image.Image) *semantic.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Convert to NRGBA (non-premultiplied alpha) to get raw color values
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(nrgba, nrgba.Bounds(), src, bounds.Min, draw.Src)

	pixels := make([]byte, 0, w*h*3)
	alpha := make([]byte, 0, w*h)
	hasAlpha := false

	for i := 0; i < w*h; i++ {
		offset := i * 4
		// Extract RGB
		pixels = append(pixels, nrgba.Pix[offset], nrgba.Pix[offset+1], nrgba.Pix[offset+2])

		// Extract Alpha
		a := nrgba.Pix[offset+3]
		alpha = append(alpha, a)
		if a < 255 {
			hasAlpha = true
		}
	}

	img := &semantic.Image{
		Width:            w,
		Height:           h,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
		BitsPerComponent: 8,
		Data:             pixels,
	}

	// If the image has transparency, attach it as a Soft Mask
	if hasAlpha {
		img.SMask = &semantic.Image{
			Width:            w,
			Height:           h,
			ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceGray"},
			BitsPerComponent: 8,
			Data:             alpha,
		}
	}

	return img
}
