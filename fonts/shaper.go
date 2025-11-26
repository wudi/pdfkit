package fonts

import (
	"bytes"

	"strings"

	"github.com/go-text/typesetting/di"
	gofont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/wudi/pdfkit/ir/semantic"
)

// ShapedGlyph represents a single shaped glyph with positioning information.
type ShapedGlyph struct {
	ID       int
	XAdvance float64 // In PDF text units (1/1000 em)
	YAdvance float64
	XOffset  float64
	YOffset  float64
}

// ShapeText shapes the given text using the provided font and returns the glyphs and positioning.
func ShapeText(text string, font *semantic.Font) ([]ShapedGlyph, error) {
	if font == nil || font.Descriptor == nil || len(font.Descriptor.FontFile) == 0 {
		return nil, nil
	}

	face, err := gofont.ParseTTF(bytes.NewReader(font.Descriptor.FontFile))
	if err != nil {
		return nil, err
	}

	shaper := &shaping.HarfbuzzShaper{}
	runes := []rune(text)

	// Detect script
	script := detectScript(runes)

	dir := scriptDirection(script)
	if strings.HasSuffix(font.Encoding, "V") {
		dir = di.DirectionTTB
	}

	// Use a standard size for shaping, we'll normalize to 1000 units later
	// 1000 * 64 (fixed point)
	size := fixed.Int26_6(1000 * 64)

	input := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: dir,
		Face:      face,
		Size:      size,
		Script:    script,
		Language:  language.DefaultLanguage(),
	}

	output := shaper.Shape(input)

	var result []ShapedGlyph
	for _, g := range output.Glyphs {
		// Convert fixed point to float
		// The size was 1000, so the values are already in 1/1000 em units relative to the font size?
		// Wait, if I set size to 1000, then the output values are in 1/64th of a unit?
		// No, fixed.Int26_6 is 26 bits integer, 6 bits fraction.
		// Value 1.0 is 64.
		// If I set size to 1000 * 64, then 1 em = 1000 units.
		// The output advances are in fixed point units.
		// So if advance is 64000, that's 1000 units.

		xAdv := float64(g.XAdvance) / 64.0
		yAdv := float64(g.YAdvance) / 64.0
		xOff := float64(g.XOffset) / 64.0
		yOff := float64(g.YOffset) / 64.0

		result = append(result, ShapedGlyph{
			ID:       int(g.GlyphID),
			XAdvance: xAdv,
			YAdvance: yAdv,
			XOffset:  xOff,
			YOffset:  yOff,
		})
	}

	return result, nil
}

// shapeRunsForFont returns the glyph IDs referenced when the given runs are shaped with go-text/typesetting.
func shapeRunsForFont(font *semantic.Font, runs []TextRun) map[int]bool {
	if font == nil || font.Descriptor == nil || len(font.Descriptor.FontFile) == 0 {
		return nil
	}
	if len(runs) == 0 {
		return nil
	}

	face, err := gofont.ParseTTF(bytes.NewReader(font.Descriptor.FontFile))
	if err != nil {
		return nil
	}

	shaper := &shaping.HarfbuzzShaper{}
	glyphs := make(map[int]bool)
	size := fixed.Int26_6(64)

	for _, run := range runs {
		if len(run.Runes) == 0 {
			continue
		}
		dir := scriptDirection(run.Script)
		input := shaping.Input{
			Text:      run.Runes,
			RunStart:  0,
			RunEnd:    len(run.Runes),
			Direction: dir,
			Face:      face,
			Size:      size,
			Script:    run.Script,
			Language:  language.DefaultLanguage(),
		}
		output := shaper.Shape(input)
		for _, g := range output.Glyphs {
			glyphs[int(g.GlyphID)] = true
		}
	}

	if len(glyphs) == 0 {
		return nil
	}
	return glyphs
}

func scriptDirection(script language.Script) di.Direction {
	switch script {
	case language.Arabic, language.Hebrew, language.Syriac, language.Thaana, language.Nko:
		return di.DirectionRTL
	default:
		return di.DirectionLTR
	}
}

func detectScript(runes []rune) language.Script {
	for _, r := range runes {
		switch {
		case r >= 0x0600 && r <= 0x06FF:
			return language.Arabic
		case r >= 0x0590 && r <= 0x05FF:
			return language.Hebrew
		case r >= 0x0700 && r <= 0x074F:
			return language.Syriac
		case r >= 0x0780 && r <= 0x07BF:
			return language.Thaana
		case r >= 0x07C0 && r <= 0x07FF:
			return language.Nko
		case r >= 0x0900 && r <= 0x097F:
			return language.Devanagari
		case r >= 0x0980 && r <= 0x09FF:
			return language.Bengali
		case r >= 0x0A00 && r <= 0x0A7F:
			return language.Gurmukhi
		case r >= 0x0A80 && r <= 0x0AFF:
			return language.Gujarati
		case r >= 0x0B00 && r <= 0x0B7F:
			return language.Oriya
		case r >= 0x0B80 && r <= 0x0BFF:
			return language.Tamil
		case r >= 0x0C00 && r <= 0x0C7F:
			return language.Telugu
		case r >= 0x0C80 && r <= 0x0CFF:
			return language.Kannada
		case r >= 0x0D00 && r <= 0x0D7F:
			return language.Malayalam
		case r >= 0x0D80 && r <= 0x0DFF:
			return language.Sinhala
		case r >= 0x0E00 && r <= 0x0E7F:
			return language.Thai
		case r >= 0x0E80 && r <= 0x0EFF:
			return language.Lao
		case r >= 0x0F00 && r <= 0x0FFF:
			return language.Tibetan
		case r >= 0x1000 && r <= 0x109F:
			return language.Myanmar
		case r >= 0x1780 && r <= 0x17FF:
			return language.Khmer
		}
	}
	return language.Latin
}
