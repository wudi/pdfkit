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

type resolverImpl struct{}

func NewResolver() Resolver { return &resolverImpl{} }

func (r *resolverImpl) ResolveWithInheritance(ctx context.Context, category ResourceCategory, name string, page *semantic.Page) (raw.Object, error) {
	var scope Scope = &PageScope{Page: page}
	for scope != nil {
		res := scope.LocalResources()
		if res == nil {
			scope = scope.ParentScope()
			continue
		}
		if category == CategoryFont {
			if _, ok := res.Fonts[name]; ok {
				// Font found (placeholder); real implementation would load raw object.
				return nil, nil
			}
		}
		scope = scope.ParentScope()
	}
	return nil, fmt.Errorf("resource not found: %s/%s", category, name)
}

type Context interface{ Done() <-chan struct{} }
