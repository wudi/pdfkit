# PDF Parser & Creator Library — Design Document v2.0

## 1. Executive Summary

This document specifies the architecture, interfaces, and extensibility model for a **production-grade PDF parser and creator library** in Go.

The library is designed to support:

* **Streaming parsing and writing** for large PDFs with configurable backpressure
* **Three-tier IR (Intermediate Representation)** with explicit transformation boundaries
* **Full fidelity read/write** including vector graphics, text, images, forms, annotations, and metadata
* **Incremental updates** with append-only or full rewrite modes
* **Font subsetting and embedding** with complete pipeline specification
* **PDF/A compliance** (1/2/3 variants) with validation and auto-correction
* **Extensible plugin system** with defined execution phases and ordering
* **Robust security and error recovery**

The library provides both **high-level convenience APIs** for typical PDF tasks and **low-level control** for applications requiring fine-grained operations.

---

## 2. Goals

**Primary Goals**

1. Parse PDFs of any complexity with configurable memory limits
2. Provide three explicit IR levels (Raw, Decoded, Semantic) with clear transformation boundaries
3. Enable deterministic PDF generation with embedded font subsetting
4. Support incremental updates and append-only modifications
5. Ensure PDF/A compliance with validation and automatic fixes
6. Provide extensibility through well-defined plugin phases
7. Handle malformed PDFs with configurable error recovery
8. Support concurrent operations where safe

**Non-Goals**

* Full OCR or text recognition (may be delegated to plugins)
* PDF rendering engine (layout calculation for display)
* Built-in cloud storage integration (external I/O adapters)

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     High-Level Builder API                   │
│                   (Convenience, Fluent Interface)            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Semantic IR (Level 3)                       │
│         Pages, Fonts, Images, Annotations, Metadata         │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Decoded IR (Level 2)                        │
│            Decompressed Streams, Decrypted Objects          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Raw IR (Level 1)                          │
│       Dictionaries, Arrays, Streams, Names, Numbers          │
└────────────────────────┬────────────────────────────────────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
          ▼              ▼              ▼
    ┌─────────┐    ┌─────────┐   ┌──────────┐
    │ Scanner │    │  XRef   │   │ Security │
    │Tokenizer│    │Resolver │   │ Handler  │
    └─────────┘    └─────────┘   └──────────┘
          │              │              │
          └──────────────┴──────────────┘
                         │
                         ▼
                   Input Stream

───────────────────────────────────────────────────────────────

                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Extension Hub                             │
│          Inspect → Sanitize → Transform → Validate           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                 Serialization Engine                         │
│       Full Writer, Incremental Writer, Linearization        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
                   Output Stream
```

**Cross-Cutting Concerns (injected into all layers):**
- Error Recovery Strategy
- Context & Cancellation
- Security Limits

---

## 4. Module Architecture

### 4.1 Core Modules

| Module           | Responsibility                                                    |
| ---------------- | ----------------------------------------------------------------- |
| `scanner`        | Tokenizes raw PDF bytes, handles PDF syntax                       |
| `xref`           | Resolves cross-reference tables and streams                       |
| `security`       | Encryption/decryption, permissions, password handling             |
| `parser`         | Coordinates scanning, xref, security to build Raw IR              |
| `ir/raw`         | Raw PDF objects (Level 1): dictionaries, arrays, streams          |
| `ir/decoded`     | Decoded objects (Level 2): decompressed, decrypted                |
| `ir/semantic`    | Semantic objects (Level 3): pages, fonts, annotations             |
| `filters`        | Stream decoders (Flate, DCT, JPX, etc.) with pipeline composition |
| `contentstream`  | Content stream parsing, graphics state, text positioning          |
| `resources`      | Resource resolution with inheritance and scoping                  |
| `fonts`          | Font subsetting, embedding, ToUnicode generation                  |
| `coords`         | Coordinate transformations, user space, device space              |
| `writer`         | PDF serialization: full, incremental, linearized                  |
| `pdfa`           | PDF/A validation, XMP generation, ICC profiles, compliance fixes  |
| `extensions`     | Plugin system with phased execution model                         |
| `recovery`       | Error recovery strategies for malformed PDFs                      |
| `builder`        | High-level fluent API for PDF construction                        |
| `layout`         | Layout engine for converting structured content (Markdown/HTML) to PDF |

### 4.2 Module Dependencies

```
builder
  └─→ ir/semantic
       └─→ ir/decoded
            └─→ ir/raw
                 └─→ scanner, xref, security

layout
  └─→ builder

extensions
  └─→ ir/semantic (operates on semantic IR)

writer
  └─→ ir/semantic
       └─→ ir/decoded
            └─→ ir/raw

fonts
  └─→ ir/semantic (pages, text)

contentstream
  └─→ ir/decoded (stream bytes)
       └─→ coords (transformations)

filters
  └─→ ir/raw (stream dictionaries)

recovery
  └─→ (injected into all layers)
```

---

## 5. Three-Tier IR Architecture

### 5.1 Level 1: Raw IR

**Purpose:** Direct representation of PDF primitive objects as per PDF spec.

```go
package raw

// ObjectRef uniquely identifies a PDF object
type ObjectRef struct {
    Num int // Object number
    Gen int // Generation number
}

func (r ObjectRef) String() string {
    return fmt.Sprintf("%d %d R", r.Num, r.Gen)
}

// Object is the base interface for all raw PDF objects
type Object interface {
    // Type returns the PDF type: "dict", "array", "stream", "name", 
    // "string", "number", "boolean", "null", "ref"
    Type() string
    
    // IsIndirect returns true if this is an indirect object reference
    IsIndirect() bool
}

// Dictionary represents a PDF dictionary object
type Dictionary interface {
    Object
    Get(key Name) (Object, bool)
    Set(key Name, value Object)
    Keys() []Name
    Len() int
}

// Array represents a PDF array object
type Array interface {
    Object
    Get(index int) (Object, bool)
    Len() int
    Append(obj Object)
}

// Stream represents a raw (undecoded) PDF stream
type Stream interface {
    Object
    Dictionary() Dictionary  // Stream dictionary
    RawData() []byte        // Compressed/encrypted data
    Length() int64
}

// Name represents a PDF name object
type Name interface {
    Object
    Value() string
}

// String represents a PDF string (literal or hex)
type String interface {
    Object
    Value() []byte
    IsHex() bool
}

// Number represents a PDF numeric value
type Number interface {
    Object
    Int() int64
    Float() float64
    IsInteger() bool
}

// Boolean represents a PDF boolean
type Boolean interface {
    Object
    Value() bool
}

// Null represents the PDF null object
type Null interface {
    Object
}

// Reference represents an indirect object reference
type Reference interface {
    Object
    Ref() ObjectRef
}

// Document is the root container for raw PDF objects
type Document struct {
    Objects  map[ObjectRef]Object
    Trailer  Dictionary
    Version  string // e.g., "1.7"
    Metadata DocumentMetadata
}

type DocumentMetadata struct {
    Producer string
    Creator  string
    Title    string
    Author   string
    Subject  string
    Keywords []string
}
```

### 5.2 Level 2: Decoded IR

**Purpose:** Objects after stream decoding and decryption.

```go
package decoded

import (
    "pdflib/ir/raw"
)

// Object wraps a raw object with decoded stream data
type Object interface {
    Raw() raw.Object
    Type() string
}

// Stream represents a decoded PDF stream
type Stream interface {
    Object
    Dictionary() raw.Dictionary
    Data() []byte  // Decompressed, decrypted data
    Filters() []string  // Applied filters (for reference)
}

// DecodedDocument contains all decoded objects
type DecodedDocument struct {
    Raw     *raw.Document
    Streams map[raw.ObjectRef]Stream
}

