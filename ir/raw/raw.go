package raw

import (
	"context"
	"fmt"
	"io"
)

// ObjectRef uniquely identifies an indirect PDF object.
type ObjectRef struct {
	Num int
	Gen int
}

func (r ObjectRef) String() string { return fmt.Sprintf("%d %d R", r.Num, r.Gen) }

// Object is the base interface for all raw PDF objects.
type Object interface {
	Type() string
	IsIndirect() bool
}

// Dictionary represents a PDF dictionary object.
type Dictionary interface {
	Object
	Get(key Name) (Object, bool)
	Set(key Name, value Object)
	Keys() []Name
	Len() int
}

// Array represents a PDF array object.
type Array interface {
	Object
	Get(index int) (Object, bool)
	Len() int
	Append(obj Object)
}

// Stream represents a raw (undecoded) PDF stream.
type Stream interface {
	Object
	Dictionary() Dictionary
	RawData() []byte
	Length() int64
}

// Name represents a PDF name object.
type Name interface {
	Object
	Value() string
}

// String represents a PDF string (literal or hex).
type String interface {
	Object
	Value() []byte
	IsHex() bool
}

// Number represents a PDF numeric value.
type Number interface {
	Object
	Int() int64
	Float() float64
	IsInteger() bool
}

// Boolean represents a PDF boolean.
type Boolean interface {
	Object
	Value() bool
}

// Null represents the PDF null object.
type Null interface{ Object }

// Reference represents an indirect object reference.
type Reference interface {
	Object
	Ref() ObjectRef
}

// DocumentMetadata contains common PDF info fields.
type DocumentMetadata struct {
	Producer string
	Creator  string
	Title    string
	Author   string
	Subject  string
	Keywords []string
}

// Permissions describes allowed actions expressed in the parsed document.
type Permissions struct {
	Print, Modify, Copy, ModifyAnnotations, FillForms, ExtractAccessible, Assemble, PrintHighQuality bool
}

// Document is the root container for raw PDF objects.
type Document struct {
	Objects           map[ObjectRef]Object
	Trailer           Dictionary
	Version           string // e.g., "1.7"
	Metadata          DocumentMetadata
	Permissions       Permissions
	MetadataEncrypted bool
	Encrypted         bool
	Linearized        bool
	HintTable         *HintTable
}

// Parser converts bytes into a raw.Document.
type Parser interface {
	Parse(ctx context.Context, r io.ReaderAt) (*Document, error)
}
