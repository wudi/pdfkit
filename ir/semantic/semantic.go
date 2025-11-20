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
	Lang              string
	Marked            bool
	PageLabels        map[int]string // page index -> prefix
	Outlines          []OutlineItem
	Articles          []ArticleThread
	AcroForm          *AcroForm
	StructTree        *StructureTree
	OutputIntents     []OutputIntent
	EmbeddedFiles     []EmbeddedFile
	decoded           *decoded.DecodedDocument
	OwnerPassword     string
	UserPassword      string
	Permissions       raw.Permissions
	Encrypted         bool
	MetadataEncrypted bool
	OriginalRef       raw.ObjectRef
	Dirty             bool
}

// Decoded returns the underlying decoded document (if set).
func (d *Document) Decoded() *decoded.DecodedDocument { return d.decoded }

// Page models a single PDF page.
type Page struct {
	Index           int
	MediaBox        Rectangle
	CropBox         Rectangle
	TrimBox         Rectangle
	BleedBox        Rectangle
	ArtBox          Rectangle
	Rotate          int // degrees: 0/90/180/270
	Resources       *Resources
	Contents        []ContentStream
	Annotations     []Annotation
	UserUnit        float64
	OutputIntents   []OutputIntent // PDF 2.0
	AssociatedFiles []EmbeddedFile // PDF 2.0
	ref             raw.ObjectRef
	OriginalRef     raw.ObjectRef
	Dirty           bool
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
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// Font represents a font resource.
type Font struct {
	Subtype        string // Type1 (default), TrueType, Type0
	BaseFont       string
	Encoding       string
	Widths         map[int]int // character code -> width
	ToUnicode      map[int][]rune
	CIDSystemInfo  *CIDSystemInfo
	DescendantFont *CIDFont
	Descriptor     *FontDescriptor
	ref            raw.ObjectRef
	OriginalRef    raw.ObjectRef
	Dirty          bool
}

// ExtGState captures graphics state defaults such as transparency.
type ExtGState struct {
	LineWidth   *float64
	StrokeAlpha *float64
	FillAlpha   *float64
}

// ColorSpace references a named colorspace.
type ColorSpace interface {
	ColorSpaceName() string
}

type DeviceColorSpace struct {
	Name string
}

func (cs DeviceColorSpace) ColorSpaceName() string { return cs.Name }

// ICCBasedColorSpace represents an ICC-based color space.
type ICCBasedColorSpace struct {
	Profile     []byte
	Alternate   ColorSpace
	Range       []float64
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (cs ICCBasedColorSpace) ColorSpaceName() string { return "ICCBased" }

// XObject describes a referenced object (limited to simple images).
type XObject struct {
	Subtype string // e.g., Image, Form
	Width   int
	Height  int
	ColorSpace
	BitsPerComponent int
	Data             []byte
	BBox             Rectangle // used for Form XObjects
	Interpolate      bool
	SMask            *XObject
	AssociatedFiles  []EmbeddedFile // PDF 2.0
	OriginalRef      raw.ObjectRef
	Dirty            bool
}

// Image is an alias for XObject for image convenience APIs.
type Image = XObject

// Pattern describes a simple tiling pattern stream.
type Pattern struct {
	PatternType int // defaults to 1 (tiling)
	PaintType   int // defaults to 1 (colored)
	TilingType  int // defaults to 1
	BBox        Rectangle
	XStep       float64
	YStep       float64
	Content     []byte
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// Shading describes a simple axial or radial shading dictionary.
type Shading struct {
	ShadingType int // 2 for axial, 3 for radial
	ColorSpace  ColorSpace
	Coords      []float64 // specific to type (x0 y0 x1 y1 ...)
	Function    []byte
	Domain      []float64
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// Rectangle represents a PDF rectangle.
type Rectangle struct {
	LLX, LLY, URX, URY float64
}

// CIDSystemInfo describes the registry/ordering of a CID font.
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int
}

// CIDFont describes a descendant font for Type0 fonts.
type CIDFont struct {
	Subtype       string // CIDFontType0 or CIDFontType2
	BaseFont      string
	CIDSystemInfo CIDSystemInfo
	DW            int
	W             map[int]int // CID -> width
	Descriptor    *FontDescriptor
}

// FontDescriptor carries metrics and font file embedding details.
type FontDescriptor struct {
	FontName     string
	Flags        int
	ItalicAngle  float64
	Ascent       float64
	Descent      float64
	CapHeight    float64
	StemV        int
	FontBBox     [4]float64
	FontFile     []byte
	FontFileType string // FontFile2 (TrueType) or FontFile3
}

type Catalog struct{}

// EmbeddedFile models an associated file (e.g., PDF/A-3 attachments).
type EmbeddedFile struct {
	Name         string
	Description  string
	Relationship string
	Subtype      string
	Data         []byte
	OriginalRef  raw.ObjectRef
	Dirty        bool
}

// DocumentInfo models /Info dictionary values.
type DocumentInfo struct {
	Title       string
	Author      string
	Subject     string
	Creator     string
	Producer    string
	Keywords    []string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

type XMPMetadata struct {
	Raw         []byte
	OriginalRef raw.ObjectRef
	Dirty       bool
}

type StructureTree struct {
	RoleMap     RoleMap
	Kids        []*StructureElement
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// StructureElement captures a tagged PDF structure element with children or marked content references.
type StructureElement struct {
	Type            string
	Title           string
	PageIndex       *int
	Kids            []StructureItem
	AssociatedFiles []EmbeddedFile // PDF 2.0
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// StructureItem represents either a child element or a marked-content reference (MCID) on a page.
type StructureItem struct {
	Element   *StructureElement
	MCID      *int
	PageIndex *int
}

// OutputIntent models color output intent metadata.
type OutputIntent struct {
	S                         string
	OutputConditionIdentifier string
	Info                      string
	DestOutputProfile         []byte
	OriginalRef               raw.ObjectRef
	Dirty                     bool
}

// RoleMap maps structure element names to role-mapped names.
type RoleMap map[string]string

// Annotation represents a page annotation.
type Annotation interface {
	Type() string
	Rect() Rectangle
	SetRect(Rectangle)
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
	Base() *BaseAnnotation
}

// BaseAnnotation provides common fields for annotations.
type BaseAnnotation struct {
	Subtype         string
	RectVal         Rectangle
	Contents        string
	Appearance      []byte
	Flags           int
	Border          []float64
	Color           []float64
	AppearanceState string
	AssociatedFiles []EmbeddedFile // PDF 2.0
	Ref             raw.ObjectRef
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

func (a *BaseAnnotation) Type() string                 { return a.Subtype }
func (a *BaseAnnotation) Rect() Rectangle              { return a.RectVal }
func (a *BaseAnnotation) SetRect(r Rectangle)          { a.RectVal = r }
func (a *BaseAnnotation) Reference() raw.ObjectRef     { return a.Ref }
func (a *BaseAnnotation) SetReference(r raw.ObjectRef) { a.Ref = r }
func (a *BaseAnnotation) Base() *BaseAnnotation        { return a }

// LinkAnnotation represents a link annotation.
type LinkAnnotation struct {
	BaseAnnotation
	URI    string
	Action Action
}

// WidgetAnnotation represents a form widget annotation.
type WidgetAnnotation struct {
	BaseAnnotation
	Field *FormField
}

// TextAnnotation represents a sticky note annotation.
type TextAnnotation struct {
	BaseAnnotation
	Open bool
	Icon string // e.g., "Comment", "Key", "Note", "Help", "NewParagraph", "Paragraph", "Insert"
}

// HighlightAnnotation represents a highlight annotation.
type HighlightAnnotation struct {
	BaseAnnotation
	QuadPoints []float64 // 8 numbers specifying the coordinates of the four corners of the quadrilateral
}

// UnderlineAnnotation represents an underline annotation.
type UnderlineAnnotation struct {
	BaseAnnotation
	QuadPoints []float64
}

// StrikeOutAnnotation represents a strikeout annotation.
type StrikeOutAnnotation struct {
	BaseAnnotation
	QuadPoints []float64
}

// SquigglyAnnotation represents a squiggly underline annotation.
type SquigglyAnnotation struct {
	BaseAnnotation
	QuadPoints []float64
}

// FreeTextAnnotation represents a free text annotation.
type FreeTextAnnotation struct {
	BaseAnnotation
	DA string // Default appearance string
	Q  int    // Quadding (justification): 0=Left, 1=Center, 2=Right
}

// LineAnnotation represents a line annotation.
type LineAnnotation struct {
	BaseAnnotation
	L  []float64 // Array of 4 numbers [x1 y1 x2 y2]
	LE []string  // Line ending styles [start end] e.g. /Square, /Circle, /Diamond, /OpenArrow, /ClosedArrow, /None
	IC []float64 // Interior color
}

// SquareAnnotation represents a square annotation.
type SquareAnnotation struct {
	BaseAnnotation
	IC []float64 // Interior color
	RD []float64 // Rect differences (padding)
}

// CircleAnnotation represents a circle annotation.
type CircleAnnotation struct {
	BaseAnnotation
	IC []float64 // Interior color
	RD []float64 // Rect differences (padding)
}

// GenericAnnotation represents an annotation not covered by specific types.
type GenericAnnotation struct {
	BaseAnnotation
}

// Action represents a PDF action.
type Action interface {
	ActionType() string
}

// URIAction represents a URI action.
type URIAction struct {
	URI         string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a URIAction) ActionType() string { return "URI" }

// GoToAction represents a GoTo action.
type GoToAction struct {
	Dest        *OutlineDestination
	PageIndex   int
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a GoToAction) ActionType() string { return "GoTo" }

// OutlineItem describes a bookmark entry.
type OutlineItem struct {
	Title       string
	PageIndex   int
	Dest        *OutlineDestination
	Children    []OutlineItem
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// OutlineDestination describes an outline destination using XYZ coordinates.
// Nil fields indicate "leave unchanged" semantics per PDF spec.
type OutlineDestination struct {
	X    *float64
	Y    *float64
	Zoom *float64
}

// ArticleThread represents an article with an ordered list of beads.
type ArticleThread struct {
	Title       string
	Beads       []ArticleBead
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// ArticleBead describes a single segment of an article.
type ArticleBead struct {
	PageIndex   int
	Rect        Rectangle
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// AcroForm represents form-level information.
type AcroForm struct {
	NeedAppearances bool
	Fields          []FormField
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// FormField is a simplified representation of a form field.
type FormField struct {
	Name            string
	Value           string
	Type            string // e.g., "Tx" for text
	PageIndex       int
	Rect            Rectangle
	Flags           int
	Appearance      []byte
	AppearanceState string
	Border          []float64
	Color           []float64
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// Builder produces a Semantic document from Decoded IR.
type Builder interface {
	Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error)
}
