package semantic

import (
	"context"
	"fmt"

	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
)

// NewBuilder returns a minimal semantic builder that wraps decoded docs.
func NewBuilder() Builder {
	return &builderImpl{}
}

type builderImpl struct{}

func (b *builderImpl) Build(ctx context.Context, dec *decoded.DecodedDocument) (*Document, error) {
	doc := &Document{
		decoded:           dec,
		Permissions:       dec.Perms,
		Encrypted:         dec.Encrypted,
		MetadataEncrypted: dec.MetadataEncrypted,
	}

	if dec.Raw != nil && dec.Raw.Trailer != nil {
		resolver := &simpleResolver{doc: dec.Raw}

		// Get Root (Catalog)
		rootObj, ok := dec.Raw.Trailer.Get(raw.NameLiteral("Root"))
		if ok {
			// Resolve if indirect
			if ref, ok := rootObj.(raw.Reference); ok {
				o, err := resolver.Resolve(ref.Ref())
				if err == nil {
					rootObj = o
				}
			}

			if catalog, ok := rootObj.(*raw.DictObj); ok {
				// Parse Pages
				if pagesObj, ok := catalog.Get(raw.NameLiteral("Pages")); ok {
					pages, err := parsePages(pagesObj, resolver, inheritedPageProps{})
					if err != nil {
						fmt.Printf("Warning: failed to parse pages: %v\n", err)
					} else {
						doc.Pages = pages
					}
				}

				// Parse OutputIntents (Document Level)
				if oiObj, ok := catalog.Get(raw.NameLiteral("OutputIntents")); ok {
					ois, err := parseOutputIntents(oiObj, resolver)
					if err != nil {
						fmt.Printf("Warning: failed to parse document OutputIntents: %v\n", err)
					} else {
						doc.OutputIntents = ois
					}
				}

				st, err := parseStructureTree(catalog, resolver)
				if err != nil {
					// Log warning or ignore? Structure tree errors shouldn't necessarily fail the whole doc load.
					// For now, we'll ignore or log.
					fmt.Printf("Warning: failed to parse structure tree: %v\n", err)
				} else {
					doc.StructTree = st
				}
			}
		}
	}

	return doc, nil
}

type simpleResolver struct {
	doc *raw.Document
}

func (r *simpleResolver) Resolve(ref raw.ObjectRef) (raw.Object, error) {
	if obj, ok := r.doc.Objects[ref]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %v not found", ref)
}
