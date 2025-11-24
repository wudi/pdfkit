package optimize

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"math"

	"golang.org/x/image/draw"

	"github.com/wudi/pdfkit/contentstream"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type imageUsage struct {
	maxWidth  float64 // in points
	maxHeight float64 // in points
}

func (o *Optimizer) optimizeImages(ctx context.Context, doc *semantic.Document) error {
	// 1. Analyze usage to determine maximum display dimensions
	usageMap := o.collectImageUsage(ctx, doc)

	// 2. Visit and optimize resources
	seenRes := make(map[*semantic.Resources]bool)

	var visitResources func(res *semantic.Resources) error
	visitResources = func(res *semantic.Resources) error {
		if res == nil || seenRes[res] {
			return nil
		}
		seenRes[res] = true

		// XObjects
		for name, xo := range res.XObjects {
			if xo.Subtype == "Image" {
				optimized, err := o.processImage(ctx, xo, usageMap)
				if err != nil {
					return err
				}
				res.XObjects[name] = optimized
			} else if xo.Subtype == "Form" {
				if err := visitResources(xo.Resources); err != nil {
					return err
				}
			}
		}

		// Patterns
		for _, pat := range res.Patterns {
			if tp, ok := pat.(*semantic.TilingPattern); ok {
				if err := visitResources(tp.Resources); err != nil {
					return err
				}
			}
		}

		// Fonts (Type3)
		for _, font := range res.Fonts {
			if font.Resources != nil {
				if err := visitResources(font.Resources); err != nil {
					return err
				}
			}
		}

		return nil
	}

	for _, page := range doc.Pages {
		if err := visitResources(page.Resources); err != nil {
			return err
		}
	}

	return nil
}

func (o *Optimizer) collectImageUsage(ctx context.Context, doc *semantic.Document) map[raw.ObjectRef]imageUsage {
	usageMap := make(map[raw.ObjectRef]imageUsage)
	tracer := contentstream.NewTracer()

	for _, page := range doc.Pages {
		if len(page.Contents) == 0 {
			continue
		}

		// Flatten operations
		var ops []semantic.Operation
		for _, cs := range page.Contents {
			ops = append(ops, cs.Operations...)
		}

		bboxes, err := tracer.Trace(ops, page.Resources)
		if err != nil {
			// Skip pages with errors
			continue
		}

		// Map OpIndex to BBox
		bboxMap := make(map[int]semantic.Rectangle)
		for _, bbox := range bboxes {
			bboxMap[bbox.OpIndex] = bbox.Rect
		}

		// Correlate Do operators with BBoxes
		for i, op := range ops {
			if op.Operator == "Do" && len(op.Operands) == 1 {
				if name, ok := op.Operands[0].(semantic.NameOperand); ok {
					if rect, hasRect := bboxMap[i]; hasRect {
						// Resolve XObject
						if xo, ok := page.Resources.XObjects[name.Value]; ok {
							// Only track indirect objects (shared)
							if xo.OriginalRef.Num != 0 {
								w := math.Abs(rect.URX - rect.LLX)
								h := math.Abs(rect.URY - rect.LLY)

								curr := usageMap[xo.OriginalRef]
								if w > curr.maxWidth {
									curr.maxWidth = w
								}
								if h > curr.maxHeight {
									curr.maxHeight = h
								}
								usageMap[xo.OriginalRef] = curr
							}
						}
					}
				}
			}
		}
	}
	return usageMap
}

