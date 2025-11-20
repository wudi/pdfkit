package cmm

// CxFObject represents a Color Exchange Format object (PDF 2.0).
type CxFObject struct {
	Data []byte // XML data
}

// SpectrallyDefinedColor represents a color defined by spectral data.
type SpectrallyDefinedColor struct {
	Name string
	CxF  *CxFObject
}
