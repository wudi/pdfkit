package builder

import (
	"fmt"
	"pdflib/ir/semantic"
)

type PDFBuilder interface { NewPage(width, height float64) PageBuilder; AddPage(page *semantic.Page) PDFBuilder; SetInfo(info *semantic.DocumentInfo) PDFBuilder; SetMetadata(xmp []byte) PDFBuilder; RegisterFont(name string, font *semantic.Font) PDFBuilder; Build() (*semantic.Document, error) }

type PageBuilder interface { DrawText(text string, x, y float64, opts TextOptions) PageBuilder; Finish() PDFBuilder }

type TextOptions struct { Font string; FontSize float64; X float64; Y float64; Text string }

type builderImpl struct { pages []*semantic.Page; info *semantic.DocumentInfo; metadata []byte }

type pageBuilderImpl struct { parent *builderImpl; page *semantic.Page }

func NewBuilder() PDFBuilder { return &builderImpl{} }

func (b *builderImpl) NewPage(w,h float64) PageBuilder { p:=&semantic.Page{MediaBox: semantic.Rectangle{0,0,w,h}}; b.pages = append(b.pages, p); return &pageBuilderImpl{parent:b, page:p} }
func (b *builderImpl) AddPage(p *semantic.Page) PDFBuilder { b.pages = append(b.pages, p); return b }
func (b *builderImpl) SetInfo(info *semantic.DocumentInfo) PDFBuilder { b.info = info; return b }
func (b *builderImpl) SetMetadata(xmp []byte) PDFBuilder { b.metadata = xmp; return b }
func (b *builderImpl) RegisterFont(name string, font *semantic.Font) PDFBuilder { return b }
func (b *builderImpl) Build() (*semantic.Document, error) { return &semantic.Document{Pages:b.pages, Info:b.info, Metadata:&semantic.XMPMetadata{Raw:b.metadata}}, nil }

func (p *pageBuilderImpl) DrawText(text string, x, y float64, opts TextOptions) PageBuilder {
	// Very naive content stream construction (no escaping, assumes ASCII)
	cs := "BT "
	fontName := "F1"
	if opts.FontSize == 0 { opts.FontSize = 12 }
	cs += "/" + fontName + " " + fmt.Sprintf("%d", int(opts.FontSize)) + " Tf "
	cs += fmt.Sprintf("%f %f Td ", x, y)
	cs += "(" + text + ") Tj ET\n"
	p.page.Contents = append(p.page.Contents, semantic.ContentStream{RawBytes: []byte(cs)})
	if p.page.Resources == nil { p.page.Resources = &semantic.Resources{Fonts: map[string]*semantic.Font{}} }
	if _, ok := p.page.Resources.Fonts[fontName]; !ok {
		p.page.Resources.Fonts[fontName] = &semantic.Font{BaseFont: "Helvetica"}
	}
	return p
}
func (p *pageBuilderImpl) Finish() PDFBuilder { return p.parent }
