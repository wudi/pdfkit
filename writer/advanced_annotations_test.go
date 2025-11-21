package writer

import (
	"bytes"
	"context"
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestAdvancedAnnotations(t *testing.T) {
	// Create a page with various advanced annotations
	page := &semantic.Page{
		MediaBox: semantic.Rectangle{URX: 595, URY: 842},
		Annotations: []semantic.Annotation{
			&semantic.ThreeDAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "3D",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 700, URX: 200, URY: 720},
				},
				ThreeD: semantic.EmbeddedFile{
					Data: []byte("dummy 3d data"),
				},
				View: "DefaultView",
			},
			&semantic.RedactAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Redact",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 650, URX: 200, URY: 670},
				},
				OverlayText: "REDACTED",
				Repeat:      []float64{1, 2},
			},
			&semantic.ProjectionAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Projection",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 600, URX: 200, URY: 620},
				},
				ProjectionType: "P",
			},
			&semantic.SoundAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Sound",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 550, URX: 200, URY: 570},
				},
				Name: "MySound",
				Sound: semantic.EmbeddedFile{
					Data: []byte("dummy sound data"),
				},
			},
			&semantic.MovieAnnotation{
				BaseAnnotation: semantic.BaseAnnotation{
					Subtype: "Movie",
					RectVal: semantic.Rectangle{LLX: 100, LLY: 500, URX: 200, URY: 520},
				},
				Title: "MyMovie",
				Movie: semantic.EmbeddedFile{
					Name: "movie.mp4",
					Data: []byte("dummy movie data"),
				},
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

	// Verify 3D
	if !strings.Contains(output, "/Subtype /3D") {
		t.Error("Missing 3D annotation")
	}
	if !strings.Contains(output, "/3DD") {
		t.Error("Missing 3D Data stream reference")
	}
	if !strings.Contains(output, "/3DV") {
		t.Error("Missing 3D View dictionary")
	}
	if !strings.Contains(output, "/XN (DefaultView)") {
		t.Error("Missing 3D View name")
	}

	// Verify Redact
	if !strings.Contains(output, "/Subtype /Redact") {
		t.Error("Missing Redact annotation")
	}
	if !strings.Contains(output, "/OverlayText (REDACTED)") {
		t.Error("Missing OverlayText for Redact")
	}
	if !strings.Contains(output, "/Repeat [1 2]") {
		t.Error("Missing/Incorrect Repeat for Redact")
	}

	// Verify Projection
	if !strings.Contains(output, "/Subtype /Projection") {
		t.Error("Missing Projection annotation")
	}
	if !strings.Contains(output, "/ProjectionType /P") {
		t.Error("Missing ProjectionType")
	}

	// Verify Sound
	if !strings.Contains(output, "/Subtype /Sound") {
		t.Error("Missing Sound annotation")
	}
	if !strings.Contains(output, "/Name /MySound") {
		t.Error("Missing Sound Name")
	}
	if !strings.Contains(output, "/Sound") {
		t.Error("Missing Sound stream reference")
	}

	// Verify Movie
	if !strings.Contains(output, "/Subtype /Movie") {
		t.Error("Missing Movie annotation")
	}
	if !strings.Contains(output, "/T (MyMovie)") {
		t.Error("Missing Movie Title")
	}
	if !strings.Contains(output, "/Movie <<") {
		t.Error("Missing Movie dictionary")
	}
}