// Decoder transforms raw IR to decoded IR
type Decoder interface {
    Decode(ctx context.Context, rawDoc *raw.Document) (*DecodedDocument, error)
}
```

### 5.3 Level 3: Semantic IR

**Purpose:** High-level PDF structures with business logic.

```go
package semantic

import (
    "pdflib/ir/decoded"
    "pdflib/ir/raw"
)

// Document is the semantic representation of a PDF
type Document struct {
    Pages       []*Page
    Catalog     *Catalog
    Info        *DocumentInfo
    Metadata    *XMPMetadata
    StructTree  *StructureTree
    decoded     *decoded.DecodedDocument
}

// Page represents a PDF page with all its content
type Page struct {
    Index       int
    MediaBox    Rectangle
    CropBox     Rectangle
    Rotate      int  // 0, 90, 180, 270
    Resources   *Resources
    Contents    []ContentStream
    Annotations []*Annotation
    UserUnit    float64
    ref         raw.ObjectRef
}

// ContentStream is a sequence of content operations
type ContentStream struct {
    Operations []Operation
    RawBytes   []byte
}

// Operation represents a single PDF operator with operands
type Operation struct {
    Operator string
    Operands []Operand
}

// Operand is a type-safe operand value
type Operand interface {
    operand()
    Type() string
}

type NumberOperand struct{ Value float64 }
type NameOperand struct{ Value string }
type StringOperand struct{ Value []byte }
type ArrayOperand struct{ Values []Operand }
type DictOperand struct{ Values map[string]Operand }

func (NumberOperand) operand() {}
func (NameOperand) operand() {}
func (StringOperand) operand() {}
func (ArrayOperand) operand() {}
func (DictOperand) operand() {}

// Resources contains page/XObject resources
type Resources struct {
    Fonts       map[string]*Font
    XObjects    map[string]*XObject
    ColorSpaces map[string]*ColorSpace
    Patterns    map[string]*Pattern
    Shadings    map[string]*Shading
    ExtGStates  map[string]*ExtGState
    Properties  map[string]raw.Dictionary
    parent      *Resources  // For inheritance
}

// Font represents an embedded or referenced font
type Font struct {
    Type         FontType
    BaseFont     string
    Encoding     Encoding
    Descriptor   *FontDescriptor
    ToUnicode    *CMap
    CIDSystemInfo *CIDSystemInfo
    ref          raw.ObjectRef
}

type FontType int

const (
    FontType1 FontType = iota
    FontTrueType
    FontType3
    FontCIDFontType0
    FontCIDFontType2
)

// XObject represents a Form or Image XObject
type XObject struct {
    Subtype   XObjectType
    Resources *Resources  // For Form XObjects
    BBox      Rectangle   // For Form XObjects
    Matrix    Matrix      // For Form XObjects
    Image     *Image      // For Image XObjects
    ref       raw.ObjectRef
}

type XObjectType int

const (
    XObjectForm XObjectType = iota
    XObjectImage
    XObjectPS
)

// Image represents an image in a PDF
type Image struct {
    Width         int
    Height        int
    ColorSpace    *ColorSpace
    BitsPerComp   int
    DecodedData   []byte
    SMask         *Image  // Soft mask for transparency
    Interpolate   bool
}

// Annotation represents a PDF annotation
type Annotation struct {
    Type     AnnotationType
    Subtype  string
    Rect     Rectangle
    Contents string
    Appearance *Appearance
    ref      raw.ObjectRef
}

type AnnotationType int

const (
    AnnotText AnnotationType = iota
    AnnotLink
    AnnotFreeText
    AnnotLine
    AnnotSquare
    AnnotCircle
    AnnotPolygon
    AnnotHighlight
    AnnotUnderline
    AnnotStrikeOut
    AnnotStamp
    AnnotInk
    AnnotPopup
    AnnotWidget  // Form fields
)

// Rectangle represents a PDF rectangle
type Rectangle struct {
    LLX, LLY, URX, URY float64  // Lower-left, upper-right
}

// Matrix represents a transformation matrix [a b c d e f]
type Matrix [6]float64

// Builder transforms decoded IR to semantic IR
type Builder interface {
    Build(ctx context.Context, decoded *decoded.DecodedDocument) (*Document, error)
}
```

### 5.4 IR Transformation Pipeline

```go
package ir

import (
    "pdflib/ir/raw"
    "pdflib/ir/decoded"
    "pdflib/ir/semantic"
)

// Pipeline orchestrates IR transformations
type Pipeline struct {
    rawParser      raw.Parser
    decoder        decoded.Decoder
    semanticBuilder semantic.Builder
    recovery       recovery.Strategy
}

func (p *Pipeline) Parse(ctx context.Context, r io.ReaderAt) (*semantic.Document, error) {
    // Stage 1: Parse to Raw IR
    rawDoc, err := p.rawParser.Parse(ctx, r)
    if err != nil {
        return nil, fmt.Errorf("raw parsing failed: %w", err)
    }
    
    // Stage 2: Decode to Decoded IR
    decodedDoc, err := p.decoder.Decode(ctx, rawDoc)
    if err != nil {
        return nil, fmt.Errorf("decoding failed: %w", err)
    }
    
    // Stage 3: Build Semantic IR
    semanticDoc, err := p.semanticBuilder.Build(ctx, decodedDoc)
    if err != nil {
        return nil, fmt.Errorf("semantic building failed: %w", err)
    }
    
    return semanticDoc, nil
}
```

---

## 6. Core Component Specifications

### 6.1 Scanner & Parser

```go
package scanner

// Token represents a PDF token
type Token struct {
    Type  TokenType
    Value interface{}
    Pos   int64  // Byte position in file
}

type TokenType int

const (
    TokenDict TokenType = iota
    TokenArray
    TokenName
    TokenString
    TokenNumber
    TokenBoolean
    TokenNull
    TokenRef
    TokenStream
    TokenKeyword  // obj, endobj, stream, endstream
)

// Scanner tokenizes PDF byte streams
type Scanner interface {
    // Next returns the next token or io.EOF
    Next() (Token, error)
    
    // Position returns current byte offset
    Position() int64
    
    // Seek moves to byte offset
    Seek(offset int64) error
}

// ScannerConfig configures scanner behavior
type ScannerConfig struct {
    MaxStringLength int64  // Limit for string tokens
    MaxArrayDepth   int    // Limit for nested arrays
    MaxDictDepth    int    // Limit for nested dicts
    Recovery        recovery.Strategy
}

// NewScanner creates a scanner with config
func NewScanner(r io.ReaderAt, cfg ScannerConfig) Scanner
```

### 6.2 XRef Resolution

```go
package xref

// Table represents cross-reference information
type Table interface {
    // Lookup returns byte offset and generation for object number
    Lookup(objNum int) (offset int64, gen int, found bool)
    
    // Objects returns all object numbers in table
    Objects() []int
    
    // Type returns "table" or "stream"
    Type() string
}

// Resolver resolves object references to byte offsets
type Resolver interface {
    // Resolve returns the xref table for the document
    Resolve(ctx context.Context, r io.ReaderAt) (Table, error)
    
    // Linearized returns true if PDF is linearized
    Linearized() bool
    
    // Incremental returns previous xref tables if PDF has updates
    Incremental() []Table
}

// ResolverConfig configures xref resolution
type ResolverConfig struct {
    MaxXRefDepth int  // Limit for chained Prev xrefs
    Recovery     recovery.Strategy
}
```

### 6.3 Object Loader

```go
package parser

// ObjectLoader loads PDF objects from byte stream
type ObjectLoader interface {
    // Load reads and parses an object at given reference
    Load(ctx context.Context, ref raw.ObjectRef) (raw.Object, error)
    
    // LoadIndirect resolves indirect references recursively
    LoadIndirect(ctx context.Context, ref raw.ObjectRef, depth int) (raw.Object, error)
}

