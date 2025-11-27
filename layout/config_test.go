package layout

import (
	"testing"

	"github.com/wudi/pdfkit/builder"
)

func TestEngineConfiguration(t *testing.T) {
	mb := &MockBuilder{}

	t.Run("Default Configuration", func(t *testing.T) {
		e := NewEngine(mb)
		if e.DefaultFont != "Helvetica" {
			t.Errorf("Expected default font Helvetica, got %s", e.DefaultFont)
		}
		if e.DefaultFontSize != 12 {
			t.Errorf("Expected default font size 12, got %f", e.DefaultFontSize)
		}
		if e.pageWidth != 595.28 {
			t.Errorf("Expected default page width 595.28, got %f", e.pageWidth)
		}
	})

	t.Run("Custom Configuration", func(t *testing.T) {
		e := NewEngine(mb,
			WithDefaultFont("Times-Roman"),
			WithDefaultFontSize(14),
			WithLineHeight(1.5),
			WithMargins(Margins{Top: 20, Bottom: 20, Left: 20, Right: 20}),
			WithPageSize(1000, 1000),
		)

		if e.DefaultFont != "Times-Roman" {
			t.Errorf("Expected font Times-Roman, got %s", e.DefaultFont)
		}
		if e.DefaultFontSize != 14 {
			t.Errorf("Expected font size 14, got %f", e.DefaultFontSize)
		}
		if e.LineHeight != 1.5 {
			t.Errorf("Expected line height 1.5, got %f", e.LineHeight)
		}
		if e.Margins.Top != 20 {
			t.Errorf("Expected top margin 20, got %f", e.Margins.Top)
		}
		if e.pageWidth != 1000 {
			t.Errorf("Expected page width 1000, got %f", e.pageWidth)
		}
	})

	t.Run("Paper Size Configuration", func(t *testing.T) {
		e := NewEngine(mb, WithPaperSize(builder.A3))
		if e.pageWidth != 841.89 {
			t.Errorf("Expected A3 width 841.89, got %f", e.pageWidth)
		}
		if e.pageHeight != 1190.55 {
			t.Errorf("Expected A3 height 1190.55, got %f", e.pageHeight)
		}
	})
}
