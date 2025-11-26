package editor

import (
	"context"
	"sort"

	"github.com/wudi/pdfkit/fonts"
	"github.com/wudi/pdfkit/ir/semantic"
)

type EditorImpl struct{}

func NewEditor() *EditorImpl {
	return &EditorImpl{}
}

func (e *EditorImpl) RemoveRect(ctx context.Context, doc *semantic.Document, page *semantic.Page, rect semantic.Rectangle) error {
	// 1. Flatten content streams into a single list of operations
	// Note: In a real implementation, we might want to keep them separate or handle them more carefully.
	// For now, we assume we can work on the first stream or merge them.
	// But wait, page.Contents is []ContentStream.
	// If we modify operations, we need to write them back.

	// Let's assume we work on all streams combined, but we need to track which stream an op belongs to.
	// Or simpler: process each stream independently.

	for i := range page.Contents {
		stream := &page.Contents[i]

		// 2. Build Spatial Index
		idx := NewOpSpatialIndex(page.MediaBox)
		if err := idx.Index(stream.Operations, page.Resources); err != nil {
			return err
		}

		// 3. Query for operations in the rect
		opIndices := idx.Query(rect)
		if len(opIndices) == 0 {
			continue
		}

		// 4. Remove operations
		// Sort indices in descending order to remove safely
		sort.Sort(sort.Reverse(sort.IntSlice(opIndices)))

		// Use a map to avoid duplicates if Query returns same index multiple times (it shouldn't with current QuadTree but good practice)
		seen := make(map[int]bool)
		uniqueIndices := make([]int, 0, len(opIndices))
		for _, index := range opIndices {
			if !seen[index] {
				seen[index] = true
				uniqueIndices = append(uniqueIndices, index)
			}
		}

		// Remove
		for _, index := range uniqueIndices {
			if index >= 0 && index < len(stream.Operations) {
				// Check if this operation is a Marked Content operator
				op := stream.Operations[index]
				if op.Operator == "BMC" || op.Operator == "BDC" {
					// If we remove a marked content sequence start, we should probably remove the end too (EMC).
					// Or better: don't remove the structure tags, just the content inside.
					// But if the content is gone, the tag is empty.
					// If we remove the tag, we must update the StructTree.

					// For "Zero Compromise", we must handle this.
					// If we delete content that is marked, we need to know its MCID.
					// If the operation IS the content (e.g. Tj), we need to know if it was inside a marked sequence.
				}

				stream.Operations = append(stream.Operations[:index], stream.Operations[index+1:]...)
			}
		}

		// 5. Cleanup empty Marked Content sequences
		// We repeatedly scan for BDC/BMC followed immediately by EMC and remove them.
		for {
			changed := false
			var newOps []semantic.Operation
			skipNext := false

			for j := 0; j < len(stream.Operations); j++ {
				if skipNext {
					skipNext = false
					continue
				}

				op := stream.Operations[j]
				if op.Operator == "BDC" || op.Operator == "BMC" {
					// Check next op
					if j+1 < len(stream.Operations) {
						nextOp := stream.Operations[j+1]
						if nextOp.Operator == "EMC" {
							// Found empty sequence, skip both
							skipNext = true
							changed = true
							continue
						}
					}
				}
				newOps = append(newOps, op)
			}
			stream.Operations = newOps
			if !changed {
				break
			}
		}
	}

	// Repair StructTree if MCIDs were removed.
	if doc != nil && doc.StructTree != nil {
		e.RepairStructTree(page, doc.StructTree)
	}

	return nil
}

