package fonts

import (
	"sort"

	"github.com/wudi/pdfkit/ir/semantic"
)

type Subset struct {
	OriginalToSubset map[int]int // Map original CID -> new CID (identity for now)
	SubsetToOriginal map[int]int // Map new CID -> original CID
	UsedCIDs         []int       // List of used CIDs
	GlyphSet         map[int]bool
}

type Planner struct {
	Subsets map[*semantic.Font]*Subset
}

func NewPlanner() *Planner {
	return &Planner{
		Subsets: make(map[*semantic.Font]*Subset),
	}
}

func (p *Planner) Plan(analyzer *Analyzer) {
	for font, used := range analyzer.UsedGlyphs {
		glyphSet := make(map[int]bool)
		for cid := range used {
			glyphSet[cid] = true
		}
		if shaped := shapeRunsForFont(font, analyzer.TextRuns[font]); len(shaped) > 0 {
			for gid := range shaped {
				glyphSet[gid] = true
			}
		}

		// Compute GSUB closure if font data is available
		if font.Descriptor != nil && len(font.Descriptor.FontFile) > 0 {
			if closure, err := ComputeClosureGSUB(font.Descriptor.FontFile, glyphSet); err == nil {
				for gid := range closure {
					glyphSet[gid] = true
				}
			}
		}

		subset := &Subset{
			OriginalToSubset: make(map[int]int),
			SubsetToOriginal: make(map[int]int),
			GlyphSet:         glyphSet,
		}

		// Renumber CIDs to 0..N to save space.
		// CID 0 is always .notdef and mapped to 0.
		subset.OriginalToSubset[0] = 0
		subset.SubsetToOriginal[0] = 0
		subset.UsedCIDs = append(subset.UsedCIDs, 0)

		// Sort original CIDs to ensure deterministic mapping
		var sortedCIDs []int
		for cid := range glyphSet {
			if cid != 0 {
				sortedCIDs = append(sortedCIDs, cid)
			}
		}
		sort.Ints(sortedCIDs)

		nextCID := 1
		for _, cid := range sortedCIDs {
			subset.OriginalToSubset[cid] = nextCID
			subset.SubsetToOriginal[nextCID] = cid
			subset.UsedCIDs = append(subset.UsedCIDs, nextCID)
			nextCID++
		}
		// UsedCIDs is already sorted 0..N

		p.Subsets[font] = subset
	}
}

// Subsetter applies the subsetting plan to the fonts.
type Subsetter struct{}

func NewSubsetter() *Subsetter {
	return &Subsetter{}
}

func (s *Subsetter) Apply(doc *semantic.Document, planner *Planner) {
	// 1. Update Fonts
	for font, subset := range planner.Subsets {
		// 1.1 Filter Widths (using New CIDs)
		newWidths := make(map[int]int)
		for _, newCID := range subset.UsedCIDs {
			oldCID := subset.SubsetToOriginal[newCID]
			if w, ok := font.Widths[oldCID]; ok {
				newWidths[newCID] = w
			} else {
				// Use default width if available in descendant
				if font.DescendantFont != nil {
					newWidths[newCID] = font.DescendantFont.DW
				}
			}
		}
		font.Widths = newWidths
		if font.DescendantFont != nil {
			font.DescendantFont.W = newWidths
		}

		// 1.2 Filter ToUnicode (using New CIDs)
		if font.ToUnicode != nil {
			newToUnicode := make(map[int][]rune)
			for _, newCID := range subset.UsedCIDs {
				oldCID := subset.SubsetToOriginal[newCID]
				if r, ok := font.ToUnicode[oldCID]; ok {
					newToUnicode[newCID] = r
				}
			}
			font.ToUnicode = newToUnicode
		}

		// 1.3 Generate CIDToGIDMap
		// We need to map NewCID -> OldGID.
		// Since original font was Identity-H, OldCID = OldGID.
		// So we map NewCID -> OldCID.
		if font.DescendantFont != nil {
			cidToGid := make([]byte, len(subset.UsedCIDs)*2)
			for _, newCID := range subset.UsedCIDs {
				oldCID := subset.SubsetToOriginal[newCID]
				cidToGid[newCID*2] = byte(oldCID >> 8)
				cidToGid[newCID*2+1] = byte(oldCID)
			}
			font.DescendantFont.CIDToGIDMap = cidToGid
			font.DescendantFont.CIDToGIDMapName = "" // Use stream, not name
		}

		// 1.4 Subset FontFile
		if font.Descriptor != nil && len(font.Descriptor.FontFile) > 0 && font.Descriptor.FontFileType == "FontFile2" {
			// Identity-H means CID=GID, so the glyph set directly maps to TrueType glyph IDs.
			usedGIDs := make(map[int]bool, len(subset.GlyphSet))
			for gid := range subset.GlyphSet {
				usedGIDs[gid] = true
			}

			newFontData, err := SubsetTrueType(font.Descriptor.FontFile, usedGIDs)
			if err == nil && len(newFontData) < len(font.Descriptor.FontFile) {
				font.Descriptor.FontFile = newFontData
			}
		}
	}

	// 2. Rewrite Content Streams
	for _, page := range doc.Pages {
		for i := range page.Contents {
			stream := &page.Contents[i]
			rewriteContentStream(stream, page.Resources, planner)
		}
	}
}

func rewriteContentStream(stream *semantic.ContentStream, resources *semantic.Resources, planner *Planner) {
	var currentFont *semantic.Font

	for i, op := range stream.Operations {
		if op.Operator == "Tf" {
			if len(op.Operands) > 0 {
				if nameOp, ok := op.Operands[0].(semantic.NameOperand); ok {
					if resources != nil && resources.Fonts != nil {
						currentFont = resources.Fonts[nameOp.Value]
					}
				}
			}
		} else if op.Operator == "Tj" {
			if currentFont != nil {
				if subset, ok := planner.Subsets[currentFont]; ok {
					if len(op.Operands) > 0 {
						if strOp, ok := op.Operands[0].(semantic.StringOperand); ok {
							newData := remapString(strOp.Value, subset)
							stream.Operations[i].Operands[0] = semantic.StringOperand{Value: newData}
						}
					}
				}
			}
		} else if op.Operator == "TJ" {
			if currentFont != nil {
				if subset, ok := planner.Subsets[currentFont]; ok {
					if len(op.Operands) > 0 {
						if arrOp, ok := op.Operands[0].(semantic.ArrayOperand); ok {
							newValues := make([]semantic.Operand, len(arrOp.Values))
							for k, v := range arrOp.Values {
								if strOp, ok := v.(semantic.StringOperand); ok {
									newData := remapString(strOp.Value, subset)
									newValues[k] = semantic.StringOperand{Value: newData}
								} else {
									newValues[k] = v
								}
							}
							stream.Operations[i].Operands[0] = semantic.ArrayOperand{Values: newValues}
						}
					}
				}
			}
		}
	}
}

func remapString(data []byte, subset *Subset) []byte {
	// Assume 2-byte CIDs (Identity-H)
	res := make([]byte, 0, len(data))
	for i := 0; i < len(data); i += 2 {
		if i+1 < len(data) {
			oldCID := int(data[i])<<8 | int(data[i+1])
			if newCID, ok := subset.OriginalToSubset[oldCID]; ok {
				res = append(res, byte(newCID>>8), byte(newCID))
			} else {
				// Should not happen if planner is correct
				res = append(res, data[i], data[i+1])
			}
		} else {
			// Odd byte? Should not happen for Identity-H
			res = append(res, data[i])
		}
	}
	return res
}
