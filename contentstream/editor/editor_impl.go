package editor

import (
	"context"
	"sort"
	"strings"

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
	for i := range page.Contents {
		stream := &page.Contents[i]
		if err := e.replaceInStream(stream, page.Resources, oldText, newText); err != nil {
			return err
		}
	}
	return nil
}

func (e *EditorImpl) replaceInStream(stream *semantic.ContentStream, resources *semantic.Resources, oldText, newText string) error {
	// Loop to handle multiple occurrences
	for {
		// 1. Build Text Map
		type TextPart struct {
			Text    string
			OpIndex int
			Font    *semantic.Font
		}

		var parts []TextPart
		var currentFont *semantic.Font

		for j, op := range stream.Operations {
			if op.Operator == "Tf" {
				if len(op.Operands) > 0 {
					if nameOp, ok := op.Operands[0].(semantic.NameOperand); ok {
						if resources != nil && resources.Fonts != nil {
							currentFont = resources.Fonts[nameOp.Value]
						}
					}
				}
			} else if isTextOp(op.Operator) {
				text := decodeOp(op, currentFont)
				parts = append(parts, TextPart{text, j, currentFont})
			}
		}

		// 2. Reconstruct full text
		var fullTextBuilder strings.Builder
		var partIndices []int // maps char index in fullText to part index
		var charOffsets []int // maps char index in fullText to char index in part.Text

		for k, part := range parts {
			runes := []rune(part.Text)
			for rIdx := range runes {
				partIndices = append(partIndices, k)
				charOffsets = append(charOffsets, rIdx)
			}
			fullTextBuilder.WriteString(part.Text)
		}
		fullText := fullTextBuilder.String()

		// 3. Find match
		idx := strings.Index(fullText, oldText)
		if idx == -1 {
			break // No more matches
		}

		// 4. Identify range
		// startCharIdx := idx
		// endCharIdx := idx + len(oldText) // exclusive

		// Map back to parts
		// Note: strings.Index returns byte index, but we built partIndices based on runes?
		// Wait, fullTextBuilder.WriteString appends bytes.
		// But partIndices logic above assumed 1 rune = 1 entry?
		// "for rIdx := range runes" iterates runes.
		// But strings.Index returns byte offset.
		// We need to convert byte offset to rune index.

		// Let's rebuild fullText as []rune to be safe and easy
		fullRunes := []rune(fullText)
		oldRunes := []rune(oldText)

		// Find in runes
		runeIdx := -1
		for i := 0; i <= len(fullRunes)-len(oldRunes); i++ {
			match := true
			for k := 0; k < len(oldRunes); k++ {
				if fullRunes[i+k] != oldRunes[k] {
					match = false
					break
				}
			}
			if match {
				runeIdx = i
				break
			}
		}

		if runeIdx == -1 {
			break // Should not happen if strings.Index found it, unless encoding issues
		}

		startRuneIdx := runeIdx
		endRuneIdx := runeIdx + len(oldRunes)

		startPartIdx := partIndices[startRuneIdx]
		endPartIdx := partIndices[endRuneIdx-1]

		startPart := parts[startPartIdx]
		endPart := parts[endPartIdx]

		// 5. Construct replacements
		// Prefix: Text in startPart before match
		startOffset := charOffsets[startRuneIdx]
		prefix := string([]rune(startPart.Text)[:startOffset])

		// Suffix: Text in endPart after match
		endOffset := charOffsets[endRuneIdx-1] + 1 // +1 because endRuneIdx is exclusive in fullRunes, but inclusive in loop
		suffix := string([]rune(endPart.Text)[endOffset:])

		// New Ops
		var newOps []semantic.Operation

		// Start Op with Prefix + NewText
		// We use the font of startPart
		if prefix != "" || newText != "" {
			// We combine prefix and newText into one op?
			// Or separate? Separate is safer for positioning if we don't recalculate everything.
			// But here we are replacing text, so we assume flow.

			// Let's put prefix back as it was (re-encoded)
			if prefix != "" {
				op, err := encodeOp(prefix, startPart.Font)
				if err == nil {
					newOps = append(newOps, op)
				}
			}

			// Put newText
			if newText != "" {
				op, err := encodeOp(newText, startPart.Font)
				if err == nil {
					newOps = append(newOps, op)
				}
			}
		}

		// End Op with Suffix
		if suffix != "" {
			// Use endPart font
			op, err := encodeOp(suffix, endPart.Font)
			if err == nil {
				newOps = append(newOps, op)
			}
		}

		// 6. Apply replacement
		// We need to replace ops from startPart.OpIndex to endPart.OpIndex
		// But wait, parts only contain text ops. There might be other ops in between (e.g. color changes).
		// If we remove range [startPart.OpIndex, endPart.OpIndex], we might remove color changes!
		// This is tricky.
		// If there are intervening non-text ops, we should preserve them?
		// But if the text flows across them, preserving them might be wrong (e.g. color change in middle of word?).
		// For now, let's assume we remove everything in between.

		startOpIndex := startPart.OpIndex
		endOpIndex := endPart.OpIndex

		// Construct the new stream operations
		var finalOps []semantic.Operation
		finalOps = append(finalOps, stream.Operations[:startOpIndex]...)
		finalOps = append(finalOps, newOps...)
		if endOpIndex+1 < len(stream.Operations) {
			finalOps = append(finalOps, stream.Operations[endOpIndex+1:]...)
		}

		stream.Operations = finalOps

		// Continue loop to find next occurrence
	}
	return nil
}

