package tesseract

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"

	"github.com/otiai10/gosseract/v2"
	"github.com/wudi/pdfkit/ocr"
)

func init() {
	ocr.SetDefaultEngine(NewTesseractEngine())
}

// TesseractEngine implements Engine and BatchEngine using the gosseract client
// as the default OCR provider.
type TesseractEngine struct {
	clientFactory func() *gosseract.Client
}

// NewTesseractEngine constructs a Tesseract-backed OCR engine.
func NewTesseractEngine() *TesseractEngine {
	return &TesseractEngine{clientFactory: gosseract.NewClient}
}

func (e *TesseractEngine) Name() string { return "tesseract" }

// Recognize performs OCR on a single image input.
func (e *TesseractEngine) Recognize(ctx context.Context, in ocr.Input) (ocr.Result, error) {
	c := e.clientFactory()
	defer c.Close()
	return e.recognizeWithClient(ctx, c, in)
}

// RecognizeBatch processes multiple inputs using a single client instance to
// amortize setup costs. Inputs are processed sequentially.
func (e *TesseractEngine) RecognizeBatch(ctx context.Context, inputs []ocr.Input) ([]ocr.Result, error) {
	results := make([]ocr.Result, 0, len(inputs))
	for _, in := range inputs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		c := e.clientFactory()
		res, err := e.recognizeWithClient(ctx, c, in)
		if err != nil {
			return nil, fmt.Errorf("recognize %s: %w", in.ID, err)
		}
		c.Close()
		results = append(results, res)
	}
	return results, nil
}

func (e *TesseractEngine) recognizeWithClient(ctx context.Context, c *gosseract.Client, in ocr.Input) (ocr.Result, error) {
	imgData, err := cropImage(in.Image, in.Region)
	if err != nil {
		return ocr.Result{}, err
	}
	if err := c.SetImageFromBytes(imgData); err != nil {
		return ocr.Result{}, fmt.Errorf("set image: %w", err)
	}
	if len(in.Languages) > 0 {
		if err := c.SetLanguage(in.Languages...); err != nil {
			return ocr.Result{}, fmt.Errorf("set languages: %w", err)
		}
	}
	if in.DPI > 0 {
		if err := c.SetVariable(gosseract.SettableVariable("user_defined_dpi"), fmt.Sprint(in.DPI)); err != nil {
			return ocr.Result{}, fmt.Errorf("set dpi: %w", err)
		}
	}
	for k, v := range in.Metadata {
		if err := c.SetVariable(gosseract.SettableVariable(k), v); err != nil {
			return ocr.Result{}, fmt.Errorf("set variable %s: %w", k, err)
		}
	}
	text, err := c.Text()
	if err != nil {
		return ocr.Result{}, fmt.Errorf("recognize text: %w", err)
	}
	plain := strings.TrimSpace(text)

	words, avgConf := extractWords(c)
	bounds := mergeBounds(words)
	block := ocr.TextBlock{
		Text:       plain,
		Bounds:     bounds,
		Lines:      []ocr.TextLine{{Text: plain, Bounds: bounds, Words: words, Confidence: avgConf}},
		Confidence: avgConf,
	}

	return ocr.Result{
		InputID:   in.ID,
		PlainText: plain,
		Blocks:    []ocr.TextBlock{block},
		Language:  firstLanguage(in.Languages),
	}, nil
}

func extractWords(c *gosseract.Client) ([]ocr.TextWord, float64) {
	boxes, err := c.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil || len(boxes) == 0 {
		return nil, 0
	}
	words := make([]ocr.TextWord, 0, len(boxes))
	var sum float64
	for _, b := range boxes {
		conf := b.Confidence / 100.0
		sum += conf
		words = append(words, ocr.TextWord{
			Text:       b.Word,
			Bounds:     ocr.Region{X: float64(b.Box.Min.X), Y: float64(b.Box.Min.Y), Width: float64(b.Box.Dx()), Height: float64(b.Box.Dy())},
			Confidence: conf,
		})
	}
	if len(words) == 0 {
		return words, 0
	}
	return words, sum / float64(len(words))
}

func mergeBounds(words []ocr.TextWord) ocr.Region {
	if len(words) == 0 {
		return ocr.Region{}
	}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	var maxX, maxY float64
	for _, w := range words {
		minX = math.Min(minX, w.Bounds.X)
		minY = math.Min(minY, w.Bounds.Y)
		maxX = math.Max(maxX, w.Bounds.X+w.Bounds.Width)
		maxY = math.Max(maxY, w.Bounds.Y+w.Bounds.Height)
	}
	return ocr.Region{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

func firstLanguage(langs []string) string {
	if len(langs) == 0 {
		return ""
	}
	return langs[0]
}

func cropImage(data []byte, region *ocr.Region) ([]byte, error) {
	if region == nil || region.IsEmpty() {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode for region: %w", err)
	}
	rect := image.Rect(
		int(math.Round(region.X)),
		int(math.Round(region.Y)),
		int(math.Round(region.X+region.Width)),
		int(math.Round(region.Y+region.Height)),
	).Intersect(img.Bounds())
	if rect.Empty() {
		return nil, fmt.Errorf("region outside image bounds")
	}
	subImg, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("image does not support sub-image")
	}
	cropped := subImg.SubImage(rect)
	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return nil, fmt.Errorf("encode cropped image: %w", err)
	}
	return buf.Bytes(), nil
}
