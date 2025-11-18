package fonts

import (
	"fmt"
	"math"
	"strings"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"pdflib/ir/semantic"
)

// LoadTrueType parses a TrueType/OpenType font, extracts basic metrics, and
// returns a semantic.Font configured for Type0 Identity-H usage with a
// FontFile2 stream. The full font is embedded (no subsetting).
func LoadTrueType(name string, data []byte) (*semantic.Font, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("truetype font data is empty")
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse truetype: %w", err)
	}
	unitsPerEm := font.UnitsPerEm()
	if unitsPerEm == 0 {
		return nil, fmt.Errorf("invalid unitsPerEm")
	}
	buf := &sfnt.Buffer{}
	ppem := fixed.Int26_6(unitsPerEm << 6)

	baseName := strings.TrimSpace(name)
	if ps, _ := font.Name(buf, sfnt.NameIDPostScript); len(ps) > 0 {
		baseName = ps
	}
	if baseName == "" {
		baseName = "CustomTT"
	}

	widths := glyphWidths(font, buf, unitsPerEm, ppem)
	defaultWidth := widths[0]
	if defaultWidth == 0 {
		defaultWidth = 1000
	}

	metrics, _ := font.Metrics(buf, ppem, xfont.HintingNone)
	bounds, _ := font.Bounds(buf, ppem, xfont.HintingNone)
	descriptor := &semantic.FontDescriptor{
		FontName:    baseName,
		Flags:       4, // Assume non-symbolic TrueType
		ItalicAngle: italicAngle(font),
		Ascent:      scaleFixed(metrics.Ascent, unitsPerEm),
		Descent:     scaleFixed(metrics.Descent, unitsPerEm),
		CapHeight:   scaleFixed(metrics.Ascent, unitsPerEm),
		StemV:       80,
		FontBBox: [4]float64{
			scaleFixed(bounds.Min.X, unitsPerEm),
			scaleFixed(bounds.Min.Y, unitsPerEm),
			scaleFixed(bounds.Max.X, unitsPerEm),
			scaleFixed(bounds.Max.Y, unitsPerEm),
		},
		FontFile:     data,
		FontFileType: "FontFile2",
	}

	cidInfo := semantic.CIDSystemInfo{Registry: "Adobe", Ordering: "Identity", Supplement: 0}
	descendant := &semantic.CIDFont{
		Subtype:       "CIDFontType2",
		BaseFont:      baseName,
		CIDSystemInfo: cidInfo,
		DW:            defaultWidth,
		W:             widths,
		Descriptor:    descriptor,
	}

	return &semantic.Font{
		Subtype:        "Type0",
		BaseFont:       baseName,
		Encoding:       "Identity-H",
		Widths:         widths,
		CIDSystemInfo:  &cidInfo,
		DescendantFont: descendant,
		Descriptor:     descriptor,
	}, nil
}

func glyphWidths(font *sfnt.Font, buf *sfnt.Buffer, unitsPerEm sfnt.Units, ppem fixed.Int26_6) map[int]int {
	glyphs := font.NumGlyphs()
	widths := make(map[int]int, glyphs)
	for i := 0; i < int(glyphs); i++ {
		adv, err := font.GlyphAdvance(buf, sfnt.GlyphIndex(i), ppem, xfont.HintingNone)
		if err != nil {
			continue
		}
		widths[i] = int(math.Round(scaleFixed(adv, unitsPerEm)))
	}
	return widths
}

func italicAngle(font *sfnt.Font) float64 {
	post := font.PostTable()
	if post == nil {
		return 0
	}
	return post.ItalicAngle
}

func scaleFixed(val fixed.Int26_6, unitsPerEm sfnt.Units) float64 {
	return float64(val) * 1000.0 / (64.0 * float64(unitsPerEm))
}
