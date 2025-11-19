package writer

import (
	"bytes"
	"context"
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestLinearization(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
			},
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	cfg := Config{
		Linearize: true,
	}

	err := w.Write(context.Background(), doc, &buf, cfg)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("Output is empty")
	}

	output := buf.String()
	if !strings.Contains(output, "/Linearized 1") {
		t.Error("Output does not contain /Linearized 1")
	}

	// Check for Hint Stream dict entry
	if !strings.Contains(output, "/S 0") {
		t.Error("Output does not contain Hint Stream /S entry")
	}

	// Check for multiple xrefs (First Page XRef and Main XRef)
	if strings.Count(output, "xref") < 2 {
		t.Errorf("Expected at least 2 xref tables, found %d", strings.Count(output, "xref"))
	}
}
