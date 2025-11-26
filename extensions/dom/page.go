package dom

import (
	"github.com/wudi/pdfkit/ir/semantic"
)

type PageProxy struct {
	page *semantic.Page
}

func NewPageProxy(p *semantic.Page) *PageProxy {
	return &PageProxy{page: p}
}

func (p *PageProxy) GetIndex() int {
	return p.page.Index
}
