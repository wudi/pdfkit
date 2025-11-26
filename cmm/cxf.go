package cmm

import (
	"encoding/xml"
)

// CxFObject represents a Color Exchange Format object (PDF 2.0).
type CxFObject struct {
	Data []byte // XML data
}

// SpectrallyDefinedColor represents a color defined by spectral data.
type SpectrallyDefinedColor struct {
	Name string
	CxF  *CxFObject
}

// CxF represents the root of a CxF document.
type CxF struct {
	XMLName   xml.Name  `xml:"CxF"`
	Resources Resources `xml:"Resources"`
}

type Resources struct {
	ObjectCollection ObjectCollection `xml:"ObjectCollection"`
}

type ObjectCollection struct {
	Objects []Object `xml:"Object"`
}

type Object struct {
	Name        string      `xml:"Name,attr"`
	ColorValues ColorValues `xml:"ColorValues"`
}

type ColorValues struct {
	ColorCIELab *ColorCIELab `xml:"ColorCIELab"`
	// Add other value types (Spectral, etc.) as needed
}

type ColorCIELab struct {
	L float64 `xml:"L"`
	A float64 `xml:"A"`
	B float64 `xml:"B"`
}

// ParseCxF parses CxF XML data.
func ParseCxF(data []byte) (*CxF, error) {
	var cxf CxF
	if err := xml.Unmarshal(data, &cxf); err != nil {
		return nil, err
	}
	return &cxf, nil
}
