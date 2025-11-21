package fonts

import (
	"unicode/utf16"

	"github.com/go-text/typesetting/language"

	"github.com/wudi/pdfkit/ir/semantic"
)

// Analyzer identifies used glyphs in a document.
type Analyzer struct {
	// Map of font -> set of used CIDs/codes
	UsedGlyphs map[*semantic.Font]map[int]bool
	// TextRuns records per-font script-aware runs derived from ToUnicode mappings.
	TextRuns map[*semantic.Font][]TextRun
}

// TextRun stores a contiguous set of runes that share the same script.
type TextRun struct {
	Runes  []rune
	Script language.Script
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{
		UsedGlyphs: make(map[*semantic.Font]map[int]bool),
		TextRuns:   make(map[*semantic.Font][]TextRun),
	}
}

func (a *Analyzer) Analyze(doc *semantic.Document) {
	for _, page := range doc.Pages {
		a.analyzePage(page)
	}
}

func (a *Analyzer) analyzePage(page *semantic.Page) {
	var currentFont *semantic.Font

	for _, stream := range page.Contents {
		for _, op := range stream.Operations {
			switch op.Operator {
			case "Tf":
				if len(op.Operands) > 0 {
					if name, ok := op.Operands[0].(semantic.NameOperand); ok {
						// Remove leading slash if present (though NameOperand usually has it stripped or not?)
						// semantic.NameOperand.Value usually stores the name without slash if it came from builder?
						// But parser might keep it.
						// Let's assume it matches the key in Resources.
						fontName := name.Value
						if page.Resources != nil && page.Resources.Fonts != nil {
							currentFont = page.Resources.Fonts[fontName]
						}
					}
				}
			case "Tj", "'", "\"":
				if currentFont != nil && len(op.Operands) > 0 {
					if str, ok := op.Operands[0].(semantic.StringOperand); ok {
						a.recordGlyphs(currentFont, str.Value)
					}
				}
			case "TJ":
				if currentFont != nil && len(op.Operands) > 0 {
					if arr, ok := op.Operands[0].(semantic.ArrayOperand); ok {
						for _, item := range arr.Values {
							if str, ok := item.(semantic.StringOperand); ok {
								a.recordGlyphs(currentFont, str.Value)
							}
						}
					}
				}
			}
		}
	}
}

func (a *Analyzer) recordGlyphs(font *semantic.Font, data []byte) {
	if a.UsedGlyphs[font] == nil {
		a.UsedGlyphs[font] = make(map[int]bool)
	}

	if font.Subtype == "Type0" && (font.Encoding == "Identity-H" || font.Encoding == "Identity-V") {
		// 2-byte CIDs
		for i := 0; i < len(data); i += 2 {
			if i+1 < len(data) {
				cid := int(data[i])<<8 | int(data[i+1])
				a.UsedGlyphs[font][cid] = true
			}
		}
	} else {
		// Single byte codes
		for _, b := range data {
			a.UsedGlyphs[font][int(b)] = true
		}
	}

	a.recordTextRuns(font, data)
}

func (a *Analyzer) recordTextRuns(font *semantic.Font, data []byte) {
	runes := decodeRunes(font, data)
	if len(runes) == 0 {
		return
	}
	var current TextRun
	for _, r := range runes {
		scr := language.LookupScript(r)
		if len(current.Runes) == 0 {
			current.Script = fallbackScript(scr)
		} else if scr != 0 && scr != current.Script {
			a.appendRun(font, current)
			current = TextRun{Script: fallbackScript(scr)}
		}
		if len(current.Runes) == 0 {
			current.Script = fallbackScript(scr)
		}
		current.Runes = append(current.Runes, r)
	}
	if len(current.Runes) > 0 {
		a.appendRun(font, current)
	}
}

func (a *Analyzer) appendRun(font *semantic.Font, run TextRun) {
	if len(run.Runes) == 0 {
		return
	}
	buf := make([]rune, len(run.Runes))
	copy(buf, run.Runes)
	a.TextRuns[font] = append(a.TextRuns[font], TextRun{Runes: buf, Script: run.Script})
}

func decodeRunes(font *semantic.Font, data []byte) []rune {
	if font == nil || len(data) == 0 {
		return nil
	}
	if font.Subtype == "Type0" && (font.Encoding == "Identity-H" || font.Encoding == "Identity-V") {
		return decodeCIDRunes(font, data)
	}
	return decodeByteRunes(font, data)
}

func decodeCIDRunes(font *semantic.Font, data []byte) []rune {
	runes := make([]rune, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		cid := int(data[i])<<8 | int(data[i+1])
		if font.ToUnicode != nil {
			if mapping, ok := font.ToUnicode[cid]; ok {
				runes = append(runes, mapping...)
				continue
			}
		}
		// Fall back to direct rune using CID to UTF-16 code unit mapping if possible
		cp := utf16.Decode([]uint16{uint16(cid)})
		if len(cp) > 0 {
			runes = append(runes, cp...)
		}
	}
	return runes
}

func decodeByteRunes(font *semantic.Font, data []byte) []rune {
	runes := make([]rune, 0, len(data))
	for _, b := range data {
		code := int(b)
		if font.ToUnicode != nil {
			if mapping, ok := font.ToUnicode[code]; ok {
				runes = append(runes, mapping...)
				continue
			}
		}
		runes = append(runes, rune(code))
	}
	return runes
}

func fallbackScript(scr language.Script) language.Script {
	if scr != 0 {
		return scr
	}
	return language.LookupScript('a')
}