// ObjectLoaderBuilder configures and builds ObjectLoader
type ObjectLoaderBuilder struct {
    reader      io.ReaderAt
    xrefTable   xref.Table
    scanner     scanner.Scanner
    security    security.Handler
    maxDepth    int  // Max indirect reference depth
    cache       Cache
    recovery    recovery.Strategy
}

func (b *ObjectLoaderBuilder) WithXRef(table xref.Table) *ObjectLoaderBuilder {
    b.xrefTable = table
    return b
}

func (b *ObjectLoaderBuilder) WithSecurity(h security.Handler) *ObjectLoaderBuilder {
    b.security = h
    return b
}

func (b *ObjectLoaderBuilder) WithCache(c Cache) *ObjectLoaderBuilder {
    b.cache = c
    return b
}

func (b *ObjectLoaderBuilder) Build() (ObjectLoader, error) {
    if b.reader == nil || b.xrefTable == nil {
        return nil, errors.New("reader and xrefTable required")
    }
    return &objectLoader{
        reader:    b.reader,
        xrefTable: b.xrefTable,
        scanner:   b.scanner,
        security:  b.security,
        maxDepth:  b.maxDepth,
        cache:     b.cache,
        recovery:  b.recovery,
    }, nil
}
```

### 6.4 Filter Pipeline

```go
package filters

// Decoder decodes a single filter type
type Decoder interface {
    // Name returns filter name (e.g., "FlateDecode", "DCTDecode")
    Name() string
    
    // Decode applies decompression/decoding to input
    Decode(ctx context.Context, input []byte, params raw.Dictionary) ([]byte, error)
}

// Pipeline chains multiple decoders
type Pipeline struct {
    decoders []Decoder
    limits   Limits
}

type Limits struct {
    MaxDecompressedSize int64  // Prevent zip bombs
    MaxDecodeTime       time.Duration
}

func (p *Pipeline) Decode(ctx context.Context, input []byte, filterNames []string, params []raw.Dictionary) ([]byte, error) {
    data := input
    
    for i, name := range filterNames {
        decoder := p.findDecoder(name)
        if decoder == nil {
            return nil, fmt.Errorf("unknown filter: %s", name)
        }
        
        // Apply size limit check
        if int64(len(data)) > p.limits.MaxDecompressedSize {
            return nil, errors.New("decompressed size exceeds limit")
        }
        
        // Decode with timeout
        decodeCtx, cancel := context.WithTimeout(ctx, p.limits.MaxDecodeTime)
        defer cancel()
        
        var err error
        var param raw.Dictionary
        if i < len(params) {
            param = params[i]
        }
        
        data, err = decoder.Decode(decodeCtx, data, param)
        if err != nil {
            return nil, fmt.Errorf("filter %s failed: %w", name, err)
        }
    }
    
    return data, nil
}

// Registry manages available decoders
type Registry struct {
    decoders map[string]Decoder
}

func (r *Registry) Register(d Decoder) {
    r.decoders[d.Name()] = d
}

func (r *Registry) Get(name string) (Decoder, bool) {
    d, ok := r.decoders[name]
    return d, ok
}

// Standard decoders
func NewFlateDecoder() Decoder
func NewLZWDecoder() Decoder
func NewDCTDecoder() Decoder  // JPEG
func NewJPXDecoder() Decoder  // JPEG2000
func NewCCITTFaxDecoder() Decoder
func NewJBIG2Decoder() Decoder
func NewRunLengthDecoder() Decoder
func NewASCII85Decoder() Decoder
func NewASCIIHexDecoder() Decoder
```

### 6.5 Security Handler

```go
package security

// Handler manages encryption/decryption
type Handler interface {
    // IsEncrypted returns true if document is encrypted
    IsEncrypted() bool
    
    // Authenticate attempts authentication with password
    Authenticate(password string) error
    
    // Decrypt decrypts object data
    Decrypt(objNum, gen int, data []byte) ([]byte, error)
    
    // Encrypt encrypts object data for writing
    Encrypt(objNum, gen int, data []byte) ([]byte, error)
    
    // Permissions returns document permissions
    Permissions() Permissions
}

// Permissions represents document access rights
type Permissions struct {
    Print           bool
    Modify          bool
    Copy            bool
    ModifyAnnotations bool
    FillForms       bool
    ExtractAccessible bool
    Assemble        bool
    PrintHighQuality bool
}

// HandlerBuilder configures security handler
type HandlerBuilder struct {
    encryptDict raw.Dictionary
    trailer     raw.Dictionary
    fileID      []byte
}

func (b *HandlerBuilder) WithEncryptDict(d raw.Dictionary) *HandlerBuilder {
    b.encryptDict = d
    return b
}

func (b *HandlerBuilder) Build() (Handler, error) {
    if b.encryptDict == nil {
        return &noEncryptionHandler{}, nil
    }
    
    // Determine security handler from /Filter
    filter := getFilter(b.encryptDict)
    
    switch filter {
    case "Standard":
        return newStandardSecurityHandler(b.encryptDict, b.fileID)
    case "AESV2":
        return newAESV2Handler(b.encryptDict, b.fileID)
    case "AESV3":
        return newAESV3Handler(b.encryptDict, b.fileID)
    default:
        return nil, fmt.Errorf("unsupported security handler: %s", filter)
    }
}
```

---

## 7. Content Stream Architecture

```go
package contentstream

// Processor parses and executes content stream operations
type Processor interface {
    // Process parses content stream and executes operations
    Process(ctx context.Context, stream []byte, state *GraphicsState) error
    
    // RegisterHandler registers a handler for an operator
    RegisterHandler(operator string, handler OperatorHandler)
}

// OperatorHandler handles a specific operator
type OperatorHandler interface {
    // Handle executes the operator with given operands
    Handle(ctx *ExecutionContext, operands []semantic.Operand) error
}

// ExecutionContext maintains state during content stream execution
type ExecutionContext struct {
    GraphicsState *GraphicsState
    TextState     *TextState
    Resources     *semantic.Resources
    Resolver      ResourceResolver
}

// GraphicsState tracks current graphics state
type GraphicsState struct {
    CTM              coords.Matrix  // Current transformation matrix
    ClippingPath     *Path
    ColorSpaceStroke *semantic.ColorSpace
    ColorSpaceFill   *semantic.ColorSpace
    ColorStroke      Color
    ColorFill        Color
    LineWidth        float64
    LineCap          LineCap
    LineJoin         LineJoin
    MiterLimit       float64
    DashPattern      []float64
    DashPhase        float64
    
    // State stack for q/Q operators
    stack []*GraphicsState
}

func (gs *GraphicsState) Save() {
    // Clone current state and push to stack
    clone := *gs
    gs.stack = append(gs.stack, &clone)
}

func (gs *GraphicsState) Restore() error {
    if len(gs.stack) == 0 {
        return errors.New("cannot restore: state stack empty")
    }
    
    // Pop from stack
    n := len(gs.stack)
    *gs = *gs.stack[n-1]
    gs.stack = gs.stack[:n-1]
    return nil
}

// TextState tracks text-specific state
type TextState struct {
    CharacterSpacing   float64
    WordSpacing        float64
    HorizontalScaling  float64
    Leading            float64
    Font               *semantic.Font
    FontSize           float64
    TextRenderMode     TextRenderMode
    TextRise           float64
    
    TextMatrix         coords.Matrix
    TextLineMatrix     coords.Matrix
}

type TextRenderMode int

const (
    TextFill TextRenderMode = iota
    TextStroke
    TextFillStroke
    TextInvisible
    TextFillClip
    TextStrokeClip
    TextFillStrokeClip
    TextClip
)

// Path represents a graphics path
type Path struct {
    Subpaths []*Subpath
}

type Subpath struct {
    Points []PathPoint
    Closed bool
}