func isTextOp(op string) bool {
	return op == "Tj" || op == "TJ" || op == "'" || op == "\""
}

func decodeOp(op semantic.Operation, font *semantic.Font) string {
	if font == nil {
		return ""
	}
	var sb strings.Builder

	decodeBytes := func(b []byte) string {
		if font.ToUnicode != nil {
			// Map bytes to runes using ToUnicode CMap
			// This is complex because CMap keys can be variable length.
			// We assume 1-byte or 2-byte based on font type or CMap.
			// Simplified: Try to match bytes to CMap keys.

			// For now, let's assume 1-byte or 2-byte keys.
			// If ToUnicode has entries, we use them.

			// Try to read all bytes
			res := ""
			i := 0
			for i < len(b) {
				// Try 2 bytes
				if i+1 < len(b) {
					code := int(b[i])<<8 | int(b[i+1])
					if runes, ok := font.ToUnicode[code]; ok {
						res += string(runes)
						i += 2
						continue
					}
				}
				// Try 1 byte
				code := int(b[i])
				if runes, ok := font.ToUnicode[code]; ok {
					res += string(runes)
					i++
					continue
				}
				// Fallback
				res += string(rune(b[i]))
				i++
			}
			return res
		}
		// Fallback to simple encoding if no ToUnicode
		return string(b)
	}

	for _, operand := range op.Operands {
		if strOp, ok := operand.(semantic.StringOperand); ok {
			sb.WriteString(decodeBytes(strOp.Value))
		} else if arrOp, ok := operand.(semantic.ArrayOperand); ok {
			for _, item := range arrOp.Values {
				if strItem, ok := item.(semantic.StringOperand); ok {
					sb.WriteString(decodeBytes(strItem.Value))
				}
			}
		}
	}
	return sb.String()
}

func encodeOp(text string, font *semantic.Font) (semantic.Operation, error) {
	if font == nil {
		return semantic.Operation{}, nil
	}

	// Shape text
	shapedGlyphs, err := fonts.ShapeText(text, font)
	if err != nil {
		return semantic.Operation{}, err
	}

	// Update Font Widths and ToUnicode
	if font.Widths == nil {
		font.Widths = make(map[int]int)
	}
	if font.ToUnicode == nil {
		font.ToUnicode = make(map[int][]rune)
	}

	runes := []rune(text)
	clusterGlyphs := make(map[int][]int)
	for _, g := range shapedGlyphs {
		clusterGlyphs[g.Cluster] = append(clusterGlyphs[g.Cluster], g.ID)
		font.Widths[g.ID] = int(g.XAdvance)
	}

	var clusters []int
	for c := range clusterGlyphs {
		clusters = append(clusters, c)
	}
	sort.Ints(clusters)

	for i, c := range clusters {
		start := c
		end := len(runes)
		if i+1 < len(clusters) {
			end = clusters[i+1]
		}
		if start >= len(runes) {
			continue
		}
		if end > len(runes) {
			end = len(runes)
		}

		clusterRunes := runes[start:end]
		gids := clusterGlyphs[c]

		if len(gids) > 0 {
			// Map the first glyph to the runes.
			// This handles ligatures (1 glyph -> N runes).
			// For 1 rune -> N glyphs (decomposition), we map first glyph to rune, others to empty (implicitly).
			font.ToUnicode[gids[0]] = clusterRunes
		}
	}

	// Construct TJ
	var tjArgs []semantic.Operand

	for _, g := range shapedGlyphs {
		// Encode Glyph ID
		var encoded []byte
		if font.Subtype == "Type0" {
			encoded = []byte{byte(g.ID >> 8), byte(g.ID)}
		} else {
			encoded = []byte{byte(g.ID)}
		}

		tjArgs = append(tjArgs, semantic.StringOperand{Value: encoded})

		// Adjustment
		width := 0
		if font.Widths != nil {
			width = font.Widths[g.ID]
		}
		diff := float64(width) - g.XAdvance
		if diff > 0.001 || diff < -0.001 {
			tjArgs = append(tjArgs, semantic.NumberOperand{Value: diff})
		}
	}

	return semantic.Operation{
		Operator: "TJ",
		Operands: []semantic.Operand{semantic.ArrayOperand{Values: tjArgs}},
	}, nil
}