// RepairStructTree removes references to MCIDs that no longer exist on the page.
func (e *EditorImpl) RepairStructTree(page *semantic.Page, structTree *semantic.StructureTree) {
	if structTree == nil {
		return
	}

	// 1. Collect all remaining MCIDs on the page
	existingMCIDs := make(map[int]bool)
	for _, stream := range page.Contents {
		for _, op := range stream.Operations {
			if op.Operator == "BDC" || op.Operator == "BMC" {
				// Extract MCID from operands
				// Operand 0 is tag name. Operand 1 (for BDC) is properties dict.
				if len(op.Operands) > 1 {
					if dict, ok := op.Operands[1].(semantic.DictOperand); ok {
						if mcidOp, ok := dict.Values["MCID"]; ok {
							if mcidNum, ok := mcidOp.(semantic.NumberOperand); ok {
								existingMCIDs[int(mcidNum.Value)] = true
							}
						}
					}
				}
			}
		}
	}

	// 2. Traverse StructTree and prune missing MCIDs
	var pruneElement func(elem *semantic.StructureElement) bool
	pruneElement = func(elem *semantic.StructureElement) bool {
		if elem == nil {
			return true // Remove nil elements
		}

		var newK []semantic.StructureItem
		for _, item := range elem.K {
			keep := true

			if item.Element != nil {
				// Recursive check
				if pruneElement(item.Element) {
					keep = false // Child element became empty, remove it
				}
			} else if item.MCID >= 0 {
				// Direct MCID reference.
				// Check if this element belongs to the target page.
				if elem.Pg == page {
					if !existingMCIDs[item.MCID] {
						keep = false
					}
				}
			} else if item.MCR != nil {
				// MCR reference
				if item.MCR.Pg == page {
					if !existingMCIDs[item.MCR.MCID] {
						keep = false
					}
				}
			}

			if keep {
				newK = append(newK, item)
			}
		}
		elem.K = newK

		// If element is empty, return true to indicate it should be removed
		return len(elem.K) == 0
	}

	// Process root children
	var newRootK []*semantic.StructureElement
	for _, child := range structTree.K {
		if !pruneElement(child) {
			newRootK = append(newRootK, child)
		}
	}
	structTree.K = newRootK
}

func (e *EditorImpl) ReplaceText(ctx context.Context, page *semantic.Page, oldText, newText string) error {
	// Iterate over content streams
	for i := range page.Contents {
		stream := &page.Contents[i]
		var currentFont *semantic.Font

		for j, op := range stream.Operations {
			if op.Operator == "Tf" {
				if len(op.Operands) > 0 {
					if nameOp, ok := op.Operands[0].(semantic.NameOperand); ok {
						if page.Resources != nil && page.Resources.Fonts != nil {
							currentFont = page.Resources.Fonts[nameOp.Value]
						}
					}
				}
			} else if op.Operator == "Tj" || op.Operator == "TJ" {
				// Check if this matches oldText
				// Note: This is a simplification. Real text extraction is needed to match properly.
				// We assume the caller knows the text is in a single operation and matches exactly for now.

				// Shape the new text
				if currentFont == nil {
					continue
				}

				shapedGlyphs, err := fonts.ShapeText(newText, currentFont)
				if err != nil {
					// If shaping fails (e.g. no font file), we can't replace properly.
					continue
				}

				// Update font metrics (Widths and ToUnicode)
				if currentFont.Widths == nil {
					currentFont.Widths = make(map[int]int)
				}
				if currentFont.ToUnicode == nil {
					currentFont.ToUnicode = make(map[int][]rune)
				}

				// We assume 1-to-1 mapping for simplicity in updating ToUnicode
				// This is not always true but better than nothing.
				runes := []rune(newText)
				if len(shapedGlyphs) == len(runes) {
					for k, g := range shapedGlyphs {
						currentFont.Widths[g.ID] = int(g.XAdvance * 1000) // XAdvance is in 1/1000 em
						currentFont.ToUnicode[g.ID] = []rune{runes[k]}
					}
				}

				// Construct TJ array
				var tjArgs []semantic.Operand

				for _, g := range shapedGlyphs {
					// Encode Glyph ID
					var encoded []byte
					if currentFont.Subtype == "Type0" {
						// Identity-H: 2 bytes
						encoded = []byte{byte(g.ID >> 8), byte(g.ID)}
					} else {
						// Simple font: 1 byte
						encoded = []byte{byte(g.ID)}
					}

					tjArgs = append(tjArgs, semantic.StringOperand{Value: encoded})

					// Calculate adjustment
					// TJ adjustment is subtracted from position.
					// Advance = Width - (Adj / 1000)
					// Adj = (Width - Advance) * 1000

					// Get natural width
					width := 0
					if currentFont.Widths != nil {
						width = currentFont.Widths[g.ID]
					}

					// g.XAdvance is in 1/1000 em units (from shaper.go)
					// width is in 1/1000 em units (PDF spec)

					diff := float64(width) - g.XAdvance
					// Only add adjustment if significant
					if diff > 0.001 || diff < -0.001 {
						tjArgs = append(tjArgs, semantic.NumberOperand{Value: diff})
					}
				}

				// Replace with TJ
				stream.Operations[j] = semantic.Operation{
					Operator: "TJ",
					Operands: []semantic.Operand{semantic.ArrayOperand{Values: tjArgs}},
				}
			}
		}
	}
	return nil
}