type PathPoint struct {
    X, Y      float64
    Type      PathPointType
    Control1X, Control1Y float64  // For curves
    Control2X, Control2Y float64  // For curves
}

type PathPointType int

const (
    PathMoveTo PathPointType = iota
    PathLineTo
    PathCurveTo
    PathClose
)
```

---

## 8. Resource Resolution Architecture

```go
package resources

// Resolver resolves resource names with inheritance
type Resolver interface {
    // Resolve looks up a resource by name in given category
    Resolve(ctx context.Context, category ResourceCategory, name string, scope Scope) (raw.Object, error)
    
    // ResolveWithInheritance walks up parent chain
    ResolveWithInheritance(ctx context.Context, category ResourceCategory, name string, page *semantic.Page) (raw.Object, error)
}

type ResourceCategory string

const (
    CategoryFont       ResourceCategory = "Font"
    CategoryXObject    ResourceCategory = "XObject"
    CategoryColorSpace ResourceCategory = "ColorSpace"
    CategoryPattern    ResourceCategory = "Pattern"
    CategoryShading    ResourceCategory = "Shading"
    CategoryExtGState  ResourceCategory = "ExtGState"
    CategoryProperties ResourceCategory = "Properties"
)

// Scope represents resource lookup scope
type Scope interface {
    // LocalResources returns resources defined at this scope
    LocalResources() *semantic.Resources
    
    // ParentScope returns parent scope for inheritance
    ParentScope() Scope
}

// PageScope implements Scope for page resources
type PageScope struct {
    page   *semantic.Page
    parent Scope  // Could be Pages node
}

func (ps *PageScope) LocalResources() *semantic.Resources {
    return ps.page.Resources
}

func (ps *PageScope) ParentScope() Scope {
    return ps.parent
}

// ResolverImpl implements resource resolution with caching
type ResolverImpl struct {
    loader ObjectLoader
    cache  map[cacheKey]raw.Object
    mu     sync.RWMutex
}

type cacheKey struct {
    category ResourceCategory
    name     string
    scopeID  string  // Unique scope identifier
}

func (r *ResolverImpl) ResolveWithInheritance(
    ctx context.Context,
    category ResourceCategory,
    name string,
    page *semantic.Page,
) (raw.Object, error) {
    scope := &PageScope{page: page}
    
    // Walk up scope chain
    for scope != nil {
        res := scope.LocalResources()
        if res == nil {
            scope = scope.ParentScope()
            continue
        }
        
        // Look up in appropriate category
        var obj raw.Object
        var found bool
        
        switch category {
        case CategoryFont:
            if font, ok := res.Fonts[name]; ok {
                return font, nil
            }
        case CategoryXObject:
            if xobj, ok := res.XObjects[name]; ok {
                return xobj, nil
            }
        // ... other categories
        }
        
        if found {
            return obj, nil
        }
        
        scope = scope.ParentScope()
    }
    
    return nil, fmt.Errorf("resource not found: %s/%s", category, name)
}
```

---

## 9. Coordinate System Architecture

```go
package coords

// Matrix represents a 3x3 transformation matrix in PDF format [a b c d e f]
// [a b 0]
// [c d 0]
// [e f 1]
type Matrix [6]float64

func Identity() Matrix {
    return Matrix{1, 0, 0, 1, 0, 0}
}

func (m Matrix) Multiply(other Matrix) Matrix {
    return Matrix{
        m[0]*other[0] + m[1]*other[2],
        m[0]*other[1] + m[1]*other[3],
        m[2]*other[0] + m[3]*other[2],
        m[2]*other[1] + m[3]*other[3],
        m[4]*other[0] + m[5]*other[2] + other[4],
        m[4]*other[1] + m[5]*other[3] + other[5],
    }
}

func (m Matrix) Transform(p Point) Point {
    return Point{
        X: m[0]*p.X + m[2]*p.Y + m[4],
        Y: m[1]*p.X + m[3]*p.Y + m[5],
    }
}

func (m Matrix) Inverse() (Matrix, error) {
    det := m[0]*m[3] - m[1]*m[2]
    if math.Abs(det) < 1e-10 {
        return Matrix{}, errors.New("matrix is singular")
    }
    
    return Matrix{
        m[3] / det,
        -m[1] / det,
        -m[2] / det,
        m[0] / det,
        (m[2]*m[5] - m[3]*m[4]) / det,
        (m[1]*m[4] - m[0]*m[5]) / det,
    }, nil
}

// Translate creates a translation matrix
func Translate(tx, ty float64) Matrix {
    return Matrix{1, 0, 0, 1, tx, ty}
}

// Scale creates a scaling matrix
func Scale(sx, sy float64) Matrix {
    return Matrix{sx, 0, 0, sy, 0, 0}
}

// Rotate creates a rotation matrix (angle in radians)
func Rotate(angle float64) Matrix {
    cos := math.Cos(angle)
    sin := math.Sin(angle)
    return Matrix{cos, sin, -sin, cos, 0, 0}
}

// Point represents a 2D point
type Point struct {
    X, Y float64
}

// Rectangle with transformation support
type Rectangle struct {
    LLX, LLY, URX, URY float64
}

func (r Rectangle) Transform(m Matrix) Rectangle {
    // Transform all four corners and find bounding box
    ll := m.Transform(Point{r.LLX, r.LLY})
    lr := m.Transform(Point{r.URX, r.LLY})
    ur := m.Transform(Point{r.URX, r.URY})
    ul := m.Transform(Point{r.LLX, r.URY})
    
    minX := math.Min(math.Min(ll.X, lr.X), math.Min(ur.X, ul.X))
    minY := math.Min(math.Min(ll.Y, lr.Y), math.Min(ur.Y, ul.Y))
    maxX := math.Max(math.Max(ll.X, lr.X), math.Max(ur.X, ul.X))
    maxY := math.Max(math.Max(ll.Y, lr.Y), math.Max(ur.Y, ul.Y))
    
    return Rectangle{minX, minY, maxX, maxY}
}

// Space represents a coordinate space
type Space interface {
    // ToDevice transforms point to device space
    ToDevice(p Point) Point
    
    // FromDevice transforms point from device space
    FromDevice(p Point) Point
    
    // CTM returns current transformation matrix
    CTM() Matrix
}
```

---

## 10. Font Subsetting Architecture

```go
package fonts

// SubsettingPipeline performs end-to-end font subsetting
type SubsettingPipeline struct {
    analyzer  GlyphAnalyzer
    planner   SubsetPlanner
    generator SubsetGenerator
    embedder  FontEmbedder
}

// GlyphAnalyzer scans content streams to find used glyphs
type GlyphAnalyzer interface {
    // Analyze returns used glyph IDs for each font
    Analyze(ctx context.Context, doc *semantic.Document) (map[string]GlyphUsage, error)
}

type GlyphUsage struct {
    Font      *semantic.Font
    GlyphIDs  []uint16
    Unicode   map[uint16][]rune  // GID -> Unicode codepoints
}

// SubsetPlanner creates subsetting plan
type SubsetPlanner interface {
    // Plan creates a subset plan mapping old GIDs to new GIDs
    Plan(ctx context.Context, usage GlyphUsage) (*SubsetPlan, error)
}

type SubsetPlan struct {
    OriginalFont *semantic.Font
    OldToNew     map[uint16]uint16  // Original GID -> Subset GID
    NewToOld     map[uint16]uint16  // Subset GID -> Original GID
    GlyphCount   int
    SubsetTag    string  // 6-letter subset prefix (e.g., "ABCDEF+")
}

// SubsetGenerator generates actual font subset
type SubsetGenerator interface {
    // Generate creates subset font bytes
    Generate(ctx context.Context, plan *SubsetPlan, fontData []byte) ([]byte, error)
}

