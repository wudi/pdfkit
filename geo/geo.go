package geo

import (
	"fmt"
	"math"

	"github.com/wudi/pdfkit/ir/raw"
)

// Viewport specifies a rectangular region of a page (PDF 2.0).
type Viewport struct {
	BBox    []float64 // [llx lly urx ury]
	Name    string
	Measure *Measure
	Owner   raw.ObjectRef // Optional owner
}

// Contains returns true if the point (x, y) is within the viewport.
func (v *Viewport) Contains(x, y float64) bool {
	if len(v.BBox) < 4 {
		return false
	}
	return x >= v.BBox[0] && x <= v.BBox[2] && y >= v.BBox[1] && y <= v.BBox[3]
}

// Measure dictionary (Type /Measure).
type Measure struct {
	Subtype string // /RL (Rectilinear) or /GEO (Geospatial)
	Bounds  []float64
	GCS     *CoordinateSystem // Geo Coordinate System
	GPTS    []float64         // Lat/Lon coords
	LPTS    []float64         // Page coords
}

// Transform maps page coordinates (x, y) to geospatial coordinates (lat, lon).
// Currently supports Affine transformation using the first 3 control points.
func (m *Measure) Transform(x, y float64) (float64, float64, error) {
	if len(m.GPTS) < 6 || len(m.LPTS) < 6 {
		return 0, 0, fmt.Errorf("need at least 3 control points for affine transform")
	}

	// Extract points
	// LPTS: x1, y1, x2, y2, x3, y3
	lx1, ly1 := m.LPTS[0], m.LPTS[1]
	lx2, ly2 := m.LPTS[2], m.LPTS[3]
	lx3, ly3 := m.LPTS[4], m.LPTS[5]

	// GPTS: lat1, lon1, lat2, lon2, lat3, lon3
	gx1, gy1 := m.GPTS[0], m.GPTS[1]
	gx2, gy2 := m.GPTS[2], m.GPTS[3]
	gx3, gy3 := m.GPTS[4], m.GPTS[5]

	// Determinant of the coefficient matrix
	det := lx1*(ly2-ly3) + lx2*(ly3-ly1) + lx3*(ly1-ly2)
	if math.Abs(det) < 1e-9 {
		return 0, 0, fmt.Errorf("collinear control points")
	}

	// Solve for lat = a*x + b*y + c
	a, b, c := solveAffine(lx1, ly1, lx2, ly2, lx3, ly3, gx1, gx2, gx3, det)
	// Solve for lon = d*x + e*y + f
	d, e, f := solveAffine(lx1, ly1, lx2, ly2, lx3, ly3, gy1, gy2, gy3, det)

	lat := a*x + b*y + c
	lon := d*x + e*y + f

	return lat, lon, nil
}

func solveAffine(x1, y1, x2, y2, x3, y3, z1, z2, z3, det float64) (float64, float64, float64) {
	a := (z1*(y2-y3) + z2*(y3-y1) + z3*(y1-y2)) / det
	b := (z1*(x3-x2) + z2*(x1-x3) + z3*(x2-x1)) / det
	c := (z1*(x2*y3-x3*y2) + z2*(x3*y1-x1*y3) + z3*(x1*y2-x2*y1)) / det
	return a, b, c
}

// CoordinateSystem defines the projection.
type CoordinateSystem struct {
	Type string // /PROJCS or /GEOGCS
	WKT  string // Well-Known Text
	EPSG int    // Optional EPSG code if parsed
}
