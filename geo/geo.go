package geo

import "pdflib/ir/raw"

// Viewport specifies a rectangular region of a page (PDF 2.0).
type Viewport struct {
	BBox    []float64 // [llx lly urx ury]
	Name    string
	Measure *Measure
	Owner   raw.ObjectRef // Optional owner
}

// Measure dictionary (Type /Measure).
type Measure struct {
	Subtype string // /RL (Rectilinear) or /GEO (Geospatial)
	Bounds  []float64
	GCS     *CoordinateSystem // Geo Coordinate System
	GPTS    []float64         // Lat/Lon coords
	LPTS    []float64         // Page coords
}

// CoordinateSystem defines the projection.
type CoordinateSystem struct {
	Type string // /PROJCS or /GEOGCS
	WKT  string // Well-Known Text
	EPSG int    // Optional EPSG code if parsed
}