// FontEmbedder embeds subset into PDF
type FontEmbedder interface {
    // Embed updates document with subset font
    Embed(ctx context.Context, doc *semantic.Document, plan *SubsetPlan, subsetData []byte) error
    
    // GenerateToUnicode creates ToUnicode CMap
    GenerateToUnicode(plan *SubsetPlan) ([]byte, error)
}

// Complete pipeline execution
func (p *SubsettingPipeline) Subset(ctx context.Context, doc *semantic.Document) error {
    // Step 1: Analyze glyph usage
    usage, err := p.analyzer.Analyze(ctx, doc)
    if err != nil {
        return fmt.Errorf("glyph analysis failed: %w", err)
    }
    
    // Step 2: Create subset plans for each font
    for fontName, fontUsage := range usage {
        plan, err := p.planner.Plan(ctx, fontUsage)
        if err != nil {
            return fmt.Errorf("subset planning failed for %s: %w", fontName, err)
        }
        
        // Step 3: Generate subset
        fontData := getFontData(fontUsage.Font)
        subsetData, err := p.generator.Generate(ctx, plan, fontData)
        if err != nil {
            return fmt.Errorf("subset generation failed for %s: %w", fontName, err)
        }
        
        // Step 4: Embed subset
        if err := p.embedder.Embed(ctx, doc, plan, subsetData); err != nil {
            return fmt.Errorf("subset embedding failed for %s: %w", fontName, err)
        }
    }
    
    return nil
}
```

---

## 10.1 Advanced Subsetting (Shaper-Aware)

For complex scripts (Arabic, Indic, etc.) or advanced typography (ligatures, swashes), simple GID-based subsetting is insufficient because the PDF content stream might contain base characters while the font's GSUB table maps them to presentation forms (which must be present in the font).

To support this without embedding the full font:

1.  **GSUB Parsing**: Use `github.com/go-text/typesetting` to parse the `GSUB` table and understand substitution rules.
2.  **Closure Expansion**:
    *   Start with the set of GIDs used in the content stream (`UsedGIDs`).
    *   Iteratively expand this set by finding all glyphs that can be produced from `UsedGIDs` via GSUB rules (e.g., if 'f' and 'i' are used, and 'fi' ligature exists, include 'fi').
    *   *Optimization*: Only include substitutions that are reachable given the input text sequences (requires partial shaping or conservative approximation).
3.  **Table Subsetting**:
    *   Subset the `GSUB` and `GPOS` tables themselves to remove rules involving unused glyphs (reducing file size).
    *   Rebuild `GDEF` if necessary.

```go
// AdvancedSubsetter extends Subsetter
type AdvancedSubsetter interface {
    // ComputeClosure returns the transitive closure of GIDs based on GSUB rules
    ComputeClosure(initialGIDs map[int]bool, gsub *GSUBTable) map[int]bool
}
```
---

## 11. Streaming Architecture

```go
package streaming

// DocumentStream provides streaming access to PDF structure
type DocumentStream interface {
    // Events returns channel of document events
    Events() <-chan Event
    
    // Errors returns channel of errors
    Errors() <-chan error
    
    // Close stops streaming and releases resources
    Close() error
}

// Event represents a streaming document event
type Event interface {
    Type() EventType
}

type EventType int

const (
    EventDocumentStart EventType = iota
    EventDocumentEnd
    EventPageStart
    EventPageEnd
    EventContentOperation
    EventResourceRef
    EventAnnotation
    EventMetadata
)

// Specific event types
type DocumentStartEvent struct {
    Version string
    Encrypted bool
}

func (DocumentStartEvent) Type() EventType { return EventDocumentStart }

type PageStartEvent struct {
    Index    int
    MediaBox semantic.Rectangle
}

func (PageStartEvent) Type() EventType { return EventPageStart }

type ContentOperationEvent struct {
    Operation semantic.Operation
}

func (ContentOperationEvent) Type() EventType { return EventContentOperation }

// StreamConfig configures streaming behavior
type StreamConfig struct {
    BufferSize  int  // Event buffer size
    ReadAhead   int  // Pages to read ahead
    Concurrency int  // Parallel decoders
}

// Parser creates streaming document access
type Parser interface {
    Stream(ctx context.Context, r io.ReaderAt, cfg StreamConfig) (DocumentStream, error)
}

// Example streaming implementation
type documentStream struct {
    events  chan Event
    errors  chan error
    cancel  context.CancelFunc
    wg      sync.WaitGroup
}

func (ds *documentStream) Events() <-chan Event {
    return ds.events
}

func (ds *documentStream) Errors() <-chan error {
    return ds.errors
}

func (ds *documentStream) Close() error {
    ds.cancel()
    ds.wg.Wait()
    close(ds.events)
    close(ds.errors)
    return nil
}
```

---

## 12. Extension System Architecture

```go
package extensions

// Hub coordinates all extensions with phased execution
type Hub interface {
    // Register adds extension to appropriate phase
    Register(ext Extension) error
    
    // Execute runs all extensions in phase order
    Execute(ctx context.Context, doc *semantic.Document) error
    
    // Extensions returns registered extensions by phase
    Extensions(phase Phase) []Extension
}

// Phase defines extension execution phase
type Phase int

const (
    PhaseInspect   Phase = iota  // Read-only inspection
    PhaseSanitize                // Remove/fix problematic content
    PhaseTransform               // Modify document structure
    PhaseValidate                // Validate compliance
)

func (p Phase) String() string {
    return []string{"Inspect", "Sanitize", "Transform", "Validate"}[p]
}

// Extension is base interface for all extensions
type Extension interface {
    // Name returns extension identifier
    Name() string
    
    // Phase returns execution phase
    Phase() Phase
    
    // Priority returns execution priority within phase (lower runs first)
    Priority() int
    
    // Execute runs the extension
    Execute(ctx context.Context, doc *semantic.Document) error
}

// Specific extension types
type Inspector interface {
    Extension
    Inspect(ctx context.Context, doc *semantic.Document) (*InspectionReport, error)
}

type Sanitizer interface {
    Extension
    Sanitize(ctx context.Context, doc *semantic.Document) (*SanitizationReport, error)
}

type Transformer interface {
    Extension
    Transform(ctx context.Context, doc *semantic.Document) error
}

type Validator interface {
    Extension
    Validate(ctx context.Context, doc *semantic.Document) (*ValidationReport, error)
}

// Reports
type InspectionReport struct {
    PageCount    int
    FontCount    int
    ImageCount   int
    FileSize     int64
    Version      string
    Encrypted    bool
    Linearized   bool
    Tagged       bool
    Metadata     map[string]interface{}
}

type SanitizationReport struct {
    ItemsRemoved int
    ItemsFixed   int
    Actions      []SanitizationAction
}

type SanitizationAction struct {
    Type        string
    Description string
    ObjectRef   raw.ObjectRef
}

type ValidationReport struct {
    Valid   bool
    Errors  []ValidationError
    Warnings []ValidationWarning
}

type ValidationError struct {
    Code        string
    Message     string
    Location    string
    ObjectRef   raw.ObjectRef
}

type ValidationWarning struct {
    Code     string
    Message  string
    Location string
}

// HubImpl implements phased execution
type HubImpl struct {
    extensions map[Phase][]Extension
}

func NewHub() *HubImpl {
    return &HubImpl{
        extensions: make(map[Phase][]Extension),
    }
}

func (h *HubImpl) Register(ext Extension) error {
    phase := ext.Phase()
    h.extensions[phase] = append(h.extensions[phase], ext)
    
    // Sort by priority within phase
    sort.Slice(h.extensions[phase], func(i, j int) bool {
        return h.extensions[phase][i].Priority() < h.extensions[phase][j].Priority()
    })
    
    return nil
}