func (o *Optimizer) processImage(ctx context.Context, xo semantic.XObject, usageMap map[raw.ObjectRef]imageUsage) (semantic.XObject, error) {
	// Skip if already optimized (check Filter)
	// Note: We might want to re-optimize if quality settings changed, but for now assume DCTDecode means "compressed enough"
	// unless we want to resize.
	isJPEG := xo.Filter == "DCTDecode"

	// If no optimization requested, return
	if o.config.ImageQuality == 0 && o.config.ImageUpperPPI == 0 {
		return xo, nil
	}

	// Determine if we need to resize
	needsResize := false
	targetW, targetH := xo.Width, xo.Height

	if o.config.ImageUpperPPI > 0 && xo.OriginalRef.Num != 0 {
		if usage, ok := usageMap[xo.OriginalRef]; ok {
			// Calculate max allowed dimensions
			// PPI = Pixels / (Points / 72)
			// Pixels = PPI * Points / 72
			maxW := o.config.ImageUpperPPI * usage.maxWidth / 72.0
			maxH := o.config.ImageUpperPPI * usage.maxHeight / 72.0

			if maxW > 0 && maxH > 0 {
				if float64(xo.Width) > maxW*1.2 || float64(xo.Height) > maxH*1.2 { // 20% buffer
					needsResize = true
					scale := math.Min(maxW/float64(xo.Width), maxH/float64(xo.Height))
					targetW = int(float64(xo.Width) * scale)
					targetH = int(float64(xo.Height) * scale)
					if targetW < 1 {
						targetW = 1
					}
					if targetH < 1 {
						targetH = 1
					}
				}
			}
		}
	}

	// If already JPEG and no resize needed and quality is not forced (or we assume existing JPEG is fine), skip
	// But if ImageQuality is set, user might want to re-compress to lower quality.
	// For now, let's re-compress if it's not JPEG OR if we resized.
	// If it is JPEG and we didn't resize, we might skip to avoid generation loss unless explicitly asked?
	// Let's assume we re-compress if it's not JPEG or if we resized.
	if isJPEG && !needsResize {
		return xo, nil
	}

	// 1. Decode to image.Image
	img, err := toImage(xo)
	if err != nil {
		return xo, nil
	}
	if img == nil {
		return xo, nil
	}

	// 2. Resize
	if needsResize {
		dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = dst
	}

	// 3. Recompress
	if o.config.ImageQuality > 0 {
		var buf bytes.Buffer
		opts := &jpeg.Options{Quality: o.config.ImageQuality}
		if err := jpeg.Encode(&buf, img, opts); err != nil {
			return xo, nil
		}

		xo.Data = buf.Bytes()
		xo.Filter = "DCTDecode"
		xo.BitsPerComponent = 8
		xo.Width = img.Bounds().Dx()
		xo.Height = img.Bounds().Dy()

		// Update ColorSpace
		if isGray(img) {
			xo.ColorSpace = &semantic.DeviceColorSpace{Name: "DeviceGray"}
		} else {
			xo.ColorSpace = &semantic.DeviceColorSpace{Name: "DeviceRGB"}
		}
	}

	return xo, nil
}

func toImage(xo semantic.XObject) (image.Image, error) {
	width, height := xo.Width, xo.Height
	if width == 0 || height == 0 {
		return nil, nil
	}

	// If Filter is DCTDecode, we can decode it using jpeg.Decode
	if xo.Filter == "DCTDecode" {
		return jpeg.Decode(bytes.NewReader(xo.Data))
	}

	csName := ""
	if xo.ColorSpace != nil {
		csName = xo.ColorSpace.ColorSpaceName()
	}

	data := xo.Data
	bpc := xo.BitsPerComponent
	if bpc == 0 {
		bpc = 8
	}

	switch csName {
	case "DeviceGray":
		if bpc == 8 {
			if len(data) < width*height {
				return nil, nil
			}
			return &image.Gray{
				Pix:    data,
				Stride: width,
				Rect:   image.Rect(0, 0, width, height),
			}, nil
		}
	case "DeviceRGB":
		if bpc == 8 {
			// RGB is 3 bytes. image.RGBA is 4 bytes.
			// Convert to NRGBA
			if len(data) < width*height*3 {
				return nil, nil
			}
			img := image.NewNRGBA(image.Rect(0, 0, width, height))
			i := 0
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					offset := (y*width + x) * 4
					img.Pix[offset] = data[i]
					img.Pix[offset+1] = data[i+1]
					img.Pix[offset+2] = data[i+2]
					img.Pix[offset+3] = 255
					i += 3
				}
			}
			return img, nil
		}
	case "DeviceCMYK":
		if bpc == 8 {
			if len(data) < width*height*4 {
				return nil, nil
			}
			img := image.NewCMYK(image.Rect(0, 0, width, height))
			copy(img.Pix, data)
			return img, nil
		}
	}

	return nil, nil // Unsupported
}

func isGray(img image.Image) bool {
	switch img.(type) {
	case *image.Gray, *image.Gray16:
		return true
	}
	return false
}
