package editor

import (
	"github.com/wudi/pdfkit/contentstream"
	"github.com/wudi/pdfkit/ir/semantic"
)

type OpSpatialIndex struct {
	tree *QuadTree
}

func NewOpSpatialIndex(pageBounds semantic.Rectangle) *OpSpatialIndex {
	return &OpSpatialIndex{
		tree: NewQuadTree(pageBounds, 10),
	}
}

func (idx *OpSpatialIndex) Index(ops []semantic.Operation, resources *semantic.Resources) error {
	tracer := contentstream.NewTracer()
	bboxes, err := tracer.Trace(ops, resources)
	if err != nil {
		return err
	}

	for _, bbox := range bboxes {
		idx.tree.Insert(bbox.Rect, bbox.OpIndex)
	}
	return nil
}

func (idx *OpSpatialIndex) Query(rect semantic.Rectangle) []int {
	return idx.tree.Query(rect)
}