func (h *HubImpl) Execute(ctx context.Context, doc *semantic.Document) error {
    phases := []Phase{PhaseInspect, PhaseSanitize, PhaseTransform, PhaseValidate}
    
    for _, phase := range phases {
        exts := h.extensions[phase]
        if len(exts) == 0 {
            continue
        }
        for _, ext := range exts {
            if err := ext.Execute(ctx, doc); err != nil {
                return fmt.Errorf("extension %s failed: %w", ext.Name(), err)
            }
        }
    }
    
    return nil
}
```

---

## 13. Writer Architecture

```go
package writer

// Config configures PDF writing behavior
type Config struct {
    // PDF version to write
    Version PDFVersion
    
    // Compression level (0-9, 0=none, 9=max)
    Compression int
    
    // Enable linearization (fast web view)
    Linearize bool
    
    // Write as incremental update
    Incremental bool
    
    // Deterministic output (for reproducible builds)
    Deterministic bool
    
    // Use cross-reference streams (PDF 1.5+)
    XRefStreams bool
    
    // Use object streams (PDF 1.5+)
    ObjectStreams bool
    
    // Subset fonts
    SubsetFonts bool
    
    // PDF/A compliance level
    PDFALevel PDFALevel
}

type PDFVersion string

const (
    PDF14 PDFVersion = "1.4"
    PDF15 PDFVersion = "1.5"
    PDF16 PDFVersion = "1.6"
    PDF17 PDFVersion = "1.7"
    PDF20 PDFVersion = "2.0"
)

type PDFALevel int

const (
    PDFA1B PDFALevel = iota
    PDFA1A
    PDFA2B
    PDFA2A
    PDFA2U
    PDFA3B
    PDFA3A
    PDFA3U
)

// Writer writes PDF documents
type Writer interface {
    // Write writes document to output with config
    Write(ctx context.Context, doc *semantic.Document, w io.Writer, cfg Config) error
}

// Interceptor allows hooking into write process
type Interceptor interface {
    // BeforeWrite called before writing object
    BeforeWrite(ctx context.Context, obj raw.Object) error
    
    // AfterWrite called after writing object
    AfterWrite(ctx context.Context, obj raw.Object, bytesWritten int64) error
}

// WriterBuilder configures writer
type WriterBuilder struct {
    interceptors []Interceptor
}

func (b *WriterBuilder) WithInterceptor(i Interceptor) *WriterBuilder {
    b.interceptors = append(b.interceptors, i)
    return b
}

func (b *WriterBuilder) Build() Writer {
    return &writerImpl{
        interceptors: b.interceptors,
    }
}

// Serializer handles low-level PDF serialization
type Serializer interface {
    // SerializeObject writes a single object
    SerializeObject(w io.Writer, ref raw.ObjectRef, obj raw.Object) error
    
    // SerializeXRef writes cross-reference table
    SerializeXRef(w io.Writer, offsets map[int]int64) error
    
    // SerializeTrailer writes trailer dictionary
    SerializeTrailer(w io.Writer, trailer raw.Dictionary) error
}

// IncrementalWriter appends updates to existing PDF
type IncrementalWriter interface {
    // AppendUpdate adds new/modified objects
    AppendUpdate(ctx context.Context, original io.ReaderAt, updates *semantic.Document, w io.WriteSeeker) error
}
```

---

## 14. PDF/A Compliance Architecture

```go
package pdfa

// Enforcer ensures PDF/A compliance
type Enforcer interface {
    // Enforce transforms document to be PDF/A compliant
    Enforce(ctx context.Context, doc *semantic.Document, level PDFALevel) error
    
    // Validate checks PDF/A compliance without modification
    Validate(ctx context.Context, doc *semantic.Document, level PDFALevel) (*ComplianceReport, error)
}

type PDFALevel int

const (
    PDFA1B PDFALevel = iota
    PDFA3B
)

// ComplianceReport details compliance status
type ComplianceReport struct {
    Compliant bool
    Level     PDFALevel
    Violations []Violation
}

type Violation struct {
    Code        string
    Description string
    Location    string
}

// XMPGenerator creates XMP metadata packets
type XMPGenerator interface {
    Generate(doc *semantic.Document, level PDFALevel) ([]byte, error)
}

// ICCProfileManager handles ICC color profiles
type ICCProfileManager interface {
    // EmbedProfile embeds ICC profile for output intent
    EmbedProfile(doc *semantic.Document, profile ICCProfile) error
    
    // DefaultProfile returns appropriate default profile
    DefaultProfile(colorSpace string) ICCProfile
}

type ICCProfile struct {
    Data        []byte
    NumComponents int
    Description string
}
```

---

## 15. Error Recovery Architecture

```go
package recovery

// Strategy defines how to handle parsing errors
type Strategy interface {
    // OnError is called when error is encountered
    OnError(ctx context.Context, err error, location Location) Action
}

type Location struct {
    ByteOffset int64
    ObjectRef  raw.ObjectRef
    Component  string  // "scanner", "xref", "filter", etc.
}

// Action defines recovery action
type Action int

const (
    ActionFail    Action = iota  // Abort parsing
    ActionSkip                   // Skip problematic object
    ActionFix                    // Attempt automatic fix
    ActionWarn                   // Log warning and continue
)

// Common strategies
type StrictStrategy struct{}

func (s *StrictStrategy) OnError(ctx context.Context, err error, loc Location) Action {
    return ActionFail
}

type LenientStrategy struct{}

func (s *LenientStrategy) OnError(ctx context.Context, err error, loc Location) Action {
    // Try to fix common issues
    if isFixable(err) {
        return ActionFix
    }
    
    return ActionSkip
}

// ErrorAccumulator collects errors without failing
type ErrorAccumulator struct {
    errors   []error
    warnings []error
    mu       sync.Mutex
}

func (a *ErrorAccumulator) AddError(err error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.errors = append(a.errors, err)
}

func (a *ErrorAccumulator) AddWarning(err error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.warnings = append(a.warnings, err)
}

func (a *ErrorAccumulator) Errors() []error {
    a.mu.Lock()
    defer a.mu.Unlock()
    return append([]error(nil), a.errors...)
}

func (a *ErrorAccumulator) HasErrors() bool {
    a.mu.Lock()
    defer a.mu.Unlock()
    return len(a.errors) > 0
}
```

---

## 17. High-Level Builder API

```go
package builder

// PDFBuilder provides fluent API for PDF construction
type PDFBuilder interface {
    // NewPage adds a new page with dimensions
    NewPage(width, height float64) PageBuilder
    
    // AddPage adds an existing page
    AddPage(page *semantic.Page) PDFBuilder
    
    // SetInfo sets document metadata
    SetInfo(info *semantic.DocumentInfo) PDFBuilder
    
    // SetMetadata sets XMP metadata
    SetMetadata(xmp []byte) PDFBuilder
    
    // RegisterFont adds a font for use in pages
    RegisterFont(name string, font *semantic.Font) PDFBuilder
    
    // Build constructs the final document
    Build() (*semantic.Document, error)
}

// PageBuilder provides fluent API for page construction
type PageBuilder interface {
    // DrawText draws text at position
    DrawText(text string, x, y float64, opts TextOptions) PageBuilder
    
    // DrawPath draws a vector path
    DrawPath(path *contentstream.Path, opts PathOptions) PageBuilder
    
    // DrawImage draws an image
    DrawImage(img *semantic.Image, x, y, width, height float64, opts ImageOptions) PageBuilder
    
    // DrawRectangle draws a rectangle
    DrawRectangle(x, y, width, height float64, opts RectOptions) PageBuilder
    
    // DrawLine draws a line
    DrawLine(x1, y1, x2, y2 float64, opts LineOptions) PageBuilder
    
    // AddAnnotation adds an annotation
    AddAnnotation(ann *semantic.Annotation) PageBuilder
    
    // SetMediaBox sets page media box
    SetMediaBox(box semantic.Rectangle) PageBuilder
    
    // SetCropBox sets page crop box
    SetCropBox(box semantic.Rectangle) PageBuilder
    
    // SetRotation sets page rotation
    SetRotation(degrees int) PageBuilder
    
    // Finish completes page and returns to document builder
    Finish() PDFBuilder
}

