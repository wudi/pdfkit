package semantic

import (
	"context"

	"github.com/wudi/pdfkit/geo"
	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
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
	DPartRoot         *DPartRoot // PDF/VT
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
	Payload           *Document // PDF 2.0 Unencrypted Wrapper Payload
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
	Viewports       []geo.Viewport // PDF 2.0 / GeoPDF
	Trans           *Transition    // Page transition
	ref             raw.ObjectRef
	OriginalRef     raw.ObjectRef
	Dirty           bool
}

// Transition describes the visual transition when moving to the page.
type Transition struct {
	Style       string   // /S (Split, Blinds, Box, Wipe, Dissolve, Glitter, R, Fly, Push, Cover, Uncover, Fade)
	Duration    *float64 // /D
	Dimension   string   // /Dm (H, V)
	Motion      string   // /M (I, O)
	Direction   int      // /Di (0, 90, 180, 270, 315)
	Scale       *float64 // /SS
	Base        *bool    // /B
	OriginalRef raw.ObjectRef
	Dirty       bool
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

type InlineImageOperand struct {
	Image DictOperand
	Data  []byte
}

func (InlineImageOperand) operand()     {}
func (InlineImageOperand) Type() string { return "inline_image" }

// Resources holds per-page resources with optional inheritance.
type Resources struct {
	Fonts       map[string]*Font
	ExtGStates  map[string]ExtGState
	ColorSpaces map[string]ColorSpace
	XObjects    map[string]XObject
	Patterns    map[string]Pattern
	Shadings    map[string]Shading
	Properties  map[string]PropertyList
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// Font represents a font resource.
type Font struct {
	Subtype        string // Type1 (default), TrueType, Type0, Type3
	BaseFont       string
	Encoding       string
	EncodingDict   *EncodingDict // For custom encodings
	EncodingCMap   []byte        // For custom CMap (Type 0)
	ToUnicodeCMap  []byte        // ToUnicode CMap stream
	Widths         map[int]int   // character code -> width
	ToUnicode      map[int][]rune
	CIDSystemInfo  *CIDSystemInfo
	DescendantFont *CIDFont
	Descriptor     *FontDescriptor
	// Type 3 specific fields
	CharProcs  map[string][]byte
	FontMatrix []float64
	Resources  *Resources
	FontBBox   Rectangle

	ref         raw.ObjectRef
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// EncodingDict represents a custom encoding dictionary.
type EncodingDict struct {
	BaseEncoding string
	Differences  []EncodingDifference
}

// EncodingDifference represents a difference in encoding.
type EncodingDifference struct {
	Code int
	Name string
}

// ExtGState captures graphics state defaults such as transparency.
type ExtGState struct {
	LineWidth      *float64
	StrokeAlpha    *float64
	FillAlpha      *float64
	BlendMode      string // /BM (Normal, Multiply, Screen, Overlay, Darken, Lighten, ColorDodge, ColorBurn, HardLight, SoftLight, Difference, Exclusion)
	AlphaSource    *bool  // /AIS (Alpha Is Shape)
	TextKnockout   *bool  // /TK
	Overprint      *bool  // /OP
	OverprintFill  *bool  // /op
	OverprintMode  *int   // /OPM
	UseBlackPtComp *bool  // /UseBlackPtComp (PDF 2.0)
	SoftMask       *SoftMaskDict
	OriginalRef    raw.ObjectRef
	Dirty          bool
}

// SoftMaskDict represents a soft-mask dictionary used in ExtGState.
type SoftMaskDict struct {
	Subtype       string    // /S (Alpha, Luminosity)
	Group         *XObject  // /G (Transparency Group XObject)
	BackdropColor []float64 // /BC
	Transfer      string    // /TR (Transfer function name)
}

// TransparencyGroup describes the attributes of a transparency group XObject.
type TransparencyGroup struct {
	CS       ColorSpace // /CS
	Isolated bool       // /I
	Knockout bool       // /K
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

func (cs *ICCBasedColorSpace) ColorSpaceName() string { return "ICCBased" }

// SeparationColorSpace represents a Separation color space.
type SeparationColorSpace struct {
	Name          string
	Alternate     ColorSpace
	TintTransform Function // Function object
	OriginalRef   raw.ObjectRef
	Dirty         bool
}

func (cs *SeparationColorSpace) ColorSpaceName() string { return "Separation" }

// DeviceNColorSpace represents a DeviceN color space.
type DeviceNColorSpace struct {
	Names         []string
	Alternate     ColorSpace
	TintTransform Function // Function object
	Attributes    *DeviceNAttributes
	OriginalRef   raw.ObjectRef
	Dirty         bool
}

func (cs *DeviceNColorSpace) ColorSpaceName() string { return "DeviceN" }

type DeviceNAttributes struct {
	Subtype string
}

// IndexedColorSpace represents an Indexed color space.
type IndexedColorSpace struct {
	Base        ColorSpace
	Hival       int
	Lookup      []byte // Can be stream or string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (cs *IndexedColorSpace) ColorSpaceName() string { return "Indexed" }

// SpectrallyDefinedColorSpace represents a CxF-based color space (PDF 2.0).
type SpectrallyDefinedColorSpace struct {
	Data        []byte // CxF XML
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (cs *SpectrallyDefinedColorSpace) ColorSpaceName() string { return "SpectrallyDefined" }

// PatternColorSpace represents the Pattern color space.
type PatternColorSpace struct {
	Underlying ColorSpace // Optional, for uncolored patterns
}

func (cs *PatternColorSpace) ColorSpaceName() string { return "Pattern" }

// XObject describes a referenced object (limited to simple images).
type XObject struct {
	Subtype string // e.g., Image, Form
	Width   int
	Height  int
	ColorSpace
	BitsPerComponent int
	Data             []byte
	Filter           string     // Optional: specific filter to use (e.g. DCTDecode)
	BBox             Rectangle  // used for Form XObjects
	Matrix           []float64  // /Matrix (optional)
	Resources        *Resources // /Resources (for Form XObjects)
	Interpolate      bool
	SMask            *XObject
	Group            *TransparencyGroup // /Group (for Form XObjects)
	AssociatedFiles  []EmbeddedFile     // PDF 2.0
	OriginalRef      raw.ObjectRef
	Dirty            bool
}

// Image is an alias for XObject for image convenience APIs.
type Image = XObject

// Pattern represents a PDF pattern.
type Pattern interface {
	PatternType() int
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
}

type BasePattern struct {
	Type        int
	Matrix      []float64 // Optional transformation matrix
	Ref         raw.ObjectRef
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (p *BasePattern) PatternType() int             { return p.Type }
func (p *BasePattern) Reference() raw.ObjectRef     { return p.Ref }
func (p *BasePattern) SetReference(r raw.ObjectRef) { p.Ref = r }

// TilingPattern (Type 1)
type TilingPattern struct {
	BasePattern
	PaintType  int // 1 = Colored, 2 = Uncolored
	TilingType int // 1 = Constant, 2 = No Distortion, 3 = Constant Spacing
	BBox       Rectangle
	XStep      float64
	YStep      float64
	Resources  *Resources
	Content    []byte
}

// ShadingPattern (Type 2)
type ShadingPattern struct {
	BasePattern
	Shading   Shading
	ExtGState *ExtGState
}

// Shading is the interface for all shading types.
type Shading interface {
	ShadingType() int
	ShadingColorSpace() ColorSpace
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
}

// BaseShading provides common fields for shadings.
type BaseShading struct {
	Type        int
	ColorSpace  ColorSpace
	BBox        Rectangle
	AntiAlias   bool
	Ref         raw.ObjectRef
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (s *BaseShading) ShadingType() int              { return s.Type }
func (s *BaseShading) ShadingColorSpace() ColorSpace { return s.ColorSpace }
func (s *BaseShading) Reference() raw.ObjectRef      { return s.Ref }
func (s *BaseShading) SetReference(r raw.ObjectRef)  { s.Ref = r }

// FunctionShading represents function-based shadings (Type 1, 2, 3).
type FunctionShading struct {
	BaseShading
	Coords   []float64
	Domain   []float64
	Function []Function // Function object or array of functions
	Extend   []bool
}

// MeshShading represents mesh-based shadings (Type 4, 5, 6, 7).
type MeshShading struct {
	BaseShading
	BitsPerCoordinate int
	BitsPerComponent  int
	BitsPerFlag       int
	Decode            []float64
	Function          Function // Optional function for Type 4, 5, 6
	Stream            []byte   // The mesh data stream
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
	Subtype         string // CIDFontType0 or CIDFontType2
	BaseFont        string
	CIDSystemInfo   CIDSystemInfo
	DW              int
	W               map[int]int // CID -> width
	CIDToGIDMap     []byte      // Stream data for CIDToGIDMap
	CIDToGIDMapName string      // Name for CIDToGIDMap (e.g., "Identity")
	Descriptor      *FontDescriptor
}

// FontDescriptor carries metrics and font file embedding details.
type FontDescriptor struct {
	FontName        string
	Flags           int
	ItalicAngle     float64
	Ascent          float64
	Descent         float64
	CapHeight       float64
	StemV           int
	FontBBox        [4]float64
	FontFile        []byte
	FontFileType    string // FontFile2 (TrueType) or FontFile3
	FontFileSubtype string // Subtype of the font file stream (e.g. Type1C, CIDFontType0C)
	Length1         int    // Length of the ASCII segment (Type 1)
	Length2         int    // Length of the encrypted segment (Type 1)
	Length3         int    // Length of the fixed-content segment (Type 1)
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
	Trapped     string // "True", "False", or "Unknown"
	Keywords    []string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

type XMPMetadata struct {
	Raw         []byte
	OriginalRef raw.ObjectRef
	Dirty       bool
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
	Field FormField
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

// StampAnnotation represents a stamp annotation.
type StampAnnotation struct {
	BaseAnnotation
	Name string // Icon name (e.g., "Approved", "Confidential")
}

// InkAnnotation represents a freehand "scribble" annotation.
type InkAnnotation struct {
	BaseAnnotation
	InkList [][]float64 // Array of arrays of points [x1 y1 x2 y2 ...]
}

// FileAttachmentAnnotation represents a file attachment annotation.
type FileAttachmentAnnotation struct {
	BaseAnnotation
	File EmbeddedFile
	Name string // Icon name (e.g., "Graph", "PushPin", "Paperclip", "Tag")
}

// PopupAnnotation represents a popup annotation.
type PopupAnnotation struct {
	BaseAnnotation
	Parent Annotation // The parent annotation with which this popup is associated
	Open   bool       // Whether the popup window should be open by default
}

// SoundAnnotation represents a sound annotation.
type SoundAnnotation struct {
	BaseAnnotation
	Sound EmbeddedFile // The sound to be played
	Name  string       // Icon name (e.g., "Speaker", "Mic")
}

// MovieAnnotation represents a movie annotation.
type MovieAnnotation struct {
	BaseAnnotation
	Title string       // The title of the movie
	Movie EmbeddedFile // The movie file
}

// ScreenAnnotation represents a screen annotation.
type ScreenAnnotation struct {
	BaseAnnotation
	Title  string // The title of the screen annotation
	Action Action // The action to be performed
}

// PrinterMarkAnnotation represents a printer's mark annotation.
type PrinterMarkAnnotation struct {
	BaseAnnotation
}

// TrapNetAnnotation represents a trapping network annotation.
type TrapNetAnnotation struct {
	BaseAnnotation
	LastModified string // The date and time of the last modification
	Version      []int  // The version of the trapping network
}

// WatermarkAnnotation represents a watermark annotation.
type WatermarkAnnotation struct {
	BaseAnnotation
	FixedPrint bool // Whether the watermark should be printed at a fixed size and position
}

// ThreeDAnnotation represents a 3D annotation.
type ThreeDAnnotation struct {
	BaseAnnotation
	ThreeD EmbeddedFile // The 3D artwork
	View   string       // The initial view of the 3D artwork
}

// RedactAnnotation represents a redaction annotation.
type RedactAnnotation struct {
	BaseAnnotation
	OverlayText string    // The text to be displayed on the redacted area
	Repeat      []float64 // The repeat interval for the overlay text
}

// ProjectionAnnotation represents a projection annotation.
type ProjectionAnnotation struct {
	BaseAnnotation
	ProjectionType string // The type of projection
}

// GenericAnnotation represents an annotation not covered by specific types.
type GenericAnnotation struct {
	BaseAnnotation
}

// SoundAction represents a sound action.
type SoundAction struct {
	Sound       *EmbeddedFile
	Volume      *float64
	Synchronous *bool
	Repeat      *bool
	Mix         *bool
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a SoundAction) ActionType() string { return "Sound" }

// MovieAction represents a movie action.
type MovieAction struct {
	Title       string // Title of the movie annotation
	Operation   string // Play, Stop, Pause, Resume
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a MovieAction) ActionType() string { return "Movie" }

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

// JavaScriptAction represents a JavaScript action.
type JavaScriptAction struct {
	JS          string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a JavaScriptAction) ActionType() string { return "JavaScript" }

// NamedAction represents a named action (e.g., NextPage, PrevPage).
type NamedAction struct {
	Name        string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a NamedAction) ActionType() string { return "Named" }

// LaunchAction represents a launch action (launching an application or opening a file).
type LaunchAction struct {
	File        string
	NewWindow   *bool
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a LaunchAction) ActionType() string { return "Launch" }

// SubmitFormAction represents a submit-form action.
type SubmitFormAction struct {
	URL         string
	Flags       int
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a SubmitFormAction) ActionType() string { return "SubmitForm" }

// ResetFormAction represents a reset-form action.
type ResetFormAction struct {
	Fields      []string // List of field names to reset (or exclude)
	Flags       int
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a ResetFormAction) ActionType() string { return "ResetForm" }

// ImportDataAction represents an import-data action.
type ImportDataAction struct {
	File        string
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a ImportDataAction) ActionType() string { return "ImportData" }

// GoToRAction represents a remote go-to action.
type GoToRAction struct {
	File        string
	Dest        *OutlineDestination
	DestName    string
	NewWindow   *bool
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a GoToRAction) ActionType() string { return "GoToR" }

// GoToEAction represents an embedded go-to action.
type GoToEAction struct {
	Dest        *OutlineDestination
	DestName    string
	NewWindow   *bool
	Target      *EmbeddedFile // The embedded file
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a GoToEAction) ActionType() string { return "GoToE" }

// HideAction represents a hide action.
type HideAction struct {
	TargetName  string // Name of the field/annotation to hide
	Hide        bool
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a HideAction) ActionType() string { return "Hide" }

// GoTo3DViewAction represents a go-to 3D view action.
type GoTo3DViewAction struct {
	Target      Annotation // The 3D annotation
	View        string     // The view name
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a GoTo3DViewAction) ActionType() string { return "GoTo3DView" }

// ThreadAction represents a thread action.
type ThreadAction struct {
	File        string        // Optional file specification
	Thread      raw.ObjectRef // Dictionary or integer or string
	Bead        raw.ObjectRef // Optional bead
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a ThreadAction) ActionType() string { return "Thread" }

// RichMediaExecuteAction represents a rich media execute action.
type RichMediaExecuteAction struct {
	Command     raw.ObjectRef // Command dictionary
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (a RichMediaExecuteAction) ActionType() string { return "RichMediaExecute" }

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

// Signature represents a digital signature dictionary (Type /Sig).
type Signature struct {
	Filter      string   // /Filter (e.g., Adobe.PPKLite)
	SubFilter   string   // /SubFilter (e.g., adbe.pkcs7.detached)
	Contents    []byte   // /Contents (hex string in PDF, bytes here)
	Cert        []byte   // /Cert (byte string)
	ByteRange   []int    // /ByteRange
	Reference   []SigRef // /Reference (array of signature reference dicts)
	Name        string   // /Name
	M           string   // /M (Date)
	Location    string   // /Location
	Reason      string   // /Reason
	ContactInfo string   // /ContactInfo
	OriginalRef raw.ObjectRef
	Dirty       bool
}

// SigRef represents a signature reference dictionary.
type SigRef struct {
	Type            string              // /Type /SigRef
	TransformMethod string              // /TransformMethod (e.g., DocMDP, UR, FieldMDP)
	TransformParams *SigTransformParams // /TransformParams
	DigestMethod    string              // /DigestMethod (e.g., MD5, SHA1)
	DigestValue     []byte              // /DigestValue
	DigestLocation  []int               // /DigestLocation
}

// SigTransformParams represents parameters for a transform method.
type SigTransformParams struct {
	Type   string   // /Type /TransformParams
	P      int      // /P (Permissions for DocMDP)
	V      string   // /V (Version)
	Fields []string // /Fields (for FieldMDP)
	Action string   // /Action (Include/Exclude/All)
}

// Builder produces a Semantic document from Decoded IR.
type Builder interface {
	Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error)
}

// Function represents a PDF function.
type Function interface {
	FunctionType() int
	FunctionDomain() []float64
	FunctionRange() []float64
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
}

type BaseFunction struct {
	Type        int
	Domain      []float64
	Range       []float64
	Ref         raw.ObjectRef
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (f *BaseFunction) FunctionType() int            { return f.Type }
func (f *BaseFunction) FunctionDomain() []float64    { return f.Domain }
func (f *BaseFunction) FunctionRange() []float64     { return f.Range }
func (f *BaseFunction) Reference() raw.ObjectRef     { return f.Ref }
func (f *BaseFunction) SetReference(r raw.ObjectRef) { f.Ref = r }

// SampledFunction (Type 0)
type SampledFunction struct {
	BaseFunction
	Size          []int
	BitsPerSample int
	Order         int // Default 1
	Encode        []float64
	Decode        []float64
	Samples       []byte
}

// ExponentialFunction (Type 2)
type ExponentialFunction struct {
	BaseFunction
	C0 []float64
	C1 []float64
	N  float64
}

// StitchingFunction (Type 3)
type StitchingFunction struct {
	BaseFunction
	Functions []Function
	Bounds    []float64
	Encode    []float64
}

// PostScriptFunction (Type 4)
type PostScriptFunction struct {
	BaseFunction
	Code []byte
}

// PropertyList is a marker interface for objects that can be in the Properties resource dictionary.
type PropertyList interface {
	PropertyListType() string
	Reference() raw.ObjectRef
	SetReference(raw.ObjectRef)
}

type BasePropertyList struct {
	Ref         raw.ObjectRef
	OriginalRef raw.ObjectRef
	Dirty       bool
}

func (p *BasePropertyList) Reference() raw.ObjectRef     { return p.Ref }
func (p *BasePropertyList) SetReference(r raw.ObjectRef) { p.Ref = r }

// OptionalContentGroup (OCG)
type OptionalContentGroup struct {
	BasePropertyList
	Name   string
	Intent []string // e.g. /View, /Design
	Usage  *OCUsage
}

func (g *OptionalContentGroup) PropertyListType() string { return "OCG" }

// OptionalContentMembership (OCMD)
type OptionalContentMembership struct {
	BasePropertyList
	OCGs   []*OptionalContentGroup
	Policy string // /AllOn, /AnyOn, /AnyOff, /AllOff
}

func (m *OptionalContentMembership) PropertyListType() string { return "OCMD" }

// OCUsage describes usage application dictionaries for OCGs.
type OCUsage struct {
	CreatorInfo *OCCreatorInfo
	Language    *OCLanguage
	Export      *OCExport
	Zoom        *OCZoom
	Print       *OCPrint
	View        *OCView
	User        *OCUser
}

type OCCreatorInfo struct {
	Creator string
	Subtype string
}

type OCLanguage struct {
	Lang      string
	Preferred bool
}

type OCExport struct {
	ExportState bool // /ON or /OFF
}

type OCZoom struct {
	Min float64
	Max float64 // 0 means infinity
}

type OCPrint struct {
	Subtype    string // /Print
	PrintState bool   // /ON or /OFF
}

type OCView struct {
	ViewState bool // /ON or /OFF
}

type OCUser struct {
	Type string // /User
	Name string
	User []string
}

// DPartRoot represents the root of the DPart hierarchy (PDF/VT).
type DPartRoot struct {
	OriginalRef raw.ObjectRef
	Dirty       bool
}
