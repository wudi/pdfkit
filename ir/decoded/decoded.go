package decoded

import (
	"context"

	"github.com/wudi/pdfkit/ir/raw"
)

// Object wraps a raw object after decoding.
type Object interface {
	Raw() raw.Object
	Type() string
}

// Stream represents a decoded PDF stream (decompressed/decrypted).
type Stream interface {
	Object
	Dictionary() raw.Dictionary
	Data() []byte
	Filters() []string
}

// DecodedDocument contains decoded objects plus a back-reference to the raw doc.
type DecodedDocument struct {
	Raw               *raw.Document
	Streams           map[raw.ObjectRef]Stream
	Perms             raw.Permissions
	Encrypted         bool
	MetadataEncrypted bool
}

// Decoder transforms Raw IR into Decoded IR (applies filters/security).
type Decoder interface {
	Decode(ctx context.Context, rawDoc *raw.Document) (*DecodedDocument, error)
}