// TextOptions configures text drawing
type TextOptions struct {
    Font         string
    FontSize     float64
    Color        Color
    RenderMode   contentstream.TextRenderMode
    CharSpacing  float64
    WordSpacing  float64
    HorizScaling float64
    Rise         float64
}

// PathOptions configures path drawing
type PathOptions struct {
    StrokeColor Color
    FillColor   Color
    LineWidth   float64
    LineCap     contentstream.LineCap
    LineJoin    contentstream.LineJoin
    DashPattern []float64
    DashPhase   float64
    Fill        bool
    Stroke      bool
}

// ImageOptions configures image drawing
type ImageOptions struct {
    Interpolate bool
    SMask       *semantic.Image
}

type Color struct {
    R, G, B float64  // 0.0 to 1.0
    A       float64  // Alpha (if supported)
}

// Example usage
func ExampleBuilder() (*semantic.Document, error) {
    b := NewBuilder()
    
    page := b.NewPage(612, 792).  // US Letter
        SetMediaBox(semantic.Rectangle{0, 0, 612, 792}).
        DrawText("Hello, World!", 100, 700, TextOptions{
            Font:     "Helvetica",
            FontSize: 24,
            Color:    Color{R: 0, G: 0, B: 0},
        }).
        DrawRectangle(50, 50, 200, 100, RectOptions{
            StrokeColor: Color{R: 1, G: 0, B: 0},
            LineWidth:   2,
            Stroke:      true,
        }).
        Finish()
    
    return b.Build()
}
```

---

## 18. Concurrency Model

### 18.1 Thread Safety

| Component | Thread-Safe | Notes |
|-----------|-------------|-------|
| `Scanner` | No | Create per-goroutine |
| `Parser` | No | Create per-goroutine |
| `raw.Document` | Read-only after construction | Immutable after parsing |
| `semantic.Document` | No (mutable) | External synchronization required |
| `ObjectLoader` | Yes (with cache locks) | Internal synchronization |
| `FilterPipeline` | Yes | Stateless decoders |
| `Resources.Resolver` | Yes | Internal synchronization |
| `Writer` | No | Create per-goroutine |
| `ExtensionHub` | No | Sequential execution |

### 18.2 Parallel Processing Opportunities

The `ir/decoded` package implements parallel stream decoding using a worker pool pattern. The concurrency level defaults to `GOMAXPROCS`.

```go
// Internal implementation in ir/decoded/decoder_impl.go
func (d *decoderImpl) Decode(ctx context.Context, rawDoc *raw.Document) (*DecodedDocument, error) {
    // ...
    workers := runtime.GOMAXPROCS(0)
    sem := make(chan struct{}, workers)
    // ...
    for _, t := range tasks {
        go func(t task) {
            sem <- struct{}{} // Acquire token
            // Decode stream...
            <-sem // Release token
        }(t)
    }
    // ...
}
```

---

## 19. Security Architecture

### 19.1 Security Limits

```go
package security

// Limits defines security boundaries
type Limits struct {
    // Maximum decompressed stream size (prevent zip bombs)
    MaxDecompressedSize int64  // Default: 100 MB
    
    // Maximum indirect reference depth (prevent stack overflow)
    MaxIndirectDepth int  // Default: 100
    
    // Maximum XRef chain depth (Prev entries)
    MaxXRefDepth int  // Default: 50
    
    // Maximum XObject nesting depth
    MaxXObjectDepth int  // Default: 20
    
    // Maximum array size
    MaxArraySize int  // Default: 100,000
    
    // Maximum dictionary size
    MaxDictSize int  // Default: 10,000
    
    // Maximum string length
    MaxStringLength int64  // Default: 10 MB
    
    // Maximum decode time per stream
    MaxDecodeTime time.Duration  // Default: 30s
    
    // Maximum total parse time
    MaxParseTime time.Duration  // Default: 5m
}

// Enforcer applies security limits
type Enforcer struct {
    limits Limits
}

func (e *Enforcer) CheckDecompressedSize(size int64) error {
    if size > e.limits.MaxDecompressedSize {
        return fmt.Errorf("decompressed size %d exceeds limit %d", 
            size, e.limits.MaxDecompressedSize)
    }
    return nil
}

func (e *Enforcer) CheckIndirectDepth(depth int) error {
    if depth > e.limits.MaxIndirectDepth {
        return fmt.Errorf("indirect reference depth %d exceeds limit %d",
            depth, e.limits.MaxIndirectDepth)
    }
    return nil
}
```

### 19.2 Input Validation

```go
// Sanitizer removes potentially malicious content
type Sanitizer interface {
    // RemoveJavaScript removes all JavaScript
    RemoveJavaScript(doc *semantic.Document) error
    
    // RemoveURIs removes all URI actions
    RemoveURIs(doc *semantic.Document) error
    
    // RemoveLaunchActions removes all launch actions
    RemoveLaunchActions(doc *semantic.Document) error
    
    // SanitizeNames ensures names don't contain exploits
    SanitizeNames(doc *semantic.Document) error
}
```

---

## 20. Layout Engine

The `layout` package provides a high-level engine for converting structured content (Markdown, HTML) into PDF pages using the `builder` API.

### 20.1 Engine Architecture

```go
package layout

import (
    "github.com/yuin/goldmark"
    "golang.org/x/net/html"
)

// Engine handles layout and rendering
type Engine struct {
    b builder.PDFBuilder
    
    // Configuration
    DefaultFont     string
    DefaultFontSize float64
    LineHeight      float64
    Margins         Margins
    
    // State
    currentPage builder.PageBuilder
    cursorX     float64
    cursorY     float64
}

// RenderMarkdown renders markdown text using goldmark
func (e *Engine) RenderMarkdown(text string) error

// RenderHTML renders HTML text using net/html
func (e *Engine) RenderHTML(text string) error
```

### 20.2 Supported Features

*   **Markdown** (via `goldmark`):
    *   Headers (H1-H6)
    *   Paragraphs with word wrapping
    *   Unordered lists (bullets)
    *   Automatic pagination
*   **HTML** (via `net/html`):
    *   Basic tags (p, h1-h6, ul, li)
    *   Automatic pagination

---

## 21. Testing Strategy

### 20.1 Test Corpus

```
tests/
├── corpus/
│   ├── valid/           # Well-formed PDFs
│   │   ├── simple.pdf
│   │   ├── images.pdf
│   │   ├── forms.pdf
│   │   └── fonts.pdf
│   ├── malformed/       # Broken PDFs for recovery testing
│   │   ├── truncated.pdf
│   │   ├── bad-xref.pdf
│   │   └── corrupt-stream.pdf
│   ├── security/        # Security test cases
│   │   ├── zipbomb.pdf
│   │   ├── deeply-nested.pdf
│   │   └── huge-array.pdf
│   └── pdfa/           # PDF/A compliance tests
│       ├── pdfa1b-valid.pdf
│       └── pdfa1b-invalid.pdf
├── golden/             # Expected outputs
└── fuzz/              # Fuzzing inputs
```

### 20.2 Test Categories

```go
// Unit tests for each module
package scanner_test

func TestScannerBasicTokens(t *testing.T) {}
func TestScannerNestedStructures(t *testing.T) {}
func TestScannerMalformed(t *testing.T) {}

// Integration tests
package integration_test

func TestParseSimplePDF(t *testing.T) {}
func TestParseAndWrite(t *testing.T) {}
func TestFontSubsetting(t *testing.T) {}

// Fuzz tests
func FuzzScanner(f *testing.F) {}
func FuzzParser(f *testing.F) {}

