package ocr

import (
	"fmt"

	"github.com/wudi/pdfkit/extractor"
)

// InputOption mutates an OCR input generated from a PDF image asset.
type InputOption func(*Input)

// WithLanguages sets language hints on the OCR input.
func WithLanguages(langs ...string) InputOption {
	return func(in *Input) { in.Languages = append([]string(nil), langs...) }
}

// WithRegion sets the recognition region on the OCR input.
func WithRegion(region Region) InputOption {
	return func(in *Input) {
		if region.IsEmpty() {
			in.Region = nil
			return
		}
		in.Region = &region
	}
}

// WithDPI overrides the DPI value on the OCR input.
func WithDPI(dpi int) InputOption {
	return func(in *Input) { in.DPI = dpi }
}

// WithMetadata sets provider-specific metadata for the input.
func WithMetadata(metadata map[string]string) InputOption {
	return func(in *Input) {
		if len(metadata) == 0 {
			in.Metadata = nil
			return
		}
		in.Metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			in.Metadata[k] = v
		}
	}
}

// InputFromImageAsset converts an extractor.ImageAsset into an OCR input using
// PNG encoding. The generated ID is stable for the resource name on a page to
// simplify correlation with downstream results.
func InputFromImageAsset(asset extractor.ImageAsset, opts ...InputOption) (Input, error) {
	data, err := asset.ToPNG()
	if err != nil {
		return Input{}, fmt.Errorf("encode image asset: %w", err)
	}
	in := Input{
		ID:        fmt.Sprintf("page-%d-%s", asset.Page, asset.ResourceName),
		Image:     data,
		Format:    ImageFormatPNG,
		PageIndex: asset.Page,
	}
	for _, opt := range opts {
		opt(&in)
	}
	return in, nil
}
