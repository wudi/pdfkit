package semantic

import (
	"context"

	"pdflib/ir/decoded"
)

// NewBuilder returns a minimal semantic builder that wraps decoded docs.
func NewBuilder() Builder {
	return &builderImpl{}
}

type builderImpl struct{}

func (b *builderImpl) Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error) {
	return &Document{decoded: dec}, nil
}
