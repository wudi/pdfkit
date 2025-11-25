package extensions

import (
	"context"
	"fmt"

	"github.com/wudi/pdfkit/extractor"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/ocr"
)

// OCRExtension runs OCR on page images using the default (Tesseract) engine
// unless another engine is supplied. Results are stored for later retrieval via
// Results().
type OCRExtension struct {
	engine        ocr.Engine
	inputOptions  []ocr.InputOption
	results       []ocr.Result
	assetProvider func(ctx context.Context, doc *semantic.Document) ([]extractor.ImageAsset, error)
}

// NewOCRExtension constructs an OCR extension. If engine is nil, the default
// Tesseract-backed engine is used.
func NewOCRExtension(engine ocr.Engine, opts ...ocr.InputOption) *OCRExtension {
	if engine == nil {
		engine = ocr.DefaultEngine()
	}
	return &OCRExtension{
		engine:        engine,
		inputOptions:  opts,
		assetProvider: extractAssets,
	}
}

func extractAssets(ctx context.Context, doc *semantic.Document) ([]extractor.ImageAsset, error) {
	if doc == nil || doc.Decoded() == nil {
		return nil, fmt.Errorf("decoded document is required for OCR")
	}
	ext, err := extractor.New(doc.Decoded())
	if err != nil {
		return nil, fmt.Errorf("build extractor: %w", err)
	}
	return ext.ExtractImages()
}

func (o *OCRExtension) Name() string          { return "ocr" }
func (o *OCRExtension) Phase() Phase          { return PhaseOCR }
func (o *OCRExtension) Priority() int         { return 100 }
func (o *OCRExtension) Results() []ocr.Result { return append([]ocr.Result(nil), o.results...) }

// Execute performs OCR and stores the results on the extension instance.
func (o *OCRExtension) Execute(ctx context.Context, doc *semantic.Document) error {
	assets, err := o.assetProvider(ctx, doc)
	if err != nil {
		return err
	}
	res, err := ocr.RecognizeAssets(ctx, o.engine, assets, o.inputOptions...)
	if err != nil {
		return err
	}
	o.results = res
	if doc != nil {
		doc.OCRResults = make([]semantic.OCRResult, len(res))
		for i, r := range res {
			doc.OCRResults[i] = semantic.OCRResult{InputID: r.InputID, PlainText: r.PlainText}
		}
	}
	return nil
}
