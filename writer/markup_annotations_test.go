package writer

import (
	"bytes"
	"context"
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestMarkupAnnotations(t *testing.T) {
	// Create a page with various markup annotations
	page := &semantic.Page{
		MediaBox: semantic.Rectangle{URX: 595, URY: 842},
		Annotations: []semantic.Annotation{
			&semantic.HighlightAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Highlight",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 700, URX: 200, URY: 720},
					Color:   []float64{1, 1, 0}, // Yellow
				},
				QuadPoints: []float64{100, 720, 200, 720, 100, 700, 200, 700},
			},
			&semantic.UnderlineAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Underline",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 650, URX: 200, URY: 670},
					Color:   []float64{0, 0, 1}, // Blue
				},
				QuadPoints: []float64{100, 670, 200, 670, 100, 650, 200, 650},
			},
			&semantic.StrikeOutAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "StrikeOut",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 600, URX: 200, URY: 620},
					Color:   []float64{1, 0, 0}, // Red
				},
				QuadPoints: []float64{100, 620, 200, 620, 100, 600, 200, 600},
			},
			&semantic.SquigglyAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Squiggly",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 550, URX: 200, URY: 570},
					Color:   []float64{0, 1, 0}, // Green
				},
				QuadPoints: []float64{100, 570, 200, 570, 100, 550, 200, 550},
			},
		},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{page},
	}

	var buf bytes.Buffer
	w := NewWriter()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	// Verify Highlight
	if !strings.Contains(output, "/Subtype /Highlight") {
		t.Error("Missing Highlight annotation")
	}
	if !strings.Contains(output, "/QuadPoints [100 720 200 720 100 700 200 700]") {
		t.Error("Missing/Incorrect QuadPoints for Highlight")
	}
	if !strings.Contains(output, "/C [1 1 0]") {
		t.Error("Missing/Incorrect Color for Highlight")
	}

	// Verify Underline
	if !strings.Contains(output, "/Subtype /Underline") {
		t.Error("Missing Underline annotation")
	}
	if !strings.Contains(output, "/QuadPoints [100 670 200 670 100 650 200 650]") {
		t.Error("Missing/Incorrect QuadPoints for Underline")
	}
	if !strings.Contains(output, "/C [0 0 1]") {
		t.Error("Missing/Incorrect Color for Underline")
	}

	// Verify StrikeOut
	if !strings.Contains(output, "/Subtype /StrikeOut") {
		t.Error("Missing StrikeOut annotation")
	}
	if !strings.Contains(output, "/QuadPoints [100 620 200 620 100 600 200 600]") {
		t.Error("Missing/Incorrect QuadPoints for StrikeOut")
	}
	if !strings.Contains(output, "/C [1 0 0]") {
		t.Error("Missing/Incorrect Color for StrikeOut")
	}

	// Verify Squiggly
	if !strings.Contains(output, "/Subtype /Squiggly") {
		t.Error("Missing Squiggly annotation")
	}
	if !strings.Contains(output, "/QuadPoints [100 570 200 570 100 550 200 550]") {
		t.Error("Missing/Incorrect QuadPoints for Squiggly")
	}
	if !strings.Contains(output, "/C [0 1 0]") {
		t.Error("Missing/Incorrect Color for Squiggly")
	}
}
