package extensions

import (
	"context"
	"errors"
	"testing"

	"github.com/wudi/pdfkit/extractor"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/ocr"
)

type fakeBatchEngine struct {
	inputs      []ocr.Input
	results     []ocr.Result
	err         error
	calledBatch bool
}

func (f *fakeBatchEngine) Name() string { return "fake" }
func (f *fakeBatchEngine) Recognize(ctx context.Context, in ocr.Input) (ocr.Result, error) {
	return ocr.Result{}, errors.New("unexpected single call")
}
func (f *fakeBatchEngine) RecognizeBatch(ctx context.Context, inputs []ocr.Input) ([]ocr.Result, error) {
	f.calledBatch = true
	f.inputs = append([]ocr.Input(nil), inputs...)
	return append([]ocr.Result(nil), f.results...), f.err
}

func TestOCRExtensionUsesBatchEngine(t *testing.T) {
	engine := &fakeBatchEngine{
		results: []ocr.Result{{InputID: "page-0-Im1", PlainText: "ok"}},
	}
	ext := &OCRExtension{
		engine:       engine,
		inputOptions: []ocr.InputOption{ocr.WithLanguages("eng")},
		assetProvider: func(ctx context.Context, doc *semantic.Document) ([]extractor.ImageAsset, error) {
			return []extractor.ImageAsset{{
				Page:         0,
				ResourceName: "Im1",
				Width:        1,
				Height:       1,
				Data:         []byte{0, 0, 0, 255},
			}}, nil
		},
	}

	doc := &semantic.Document{}
	if err := ext.Execute(context.Background(), doc); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !engine.calledBatch {
		t.Fatalf("expected batch engine to be used")
	}
	if len(engine.inputs) != 1 || engine.inputs[0].Languages[0] != "eng" {
		t.Fatalf("unexpected inputs: %+v", engine.inputs)
	}
	if got := ext.Results(); len(got) != 1 || got[0].PlainText != "ok" {
		t.Fatalf("unexpected results: %+v", got)
	}
	if len(doc.OCRResults) != 1 || doc.OCRResults[0].PlainText != "ok" {
		t.Fatalf("expected OCR results on document, got %+v", doc.OCRResults)
	}
}
