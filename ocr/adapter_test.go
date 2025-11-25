package ocr

import (
	"reflect"
	"testing"

	"github.com/wudi/pdfkit/extractor"
)

func TestInputFromImageAsset(t *testing.T) {
	asset := extractor.ImageAsset{
		Page:         2,
		ResourceName: "Im1",
		Width:        1,
		Height:       1,
		Data:         []byte{0, 0, 255},
	}
	region := Region{X: 0, Y: 0, Width: 1, Height: 1}
	meta := map[string]string{"psm": "6"}

	in, err := InputFromImageAsset(
		asset,
		WithLanguages("eng", "spa"),
		WithRegion(region),
		WithDPI(300),
		WithMetadata(meta),
	)
	if err != nil {
		t.Fatalf("InputFromImageAsset() error = %v", err)
	}
	if in.Format != ImageFormatPNG {
		t.Fatalf("unexpected format: %v", in.Format)
	}
	if in.PageIndex != 2 {
		t.Fatalf("unexpected page index: %d", in.PageIndex)
	}
	if got := in.ID; got != "page-2-Im1" {
		t.Fatalf("unexpected id: %s", got)
	}
	if len(in.Image) == 0 {
		t.Fatalf("expected encoded image data")
	}
	if !reflect.DeepEqual(in.Languages, []string{"eng", "spa"}) {
		t.Fatalf("unexpected languages: %+v", in.Languages)
	}
	if in.Region == nil || *in.Region != region {
		t.Fatalf("unexpected region: %#v", in.Region)
	}
	if in.DPI != 300 {
		t.Fatalf("unexpected dpi: %d", in.DPI)
	}
	meta["psm"] = "7"
	if in.Metadata["psm"] != "6" {
		t.Fatalf("metadata was not copied: %+v", in.Metadata)
	}
}

func TestWithRegionClearsEmpty(t *testing.T) {
	in := Input{Region: &Region{X: 1, Y: 1, Width: 2, Height: 2}}
	WithRegion(Region{})(&in)
	if in.Region != nil {
		t.Fatalf("expected nil region for empty input, got %#v", in.Region)
	}
}
