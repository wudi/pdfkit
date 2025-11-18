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
	Index     int
	MediaBox  Rectangle
	CropBox   Rectangle
	Rotate    int // degrees: 0/90/180/270
	Resources *Resources
	Contents  []ContentStream
	UserUnit  float64
	ref       raw.ObjectRef
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
	Fonts map[string]*Font
}

// Font represents a font resource.
type Font struct {
	BaseFont string
	ref      raw.ObjectRef
}

// Rectangle represents a PDF rectangle.
type Rectangle struct {
	LLX, LLY, URX, URY float64
}

type Catalog struct{}

// DocumentInfo models /Info dictionary values.
type DocumentInfo struct {
	Title string
}

type XMPMetadata struct{ Raw []byte }

type StructureTree struct{}

// Builder produces a Semantic document from Decoded IR.
type Builder interface {
	Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error)
}
