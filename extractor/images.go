package extractor

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// ImageAsset represents an image XObject found on a page.
type ImageAsset struct {
	Page             int
	ResourceName     string
	Width            int
	Height           int
	BitsPerComponent int
	ColorSpace       string
	Filters          []string
	Data             []byte
}

// ExtractImages walks page resources and returns embedded image XObjects.
func (e *Extractor) ExtractImages() ([]ImageAsset, error) {
	var assets []ImageAsset
	for idx, page := range e.pages {
		resDict := derefDict(e.raw, valueFromDict(page, "Resources"))
		if resDict == nil {
			continue
		}
		xobjects := derefDict(e.raw, valueFromDict(resDict, "XObject"))
		if xobjects == nil {
			continue
		}
		for name, obj := range xobjects.KV {
			data, dict, filters, ok := streamData(e.dec, obj)
			if !ok || dict == nil {
				continue
			}
			subtype, _ := nameFromDict(dict, "Subtype")
			if subtype != "Image" {
				continue
			}
			width, _ := intFromObject(valueFromDict(dict, "Width"))
			height, _ := intFromObject(valueFromDict(dict, "Height"))
			bpc, _ := intFromObject(valueFromDict(dict, "BitsPerComponent"))
			color, _ := nameFromDict(dict, "ColorSpace")
			asset := ImageAsset{
				Page:             idx,
				ResourceName:     name,
				Width:            width,
				Height:           height,
				BitsPerComponent: bpc,
				ColorSpace:       color,
				Filters:          filters,
				Data:             data,
			}
			assets = append(assets, asset)
		}
	}
	return assets, nil
}

// ToImage converts the raw image data into a standard Go image.Image.
func (i ImageAsset) ToImage() (image.Image, error) {
	if len(i.Data) == 0 {
		return nil, errors.New("image data is empty")
	}

	// Check if the image was decoded by a filter that produces NRGBA (JPEG, JPX)
	for _, f := range i.Filters {
		if f == "DCTDecode" || f == "JPXDecode" {
			// These filters return NRGBA data (4 bytes per pixel)
			if len(i.Data) != i.Width*i.Height*4 {
				// It might be that JPX returned something else, but our filter implementation
				// tries to normalize to NRGBA.
				// If length matches W*H*4, assume NRGBA.
				return nil, fmt.Errorf("unexpected data length for %s: got %d, want %d", f, len(i.Data), i.Width*i.Height*4)
			}
			return &image.NRGBA{
				Pix:    i.Data,
				Stride: i.Width * 4,
				Rect:   image.Rect(0, 0, i.Width, i.Height),
			}, nil
		}
		if f == "CCITTFaxDecode" {
			// CCITT returns Gray (1 byte per pixel)
			if len(i.Data) != i.Width*i.Height {
				return nil, fmt.Errorf("unexpected data length for CCITT: got %d, want %d", len(i.Data), i.Width*i.Height)
			}
			return &image.Gray{
				Pix:    i.Data,
				Stride: i.Width,
				Rect:   image.Rect(0, 0, i.Width, i.Height),
			}, nil
		}
	}

	// Fallback based on data length and color space
	pixelCount := i.Width * i.Height
	if pixelCount == 0 {
		return nil, errors.New("invalid image dimensions")
	}

	if len(i.Data) == pixelCount*4 {
		// Likely CMYK or RGBA.
		// If ColorSpace is DeviceCMYK, it's CMYK.
		if i.ColorSpace == "DeviceCMYK" {
			return &image.CMYK{
				Pix:    i.Data,
				Stride: i.Width * 4,
				Rect:   image.Rect(0, 0, i.Width, i.Height),
			}, nil
		}
		// Assume RGBA/NRGBA if not CMYK
		return &image.RGBA{
			Pix:    i.Data,
			Stride: i.Width * 4,
			Rect:   image.Rect(0, 0, i.Width, i.Height),
		}, nil
	}

	if len(i.Data) == pixelCount*3 {
		// RGB
		return &rgbImage{
			Pix:    i.Data,
			Stride: i.Width * 3,
			Rect:   image.Rect(0, 0, i.Width, i.Height),
		}, nil
	}

	if len(i.Data) == pixelCount {
		// Gray
		return &image.Gray{
			Pix:    i.Data,
			Stride: i.Width,
			Rect:   image.Rect(0, 0, i.Width, i.Height),
		}, nil
	}

	return nil, fmt.Errorf("unsupported image format: %d bytes for %dx%d image", len(i.Data), i.Width, i.Height)
}

// ToPNG encodes the image asset to PNG format.
func (i ImageAsset) ToPNG() ([]byte, error) {
	img, err := i.ToImage()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Custom image implementations for RGB to support standard library interfaces

type rgbImage struct {
	Pix    []byte
	Stride int
	Rect   image.Rectangle
}

func (p *rgbImage) ColorModel() color.Model { return color.RGBAModel }
func (p *rgbImage) Bounds() image.Rectangle { return p.Rect }
func (p *rgbImage) At(x, y int) color.Color {
	if !(image.Point{x, y}.In(p.Rect)) {
		return color.RGBA{}
	}
	i := (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
	return color.RGBA{R: p.Pix[i], G: p.Pix[i+1], B: p.Pix[i+2], A: 255}
}
