package resources

import (
	"context"
	"fmt"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type ResourceCategory string

const (
	CategoryFont    ResourceCategory = "Font"
	CategoryXObject ResourceCategory = "XObject"
)

type Scope interface {
	LocalResources() *semantic.Resources
	ParentScope() Scope
}

type PageScope struct {
	Page   *semantic.Page
	Parent Scope
}

func (ps *PageScope) LocalResources() *semantic.Resources { return ps.Page.Resources }
func (ps *PageScope) ParentScope() Scope                  { return ps.Parent }

type Resolver interface {
	ResolveWithInheritance(ctx context.Context, category ResourceCategory, name string, page *semantic.Page) (raw.Object, error)
}

type resolverImpl struct {
	doc *semantic.Document
}

func NewResolver(doc *semantic.Document) Resolver { return &resolverImpl{doc: doc} }

func (r *resolverImpl) ResolveWithInheritance(ctx context.Context, category ResourceCategory, name string, page *semantic.Page) (raw.Object, error) {
	var scope Scope = &PageScope{Page: page}
	for scope != nil {
		res := scope.LocalResources()
		if res == nil {
			scope = scope.ParentScope()
			continue
		}
		if category == CategoryFont {
			if font, ok := res.Fonts[name]; ok {
				if font.OriginalRef.Num == 0 {
					return nil, fmt.Errorf("font %s has no raw object", name)
				}
				if r.doc == nil || r.doc.Decoded() == nil || r.doc.Decoded().Raw == nil {
					return nil, fmt.Errorf("document context missing")
				}
				if obj, ok := r.doc.Decoded().Raw.Objects[font.OriginalRef]; ok {
					return obj, nil
				}
				return nil, fmt.Errorf("raw object %s not found", font.OriginalRef)
			}
		}
		scope = scope.ParentScope()
	}
	return nil, fmt.Errorf("resource not found: %s/%s", category, name)
}

type Context interface{ Done() <-chan struct{} }
