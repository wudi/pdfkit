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
				stream.Operations = append(stream.Operations[:index], stream.Operations[index+1:]...)
			}
		}
		
		// Mark page as dirty? The semantic model has Dirty flags.
		// page.Dirty = true // Assuming we have access or method
	}
	
	return nil
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
