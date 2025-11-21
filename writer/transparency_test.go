package writer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
)

func TestSerializeBlendModes(t *testing.T) {
	// Test all 16 blend modes
	blendModes := []string{
		"Normal", "Multiply", "Screen", "Overlay", "Darken", "Lighten",
		"ColorDodge", "ColorBurn", "HardLight", "SoftLight", "Difference",
		"Exclusion", "Hue", "Saturation", "Color", "Luminosity",
	}

	for _, bm := range blendModes {
		t.Run(bm, func(t *testing.T) {
			doc := &semantic.Document{
				Pages: []*semantic.Page{
					{
						MediaBox: semantic.Rectangle{URX: 100, URY: 100},
						Resources: &semantic.Resources{
							ExtGStates: map[string]semantic.ExtGState{
								"GS1": {
									BlendMode: bm,
								},
							},
						},
						Contents: []semantic.ContentStream{
							{
								Operations: []semantic.Operation{
									{Operator: "gs", Operands: []semantic.Operand{semantic.NameOperand{Value: "GS1"}}},
								},
							},
						},
					},
				},
			}

			var buf bytes.Buffer
			w := NewWriter()
			if err := w.Write(context.Background(), doc, &buf, Config{}); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			output := buf.String()
			expected := "/BM /" + bm
			if !strings.Contains(output, expected) {
				t.Errorf("Expected output to contain %q, got:\n%s", expected, output)
			}
		})
	}
}

func TestSerializeTransparencyGroup(t *testing.T) {
	// Test Isolated and Knockout groups
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Form1": {
							Subtype: "Form",
							BBox:    semantic.Rectangle{URX: 50, URY: 50},
							Group: &semantic.TransparencyGroup{
								Isolated: true,
								Knockout: true,
								CS:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
							},
							Data: []byte("1 0 0 rg 0 0 50 50 re f"),
						},
					},
				},
				Contents: []semantic.ContentStream{
					{
						Operations: []semantic.Operation{
							{Operator: "Do", Operands: []semantic.Operand{semantic.NameOperand{Value: "Form1"}}},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	if err := w.Write(context.Background(), doc, &buf, Config{}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	// Check for Group dictionary
	if !strings.Contains(output, "/Type /Group") {
		t.Error("Output missing /Type /Group")
	}
	if !strings.Contains(output, "/S /Transparency") {
		t.Error("Output missing /S /Transparency")
	}
	if !strings.Contains(output, "/I true") {
		t.Error("Output missing /I true")
	}
	if !strings.Contains(output, "/K true") {
		t.Error("Output missing /K true")
	}
	if !strings.Contains(output, "/CS /DeviceRGB") {
		t.Error("Output missing /CS /DeviceRGB")
	}
}
