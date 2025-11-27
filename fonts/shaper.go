package fonts

import (
	"bytes"
	"strings"
	"unicode"

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
	Cluster  int
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
			Cluster:  int(g.ClusterIndex),
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
	counts := make(map[language.Script]int)
	maxCount := 0
	bestScript := language.Latin

	for _, r := range runes {
		script := scriptFromRune(r)
		if script == language.Unknown {
			continue
		}
		counts[script]++
		if counts[script] > maxCount {
			maxCount = counts[script]
			bestScript = script
		}
	}
	return bestScript
}

func scriptFromRune(r rune) language.Script {
	switch {
	case unicode.Is(unicode.Arabic, r):
		return language.Arabic
	case unicode.Is(unicode.Hebrew, r):
		return language.Hebrew
	case unicode.Is(unicode.Latin, r):
		return language.Latin
	case unicode.Is(unicode.Cyrillic, r):
		return language.Cyrillic
	case unicode.Is(unicode.Greek, r):
		return language.Greek
	case unicode.Is(unicode.Thai, r):
		return language.Thai
	case unicode.Is(unicode.Devanagari, r):
		return language.Devanagari
	case unicode.Is(unicode.Bengali, r):
		return language.Bengali
	case unicode.Is(unicode.Gurmukhi, r):
		return language.Gurmukhi
	case unicode.Is(unicode.Gujarati, r):
		return language.Gujarati
	case unicode.Is(unicode.Oriya, r):
		return language.Oriya
	case unicode.Is(unicode.Tamil, r):
		return language.Tamil
	case unicode.Is(unicode.Telugu, r):
		return language.Telugu
	case unicode.Is(unicode.Kannada, r):
		return language.Kannada
	case unicode.Is(unicode.Malayalam, r):
		return language.Malayalam
	case unicode.Is(unicode.Sinhala, r):
		return language.Sinhala
	case unicode.Is(unicode.Lao, r):
		return language.Lao
	case unicode.Is(unicode.Tibetan, r):
		return language.Tibetan
	case unicode.Is(unicode.Myanmar, r):
		return language.Myanmar
	case unicode.Is(unicode.Khmer, r):
		return language.Khmer
	case unicode.Is(unicode.Han, r):
		return language.Han
	case unicode.Is(unicode.Hiragana, r):
		return language.Hiragana
	case unicode.Is(unicode.Katakana, r):
		return language.Katakana
	case unicode.Is(unicode.Hangul, r):
		return language.Hangul
	}
	return language.Unknown
}
