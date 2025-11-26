package editor

import (
	"context"

	"github.com/wudi/pdfkit/ir/semantic"
)

// Editor provides high-level content editing capabilities.
type Editor interface {
	// RemoveRect removes all content within the specified rectangle.
	// This is useful for redaction.
	// It also updates the StructureTree to remove references to deleted content (MCIDs).
	RemoveRect(ctx context.Context, doc *semantic.Document, page *semantic.Page, rect semantic.Rectangle) error

	// ReplaceText replaces occurrences of oldText with newText.
	// Note: This is a complex operation that may require font subsetting adjustments
	// and layout recalculation.
	ReplaceText(ctx context.Context, page *semantic.Page, oldText, newText string) error
}

// SpatialIndex indexes content stream operations by their bounding box.
type SpatialIndex interface {
	// Index indexes the operations of a page.
	Index(ops []semantic.Operation, resources *semantic.Resources) error

	// Query returns operations that intersect with the given rectangle.
	Query(rect semantic.Rectangle) []int // Returns indices of operations
}
