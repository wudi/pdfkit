package fonts

import (
	"bytes"

	"github.com/go-text/typesetting/di"
	gofont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"pdflib/ir/semantic"
)

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
