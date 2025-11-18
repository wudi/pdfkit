package semantic

import (
	"context"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
)

// Document is the semantic representation of a PDF.
type Document struct {
	Pages             []*Page
	Catalog           *Catalog
	Info              *DocumentInfo
	Metadata          *XMPMetadata
	PageLabels        map[int]string // page index -> prefix
	Outlines          []OutlineItem
	AcroForm          *AcroForm
	StructTree        *StructureTree
	decoded           *decoded.DecodedDocument
	Permissions       raw.Permissions
	Encrypted         bool
	MetadataEncrypted bool
}

// Decoded returns the underlying decoded document (if set).
func (d *Document) Decoded() *decoded.DecodedDocument { return d.decoded }

// Page models a single PDF page.
type Page struct {
	Index       int
	MediaBox    Rectangle
	CropBox     Rectangle
	TrimBox     Rectangle
	BleedBox    Rectangle
	ArtBox      Rectangle
	Rotate      int // degrees: 0/90/180/270
	Resources   *Resources
	Contents    []ContentStream
	Annotations []Annotation
	UserUnit    float64
	ref         raw.ObjectRef
}

// ContentStream is a sequence of operations on a page.
type ContentStream struct {
	Operations []Operation
	RawBytes   []byte
}

// Operation represents a PDF operator and operands.
type Operation struct {
	Operator string
	Operands []Operand
}

// Operand is a type-safe operand value.
type Operand interface {
	operand()
	Type() string
}

type NumberOperand struct{ Value float64 }

func (NumberOperand) operand()     {}
func (NumberOperand) Type() string { return "number" }

type NameOperand struct{ Value string }

func (NameOperand) operand()     {}
func (NameOperand) Type() string { return "name" }

type StringOperand struct{ Value []byte }

func (StringOperand) operand()     {}
func (StringOperand) Type() string { return "string" }

type ArrayOperand struct{ Values []Operand }

func (ArrayOperand) operand()     {}
func (ArrayOperand) Type() string { return "array" }

type DictOperand struct{ Values map[string]Operand }

func (DictOperand) operand()     {}
func (DictOperand) Type() string { return "dict" }

// Resources holds per-page resources with optional inheritance.
type Resources struct {
	Fonts       map[string]*Font
	ExtGStates  map[string]ExtGState
	ColorSpaces map[string]ColorSpace
	XObjects    map[string]XObject
	Patterns    map[string]Pattern
	Shadings    map[string]Shading
}

// Font represents a font resource.
type Font struct {
	BaseFont string
	ref      raw.ObjectRef
}

// ExtGState captures graphics state defaults such as transparency.
type ExtGState struct {
	LineWidth   *float64
	StrokeAlpha *float64
	FillAlpha   *float64
}

// ColorSpace references a named colorspace.
type ColorSpace struct {
	Name string // e.g., DeviceRGB, DeviceGray
}

// XObject describes a referenced object (limited to simple images).
type XObject struct {
	Subtype string // e.g., Image, Form
	Width   int
	Height  int
	ColorSpace
	BitsPerComponent int
	Data             []byte
	BBox             Rectangle // used for Form XObjects
}

// Pattern describes a simple tiling pattern stream.
type Pattern struct {
	PatternType int // defaults to 1 (tiling)
	PaintType   int // defaults to 1 (colored)
	TilingType  int // defaults to 1
	BBox        Rectangle
	XStep       float64
	YStep       float64
	Content     []byte
}

// Shading describes a simple axial or radial shading dictionary.
type Shading struct {
	ShadingType int // 2 for axial, 3 for radial
	ColorSpace  ColorSpace
	Coords      []float64 // specific to type (x0 y0 x1 y1 ...)
	Function    []byte
	Domain      []float64
}

// Rectangle represents a PDF rectangle.
type Rectangle struct {
	LLX, LLY, URX, URY float64
}

type Catalog struct{}

// DocumentInfo models /Info dictionary values.
type DocumentInfo struct {
	Title    string
	Author   string
	Subject  string
	Creator  string
	Producer string
	Keywords []string
}

type XMPMetadata struct{ Raw []byte }

type StructureTree struct {
	RoleMap RoleMap
}

// RoleMap maps structure element names to role-mapped names.
type RoleMap map[string]string

// Annotation represents a simple page annotation.
type Annotation struct {
	Subtype  string
	Rect     Rectangle
	URI      string // used for Link annotations
	Contents string
}

// OutlineItem describes a bookmark entry.
type OutlineItem struct {
	Title     string
	PageIndex int
	Children  []OutlineItem
}

// AcroForm represents form-level information.
type AcroForm struct {
	NeedAppearances bool
}

// Builder produces a Semantic document from Decoded IR.
type Builder interface {
	Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error)
}
