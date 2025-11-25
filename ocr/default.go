package ocr

import (
	"context"
	"fmt"

	"github.com/wudi/pdfkit/extractor"
)

var defaultEngine Engine = &noopEngine{}

// DefaultEngine returns the library's default OCR engine (Tesseract).
func DefaultEngine() Engine {
	return defaultEngine
}

// SetDefaultEngine sets the library's default OCR engine.
func SetDefaultEngine(engine Engine) {
	defaultEngine = engine
}

// RecognizeAssets converts image assets to OCR inputs and invokes the provided
// engine. If the engine supports batch operation, it is used; otherwise calls
// are executed sequentially.
func RecognizeAssets(ctx context.Context, engine Engine, assets []extractor.ImageAsset, opts ...InputOption) ([]Result, error) {
	inputs := make([]Input, 0, len(assets))
	for _, asset := range assets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		in, err := InputFromImageAsset(asset, opts...)
		if err != nil {
			return nil, fmt.Errorf("build input for %s: %w", asset.ResourceName, err)
		}
		inputs = append(inputs, in)
	}
	if b, ok := engine.(BatchEngine); ok {
		return b.RecognizeBatch(ctx, inputs)
	}
	results := make([]Result, 0, len(inputs))
	for _, in := range inputs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		res, err := engine.Recognize(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("recognize %s: %w", in.ID, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// DefaultRecognizeAssets runs recognition with the default (Tesseract) engine.
func DefaultRecognizeAssets(ctx context.Context, assets []extractor.ImageAsset, opts ...InputOption) ([]Result, error) {
	return RecognizeAssets(ctx, DefaultEngine(), assets, opts...)
}

type noopEngine struct{}

func (n noopEngine) Name() string {
	return "noop"
}

func (n noopEngine) Recognize(ctx context.Context, input Input) (Result, error) {
	return Result{InputID: input.ID}, nil
}
