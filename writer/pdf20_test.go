package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestPDF20_BlackPointCompensation(t *testing.T) {
	// 1. Create a document with ExtGState using UseBlackPtComp
	doc := &semantic.Document{}

	bpc := true
	gs := semantic.ExtGState{
		UseBlackPtComp: &bpc,
	}

	res := &semantic.Resources{
		ExtGStates: map[string]semantic.ExtGState{
			"GS1": gs,
		},
	}

	page := &semantic.Page{
		MediaBox:  semantic.Rectangle{LLX: 0, LLY: 0, URX: 612, URY: 792},
		Resources: res,
	}
	doc.Pages = []*semantic.Page{page}

	// 2. Write to buffer
	var buf bytes.Buffer
	wb := &WriterBuilder{}
	w := wb.Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Version: PDF17}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 3. Parse back using Pipeline
	parsedDoc, err := ir.NewDefault().Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Pipeline Parse failed: %v", err)
	}

	// 4. Verify
	if len(parsedDoc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(parsedDoc.Pages))
	}
	p := parsedDoc.Pages[0]
	if p.Resources == nil {
		t.Fatal("Page resources are nil")
	}
	if len(p.Resources.ExtGStates) != 1 {
		t.Fatalf("Expected 1 ExtGState, got %d", len(p.Resources.ExtGStates))
	}

	parsedGS, ok := p.Resources.ExtGStates["GS1"]
	if !ok {
		t.Fatal("GS1 not found")
	}

	if parsedGS.UseBlackPtComp == nil {
		t.Fatal("UseBlackPtComp is nil")
	}
	if *parsedGS.UseBlackPtComp != true {
		t.Fatal("UseBlackPtComp is not true")
	}
}

func TestPDF20_CxFColorSpace(t *testing.T) {
	// 1. Create a document with SpectrallyDefinedColorSpace
	doc := &semantic.Document{}

	cxfData := []byte("<CxF>...</CxF>")
	cs := &semantic.SpectrallyDefinedColorSpace{
		Data: cxfData,
	}

	res := &semantic.Resources{
		ColorSpaces: map[string]semantic.ColorSpace{
			"CS1": cs,
		},
	}

	page := &semantic.Page{
		MediaBox:  semantic.Rectangle{LLX: 0, LLY: 0, URX: 612, URY: 792},
		Resources: res,
	}
	doc.Pages = []*semantic.Page{page}

	// 2. Write to buffer
	var buf bytes.Buffer
	wb := &WriterBuilder{}
	w := wb.Build()
	if err := w.Write(context.Background(), doc, &buf, Config{Version: PDF17}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 3. Parse back
	parsedDoc, err := ir.NewDefault().Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Pipeline Parse failed: %v", err)
	}

	// 4. Verify
	if len(parsedDoc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(parsedDoc.Pages))
	}
	p := parsedDoc.Pages[0]
	if p.Resources == nil {
		t.Fatal("Page resources are nil")
	}

	parsedCS, ok := p.Resources.ColorSpaces["CS1"]
	if !ok {
		t.Fatal("CS1 not found")
	}

	sdCS, ok := parsedCS.(*semantic.SpectrallyDefinedColorSpace)
	if !ok {
		t.Fatalf("Expected SpectrallyDefinedColorSpace, got %T", parsedCS)
	}

	if string(sdCS.Data) != string(cxfData) {
		t.Fatalf("Expected data %q, got %q", cxfData, sdCS.Data)
	}
}
