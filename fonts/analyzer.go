package fonts

import (
	"pdflib/ir/semantic"
)

// Analyzer identifies used glyphs in a document.
type Analyzer struct {
	// Map of font -> set of used CIDs/codes
	UsedGlyphs map[*semantic.Font]map[int]bool
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{
		UsedGlyphs: make(map[*semantic.Font]map[int]bool),
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
}