// Benchmark tests
func BenchmarkParse(b *testing.B) {}
func BenchmarkWrite(b *testing.B) {}
```

---

## 22. Performance Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Parse 100-page PDF | < 1s | Text and images |
| Parse 1000-page PDF | < 10s | Streaming mode |
| Write 100-page PDF | < 2s | Without subsetting |
| Font subsetting | < 500ms | Per font |
| PDF/A validation | < 1s | 100-page document |
| Memory usage (parse) | < 100 MB | For 100-page PDF |
| Memory usage (streaming) | < 50 MB | For any size PDF |

---

## 23. Roadmap

### Phase 1: Core Foundation (Months 1-3)
- Scanner and tokenizer
- XRef resolution
- Raw IR parsing
- Filter pipeline (Flate, LZW, ASCII)
- Basic object loader
- Simple writer (no subsetting)

### Phase 2: Semantic Layer (Months 4-6)
- Decoded IR
- Semantic IR (pages, fonts, images)
- Content stream parser
- Resource resolution
- Graphics state tracking
- Coordinate transformations

### Phase 3: Advanced Features (Months 7-9)
- Font subsetting (TTF/OTF)
- ToUnicode CMap generation
- Security/encryption support
- Incremental updates
- Extension system
- Error recovery

### Phase 4: Compliance & Quality (Months 10-12)
- PDF/A validation
- PDF/A enforcement
- Linearization
- Comprehensive test suite
- Performance optimization
- Documentation and examples

### Phase 5: Ecosystem (Months 13+)
- Standard plugins (OCR, watermarking, redaction)
- CLI tools
- CJK font support
- PDF/A-2/3 compliance
- Digital signatures
- Advanced optimizations

---

## 24. Dependencies

### Standard Library
- `io`, `io/ioutil`, `io/fs`
- `bytes`, `bufio`
- `compress/flate`, `compress/zlib`, `compress/lzw`
- `image`, `image/jpeg`, `image/png`
- `encoding/binary`, `encoding/hex`, `encoding/base64`
- `crypto/aes`, `crypto/md5`, `crypto/rc4`, `crypto/sha256`
- `context`, `sync`, `time`

### Third-Party (Minimal)
- Font parsing: `golang.org/x/image/font/sfnt`
- JPEG2000: Consider `github.com/strukturag/libheif` bindings
- JBIG2: Consider CGo bindings to `jbig2dec`
- Logging: Pluggable (support `log/slog`, `zap`, `logrus`)
- Metrics: Pluggable (support Prometheus, StatsD)

---

## 25. API Stability

### Versioning
- Semantic versioning: `v1.x.x`, `v2.x.x`
- Major version for breaking changes
- Minor version for new features
- Patch version for bug fixes

### Stability Guarantees
- `ir/raw`, `ir/decoded`, `ir/semantic`: Stable from v1.0
- `builder`: Stable from v1.0
- `filters`, `fonts`, `writer`: Stable from v1.0
- `extensions`: Extension interface stable, implementations may evolve

### Deprecation Policy
- Deprecated APIs supported for minimum 6 months
- Deprecation warnings in documentation and code comments
- Migration guides provided

---

## 26. References

### Specifications
- **ISO 32000-1:2008** - PDF 1.7 specification
- **ISO 32000-2:2020** - PDF 2.0 specification
- **ISO 19005-1:2005** - PDF/A-1
- **ISO 19005-2:2011** - PDF/A-2
- **ISO 19005-3:2012** - PDF/A-3
- **ISO 14289-1:2014** - PDF/UA (Universal Accessibility)

### Font Specifications
- **OpenType specification** (Microsoft/Adobe)
- **TrueType Reference Manual** (Apple)
- **CFF specification** (Adobe)
- **CID-Keyed Font Technology Overview** (Adobe)

### Related Libraries (Reference)
- **pdfcpu** (Go) - Command-line PDF processor
- **gopdf** (Go) - Simple PDF creator
- **PDFBox** (Java) - Apache PDF library
- **PyPDF2** (Python) - PDF toolkit
- **pdf-lib** (TypeScript) - PDF creation/modification

---

## 27. Appendix: Example Workflows

### Example 1: Parse and Extract Text

```go
package main

import (
    "context"
    "pdflib/parser"
    "pdflib/ir/semantic"
)

func main() {
    file, _ := os.Open("document.pdf")
    defer file.Close()
    
    // Parse PDF
    p := parser.NewParser(parser.Config{})
    doc, err := p.Parse(context.Background(), file)
    if err != nil {
        log.Fatal(err)
    }
    
    // Extract text from all pages
    for i, page := range doc.Pages {
        text := extractText(page)
        fmt.Printf("Page %d:\n%s\n\n", i+1, text)
    }
}

func extractText(page *semantic.Page) string {
    var buf bytes.Buffer
    
    for _, content := range page.Contents {
        for _, op := range content.Operations {
            if op.Operator == "Tj" || op.Operator == "TJ" {
                // Extract text from show operators
                buf.WriteString(decodeText(op.Operands[0]))
            }
        }
    }
    
    return buf.String()
}
```

### Example 2: Create Simple PDF

```go
package main

import (
    "pdflib/builder"
    "pdflib/writer"
)

func main() {
    // Build document
    b := builder.NewBuilder()
    
    b.NewPage(612, 792).
        DrawText("Hello, PDF!", 100, 700, builder.TextOptions{
            Font:     "Helvetica",
            FontSize: 24,
            Color:    Color{R: 0, G: 0, B: 0},
        }).
        DrawRectangle(100, 650, 200, 50, builder.RectOptions{
            StrokeColor: Color{R: 1, G: 0, B: 0},
            LineWidth:   2,
            Stroke:      true,
        }).
        Finish()
    
    doc, _ := b.Build()
    
    // Write to file
    w := writer.NewWriter()
    out, _ := os.Create("output.pdf")
    defer out.Close()
    
    w.Write(context.Background(), doc, out, writer.Config{
        Version:     writer.PDF17,
        Compression: 9,
    })
}
```

### Example 3: Font Subsetting

```go
package main

import (
    "pdflib/fonts"
    "pdflib/parser"
)

func main() {
    // Parse input PDF
    file, _ := os.Open("input.pdf")
    doc, _ := parser.NewParser(parser.Config{}).Parse(context.Background(), file)
    file.Close()
    
    // Subset fonts
    pipeline := fonts.NewSubsettingPipeline()
    err := pipeline.Subset(context.Background(), doc)
    if err != nil {
        log.Fatal(err)
    }
    
    // Write with subsetted fonts
    w := writer.NewWriter()
    out, _ := os.Create("output.pdf")
    defer out.Close()
    
    w.Write(context.Background(), doc, out, writer.Config{
        SubsetFonts: true,
    })
}
```

### Example 4: PDF/A Conversion

```go
package main

import (
    "pdflib/pdfa"
    "pdflib/parser"
    "pdflib/writer"
)

func main() {
    // Parse input
    file, _ := os.Open("input.pdf")
    doc, _ := parser.NewParser(parser.Config{}).Parse(context.Background(), file)
    file.Close()
    
    // Convert to PDF/A-1b
    enforcer := pdfa.NewEnforcer()
    err := enforcer.Enforce(context.Background(), doc, pdfa.PDFA1B)
    if err != nil {
        log.Fatal(err)
    }
    
    // Validate
    report, _ := enforcer.Validate(context.Background(), doc, pdfa.PDFA1B)
    if !report.Compliant {
        log.Fatal("Failed to convert to PDF/A-1b")
    }
    
    // Write PDF/A compliant file
    w := writer.NewWriter()
    out, _ := os.Create("output-pdfa.pdf")
    defer out.Close()
    
    w.Write(context.Background(), doc, out, writer.Config{
        Version:   writer.PDF14,
        PDFALevel: writer.PDFA1B,
    })
}
```

---

**End of Design Document v2.0**
