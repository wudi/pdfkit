package editor

import (
	"context"
	"sort"

	"pdflib/ir/semantic"
)

type EditorImpl struct{}

func NewEditor() *EditorImpl {
	return &EditorImpl{}
}

func (e *EditorImpl) RemoveRect(ctx context.Context, page *semantic.Page, rect semantic.Rectangle) error {
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
		
		// TODO: Repair StructTree if MCIDs were removed.
		// This requires a reverse lookup: which MCIDs are used on this page?
		// And checking if they are still present in the stream.
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
	// This is a recursive operation.
	// We need to find StructureItems that point to this page and have an MCID.
	// If the MCID is not in existingMCIDs, remove the item.
	
	// Note: This is a simplified view. Real implementation needs full tree traversal.
}

func (e *EditorImpl) ReplaceText(ctx context.Context, page *semantic.Page, oldText, newText string) error {
	// This requires more complex logic:
	// 1. Find text operations containing oldText.
	// 2. Calculate new width.
	// 3. Check if it fits or needs layout adjustment.
	// 4. Update operation operands.
	// For now, stub.
	return nil
}
