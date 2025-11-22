package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestObjectStreams(t *testing.T) {
	// Create a simple document with many small objects
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
				Contents: []semantic.ContentStream{
					{
						Operations: []semantic.Operation{
							{Operator: "q"},
							{Operator: "Q"},
						},
					},
				},
			},
		},
	}

	// 1. Write without Object Streams
	var bufNoComp bytes.Buffer
	cfgNoComp := Config{
		ObjectStreams: false,
		Compression:   0,
	}
	w := NewWriter()
	if err := w.Write(context.Background(), doc, &bufNoComp, cfgNoComp); err != nil {
		t.Fatalf("Write failed without compression: %v", err)
	}

	// 2. Write with Object Streams (Uncompressed)
	var bufObjStm bytes.Buffer
	cfgObjStm := Config{
		ObjectStreams: true,
		Compression:   0,
	}
	if err := w.Write(context.Background(), doc, &bufObjStm, cfgObjStm); err != nil {
		t.Fatalf("Write failed with ObjStm (no comp): %v", err)
	}

	// 3. Write with Object Streams (Compressed)
	var bufComp bytes.Buffer
	cfgComp := Config{
		ObjectStreams: true,
		Compression:   9,
	}
	if err := w.Write(context.Background(), doc, &bufComp, cfgComp); err != nil {
		t.Fatalf("Write failed with compression: %v", err)
	}

	// 4. Verify output size
	t.Logf("Size without compression: %d", bufNoComp.Len())
	t.Logf("Size with ObjStm (no comp): %d", bufObjStm.Len())
	t.Logf("Size with compression: %d", bufComp.Len())

	// 5. Parse the ObjStm (no comp) PDF
	pipeline := ir.NewDefault()
	parsedDoc, err := pipeline.Parse(context.Background(), bytes.NewReader(bufObjStm.Bytes()))
	if err != nil {
		t.Fatalf("Failed to parse ObjStm (no comp) PDF: %v", err)
	}
	if len(parsedDoc.Pages) != 1 {
		t.Errorf("ObjStm (no comp): Expected 1 page, got %d", len(parsedDoc.Pages))
	}

	// 6. Parse the compressed PDF
	parsedDocComp, err := pipeline.Parse(context.Background(), bytes.NewReader(bufComp.Bytes()))
	if err != nil {
		t.Fatalf("Failed to parse compressed PDF: %v", err)
	}

	if len(parsedDocComp.Pages) != 1 {
		t.Errorf("Compressed: Expected 1 page, got %d", len(parsedDocComp.Pages))
	}
}
