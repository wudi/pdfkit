package editor

import (
	"github.com/wudi/pdfkit/ir/semantic"
)

// QuadTree implements a spatial index for rectangles.
type QuadTree struct {
	Bounds   semantic.Rectangle
	Capacity int
	Points   []PointData
	Nodes    []*QuadTree
}

type PointData struct {
	Rect  semantic.Rectangle
	Index int
}

func NewQuadTree(bounds semantic.Rectangle, capacity int) *QuadTree {
	return &QuadTree{
		Bounds:   bounds,
		Capacity: capacity,
		Points:   make([]PointData, 0, capacity),
	}
}

func (qt *QuadTree) Insert(rect semantic.Rectangle, index int) bool {
	if !intersects(qt.Bounds, rect) {
		return false
	}

	if qt.Nodes != nil {
		// Try to fit in children
		for _, node := range qt.Nodes {
			if contains(node.Bounds, rect) {
				if node.Insert(rect, index) {
					return true
				}
			}
		}
	}

	// If we are here, either it's a leaf, or it doesn't fit in any child (overlaps)
	// Check capacity if leaf
	if qt.Nodes == nil {
		if len(qt.Points) < qt.Capacity {
			qt.Points = append(qt.Points, PointData{Rect: rect, Index: index})
			return true
		}
		// Split and redistribute
		qt.subdivide()
		// Re-insert current points
		oldPoints := qt.Points
		qt.Points = make([]PointData, 0, qt.Capacity)
		for _, p := range oldPoints {
			qt.Insert(p.Rect, p.Index)
		}
		// Try inserting the new one again
		return qt.Insert(rect, index)
	}

	// It's a node (not leaf), and rect didn't fit in children, so it belongs here
	qt.Points = append(qt.Points, PointData{Rect: rect, Index: index})
	return true
}

func (qt *QuadTree) subdivide() {
	xMid := (qt.Bounds.LLX + qt.Bounds.URX) / 2
	yMid := (qt.Bounds.LLY + qt.Bounds.URY) / 2

	qt.Nodes = []*QuadTree{
		NewQuadTree(semantic.Rectangle{LLX: qt.Bounds.LLX, LLY: yMid, URX: xMid, URY: qt.Bounds.URY}, qt.Capacity), // Top-Left
		NewQuadTree(semantic.Rectangle{LLX: xMid, LLY: yMid, URX: qt.Bounds.URX, URY: qt.Bounds.URY}, qt.Capacity), // Top-Right
		NewQuadTree(semantic.Rectangle{LLX: qt.Bounds.LLX, LLY: qt.Bounds.LLY, URX: xMid, URY: yMid}, qt.Capacity), // Bottom-Left
		NewQuadTree(semantic.Rectangle{LLX: xMid, LLY: qt.Bounds.LLY, URX: qt.Bounds.URX, URY: yMid}, qt.Capacity), // Bottom-Right
	}
}

func (qt *QuadTree) Query(rangeRect semantic.Rectangle) []int {
	var found []int
	if !intersects(qt.Bounds, rangeRect) {
		return found
	}

	for _, p := range qt.Points {
		if intersects(p.Rect, rangeRect) {
			found = append(found, p.Index)
		}
	}

	if qt.Nodes != nil {
		for _, node := range qt.Nodes {
			found = append(found, node.Query(rangeRect)...)
		}
	}
	return found
}

func intersects(r1, r2 semantic.Rectangle) bool {
	return !(r2.LLX > r1.URX || r2.URX < r1.LLX || r2.LLY > r1.URY || r2.URY < r1.LLY)
}

func contains(outer, inner semantic.Rectangle) bool {
	return inner.LLX >= outer.LLX && inner.URX <= outer.URX &&
		inner.LLY >= outer.LLY && inner.URY <= outer.URY
}
