package builder

import (
	"fmt"

	"pdflib/contentstream"
	"pdflib/ir/semantic"
)

// PDFBuilder provides a fluent API for PDF construction.
type PDFBuilder interface {
	NewPage(width, height float64) PageBuilder
	AddPage(page *semantic.Page) PDFBuilder
	SetInfo(info *semantic.DocumentInfo) PDFBuilder
	SetMetadata(xmp []byte) PDFBuilder
	SetLanguage(lang string) PDFBuilder
	SetMarked(marked bool) PDFBuilder
	AddPageLabel(pageIndex int, prefix string) PDFBuilder
	AddOutline(out Outline) PDFBuilder
	RegisterFont(name string, font *semantic.Font) PDFBuilder
	Build() (*semantic.Document, error)
}

// PageBuilder provides a fluent API for page construction.
type PageBuilder interface {
	DrawText(text string, x, y float64, opts TextOptions) PageBuilder
	DrawPath(path *contentstream.Path, opts PathOptions) PageBuilder
	DrawImage(img *semantic.Image, x, y, width, height float64, opts ImageOptions) PageBuilder
	DrawRectangle(x, y, width, height float64, opts RectOptions) PageBuilder
	DrawLine(x1, y1, x2, y2 float64, opts LineOptions) PageBuilder
	AddAnnotation(ann *semantic.Annotation) PageBuilder
	SetMediaBox(box semantic.Rectangle) PageBuilder
	SetCropBox(box semantic.Rectangle) PageBuilder
	SetRotation(degrees int) PageBuilder
	Finish() PDFBuilder
}

// TextOptions configures text drawing.
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

// PathOptions configures path drawing.
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

// RectOptions configures rectangle drawing (defaults to stroke if neither fill nor stroke is set).
type RectOptions = PathOptions

// LineOptions configures line drawing.
type LineOptions struct {
	StrokeColor Color
	LineWidth   float64
	LineCap     contentstream.LineCap
	DashPattern []float64
	DashPhase   float64
}

// ImageOptions configures image drawing.
type ImageOptions struct {
	Interpolate bool
	SMask       *semantic.Image
}

// Color represents an RGB color (alpha is ignored for now).
type Color struct {
	R, G, B float64
	A       float64
}

// Outline defines a bookmark entry for the builder API.
// Page or PageIndex can be set; if both are provided, Page takes precedence.
type Outline struct {
	Title     string
	Page      *semantic.Page
	PageIndex int
	X         *float64
	Y         *float64
	Zoom      *float64
	Children  []Outline
}

type builderImpl struct {
	pages        []*semantic.Page
	info         *semantic.DocumentInfo
	metadata     []byte
	lang         string
	marked       bool
	pageLabels   map[int]string
	outlines     []Outline
	fonts        map[string]*semantic.Font
	defaultFont  string
	xobjectCount int
	xobjectNames map[*semantic.Image]string
}

type pageBuilderImpl struct {
	parent *builderImpl
	page   *semantic.Page
}

const (
	defaultFontResource = "F1"
	defaultBaseFont     = "Helvetica"
)

// NewBuilder constructs a PDFBuilder.
func NewBuilder() PDFBuilder { return &builderImpl{defaultFont: defaultFontResource} }

func (b *builderImpl) NewPage(w, h float64) PageBuilder {
	p := &semantic.Page{MediaBox: semantic.Rectangle{0, 0, w, h}}
	b.pages = append(b.pages, p)
	return &pageBuilderImpl{parent: b, page: p}
}

func (b *builderImpl) AddPage(p *semantic.Page) PDFBuilder {
	b.pages = append(b.pages, p)
	return b
}

func (b *builderImpl) SetInfo(info *semantic.DocumentInfo) PDFBuilder {
	b.info = info
	return b
}

func (b *builderImpl) SetMetadata(xmp []byte) PDFBuilder {
	b.metadata = xmp
	return b
}

func (b *builderImpl) SetLanguage(lang string) PDFBuilder {
	b.lang = lang
	return b
}

func (b *builderImpl) SetMarked(marked bool) PDFBuilder {
	b.marked = marked
	return b
}

func (b *builderImpl) AddPageLabel(pageIndex int, prefix string) PDFBuilder {
	if b.pageLabels == nil {
		b.pageLabels = make(map[int]string)
	}
	b.pageLabels[pageIndex] = prefix
	return b
}

func (b *builderImpl) AddOutline(out Outline) PDFBuilder {
	b.outlines = append(b.outlines, out)
	return b
}

func (b *builderImpl) RegisterFont(name string, font *semantic.Font) PDFBuilder {
	if b.fonts == nil {
		b.fonts = make(map[string]*semantic.Font)
	}
	b.fonts[name] = font
	if b.defaultFont == "" {
		b.defaultFont = name
	}
	return b
}

func (b *builderImpl) Build() (*semantic.Document, error) {
	pageIndexByPtr := make(map[*semantic.Page]int, len(b.pages))
	for i, p := range b.pages {
		p.Index = i
		pageIndexByPtr[p] = i
	}
	doc := &semantic.Document{
		Pages:  b.pages,
		Info:   b.info,
		Lang:   b.lang,
		Marked: b.marked,
	}
	if len(b.pageLabels) > 0 {
		doc.PageLabels = b.pageLabels
	}
	if len(b.outlines) > 0 {
		doc.Outlines = make([]semantic.OutlineItem, 0, len(b.outlines))
		for _, out := range b.outlines {
			doc.Outlines = append(doc.Outlines, b.convertOutline(out, pageIndexByPtr))
		}
	}
	if len(b.metadata) > 0 {
		doc.Metadata = &semantic.XMPMetadata{Raw: b.metadata}
	}
	return doc, nil
}

func (p *pageBuilderImpl) DrawText(text string, x, y float64, opts TextOptions) PageBuilder {
	ops := p.ensureContentOps()
	res := p.ensureResources()

	font, fontName := p.parent.fontForName(opts.Font)
	if res.Fonts == nil {
		res.Fonts = make(map[string]*semantic.Font)
	}
	if _, ok := res.Fonts[fontName]; !ok {
		res.Fonts[fontName] = font
	}
	size := opts.FontSize
	if size <= 0 {
		size = 12
	}

	*ops = append(*ops, semantic.Operation{Operator: "BT"})
	*ops = append(*ops, semantic.Operation{
		Operator: "Tf",
		Operands: []semantic.Operand{semantic.NameOperand{Value: fontName}, semantic.NumberOperand{Value: size}},
	})
	if opts.CharSpacing != 0 {
		*ops = append(*ops, semantic.Operation{Operator: "Tc", Operands: []semantic.Operand{semantic.NumberOperand{Value: opts.CharSpacing}}})
	}
	if opts.WordSpacing != 0 {
		*ops = append(*ops, semantic.Operation{Operator: "Tw", Operands: []semantic.Operand{semantic.NumberOperand{Value: opts.WordSpacing}}})
	}
	if opts.HorizScaling != 0 && opts.HorizScaling != 100 {
		*ops = append(*ops, semantic.Operation{Operator: "Tz", Operands: []semantic.Operand{semantic.NumberOperand{Value: opts.HorizScaling}}})
	}
	if opts.Rise != 0 {
		*ops = append(*ops, semantic.Operation{Operator: "Ts", Operands: []semantic.Operand{semantic.NumberOperand{Value: opts.Rise}}})
	}
	if opts.RenderMode != contentstream.TextFill {
		*ops = append(*ops, semantic.Operation{Operator: "Tr", Operands: []semantic.Operand{semantic.NumberOperand{Value: float64(opts.RenderMode)}}})
	}
	*ops = append(*ops, semantic.Operation{
		Operator: "Tm",
		Operands: []semantic.Operand{
			semantic.NumberOperand{Value: 1},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 1},
			semantic.NumberOperand{Value: x},
			semantic.NumberOperand{Value: y},
		},
	})
	if !isZeroColor(opts.Color) {
		p.appendColorOp(ops, opts.Color, false)
		if isStrokeMode(opts.RenderMode) {
			p.appendColorOp(ops, opts.Color, true)
		}
	}
	*ops = append(*ops, semantic.Operation{
		Operator: "Tj",
		Operands: []semantic.Operand{semantic.StringOperand{Value: []byte(text)}},
	})
	*ops = append(*ops, semantic.Operation{Operator: "ET"})
	return p
}

func (p *pageBuilderImpl) DrawPath(path *contentstream.Path, opts PathOptions) PageBuilder {
	if path == nil {
		return p
	}
	ops := p.ensureContentOps()
	*ops = append(*ops, semantic.Operation{Operator: "q"})
	p.applyPathState(ops, opts)
	p.appendPathOps(ops, path)
	*ops = append(*ops, semantic.Operation{Operator: paintOperator(opts.Fill, opts.Stroke)})
	*ops = append(*ops, semantic.Operation{Operator: "Q"})
	return p
}

func (p *pageBuilderImpl) DrawImage(img *semantic.Image, x, y, width, height float64, opts ImageOptions) PageBuilder {
	if img == nil {
		return p
	}
	res := p.ensureResources()
	if res.XObjects == nil {
		res.XObjects = make(map[string]semantic.XObject)
	}

	name := p.parent.imageName(img)
	if _, exists := res.XObjects[name]; !exists {
		xobj := semantic.XObject(*img)
		xobj.Subtype = "Image"
		if opts.Interpolate {
			xobj.Interpolate = true
		}
		if opts.SMask != nil {
			xobj.SMask = opts.SMask
		}
		res.XObjects[name] = xobj
	}
	w := width
	if w == 0 {
		w = float64(img.Width)
	}
	h := height
	if h == 0 {
		h = float64(img.Height)
	}

	ops := p.ensureContentOps()
	*ops = append(*ops, semantic.Operation{Operator: "q"})
	*ops = append(*ops, semantic.Operation{
		Operator: "cm",
		Operands: []semantic.Operand{
			semantic.NumberOperand{Value: w},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: 0},
			semantic.NumberOperand{Value: h},
			semantic.NumberOperand{Value: x},
			semantic.NumberOperand{Value: y},
		},
	})
	*ops = append(*ops, semantic.Operation{
		Operator: "Do",
		Operands: []semantic.Operand{semantic.NameOperand{Value: name}},
	})
	*ops = append(*ops, semantic.Operation{Operator: "Q"})
	return p
}

func (p *pageBuilderImpl) DrawRectangle(x, y, width, height float64, opts RectOptions) PageBuilder {
	po := opts
	if !po.Stroke && !po.Fill {
		po.Stroke = true
	}
	ops := p.ensureContentOps()
	*ops = append(*ops, semantic.Operation{Operator: "q"})
	p.applyPathState(ops, po)
	*ops = append(*ops, semantic.Operation{
		Operator: "re",
		Operands: []semantic.Operand{
			semantic.NumberOperand{Value: x},
			semantic.NumberOperand{Value: y},
			semantic.NumberOperand{Value: width},
			semantic.NumberOperand{Value: height},
		},
	})
	*ops = append(*ops, semantic.Operation{Operator: paintOperator(po.Fill, po.Stroke)})
	*ops = append(*ops, semantic.Operation{Operator: "Q"})
	return p
}

func (p *pageBuilderImpl) DrawLine(x1, y1, x2, y2 float64, opts LineOptions) PageBuilder {
	ops := p.ensureContentOps()
	*ops = append(*ops, semantic.Operation{Operator: "q"})
	po := PathOptions{
		StrokeColor: opts.StrokeColor,
		LineWidth:   opts.LineWidth,
		LineCap:     opts.LineCap,
		DashPattern: opts.DashPattern,
		DashPhase:   opts.DashPhase,
		Stroke:      true,
	}
	p.applyPathState(ops, po)
	*ops = append(*ops, semantic.Operation{
		Operator: "m",
		Operands: []semantic.Operand{semantic.NumberOperand{Value: x1}, semantic.NumberOperand{Value: y1}},
	})
	*ops = append(*ops, semantic.Operation{
		Operator: "l",
		Operands: []semantic.Operand{semantic.NumberOperand{Value: x2}, semantic.NumberOperand{Value: y2}},
	})
	*ops = append(*ops, semantic.Operation{Operator: "S"})
	*ops = append(*ops, semantic.Operation{Operator: "Q"})
	return p
}

func (p *pageBuilderImpl) AddAnnotation(ann *semantic.Annotation) PageBuilder {
	if ann != nil {
		p.page.Annotations = append(p.page.Annotations, *ann)
	}
	return p
}

func (p *pageBuilderImpl) SetMediaBox(box semantic.Rectangle) PageBuilder {
	p.page.MediaBox = box
	return p
}

func (p *pageBuilderImpl) SetCropBox(box semantic.Rectangle) PageBuilder {
	p.page.CropBox = box
	return p
}

func (p *pageBuilderImpl) SetRotation(degrees int) PageBuilder {
	p.page.Rotate = normalizeRotation(degrees)
	return p
}

func (p *pageBuilderImpl) Finish() PDFBuilder { return p.parent }

func (b *builderImpl) fontForName(name string) (*semantic.Font, string) {
	if name == "" {
		name = b.defaultFont
		if name == "" {
			name = defaultFontResource
		}
	}
	if b.fonts == nil {
		b.fonts = make(map[string]*semantic.Font)
	}
	if f, ok := b.fonts[name]; ok {
		return f, name
	}
	font := &semantic.Font{BaseFont: defaultBaseFont}
	b.fonts[name] = font
	return font, name
}

func (b *builderImpl) imageName(img *semantic.Image) string {
	if b.xobjectNames == nil {
		b.xobjectNames = make(map[*semantic.Image]string)
	}
	if name, ok := b.xobjectNames[img]; ok {
		return name
	}
	b.xobjectCount++
	name := fmt.Sprintf("Im%d", b.xobjectCount)
	b.xobjectNames[img] = name
	return name
}

func (p *pageBuilderImpl) ensureResources() *semantic.Resources {
	if p.page.Resources == nil {
		p.page.Resources = &semantic.Resources{}
	}
	if p.page.Resources.Fonts == nil {
		p.page.Resources.Fonts = make(map[string]*semantic.Font)
	}
	if p.page.Resources.ExtGStates == nil {
		p.page.Resources.ExtGStates = make(map[string]semantic.ExtGState)
	}
	if p.page.Resources.ColorSpaces == nil {
		p.page.Resources.ColorSpaces = make(map[string]semantic.ColorSpace)
	}
	if p.page.Resources.XObjects == nil {
		p.page.Resources.XObjects = make(map[string]semantic.XObject)
	}
	if p.page.Resources.Patterns == nil {
		p.page.Resources.Patterns = make(map[string]semantic.Pattern)
	}
	if p.page.Resources.Shadings == nil {
		p.page.Resources.Shadings = make(map[string]semantic.Shading)
	}
	return p.page.Resources
}

func (p *pageBuilderImpl) ensureContentOps() *[]semantic.Operation {
	if len(p.page.Contents) == 0 {
		p.page.Contents = append(p.page.Contents, semantic.ContentStream{})
	}
	return &p.page.Contents[0].Operations
}

func (b *builderImpl) convertOutline(out Outline, pageIndex map[*semantic.Page]int) semantic.OutlineItem {
	idx := out.PageIndex
	if out.Page != nil {
		if resolved, ok := pageIndex[out.Page]; ok {
			idx = resolved
		}
	}
	item := semantic.OutlineItem{Title: out.Title, PageIndex: idx}
	if out.X != nil || out.Y != nil || out.Zoom != nil {
		item.Dest = &semantic.OutlineDestination{X: out.X, Y: out.Y, Zoom: out.Zoom}
	}
	if len(out.Children) > 0 {
		item.Children = make([]semantic.OutlineItem, 0, len(out.Children))
		for _, child := range out.Children {
			item.Children = append(item.Children, b.convertOutline(child, pageIndex))
		}
	}
	return item
}

func (p *pageBuilderImpl) appendColorOp(ops *[]semantic.Operation, c Color, stroking bool) {
	if isZeroColor(c) {
		return
	}
	op := "rg"
	if stroking {
		op = "RG"
	}
	*ops = append(*ops, semantic.Operation{
		Operator: op,
		Operands: colorOperands(c),
	})
}

func (p *pageBuilderImpl) applyPathState(ops *[]semantic.Operation, opts PathOptions) {
	if opts.Fill {
		p.appendColorOp(ops, opts.FillColor, false)
	}
	if opts.Stroke || (!opts.Fill && !opts.Stroke) {
		p.appendColorOp(ops, opts.StrokeColor, true)
		if opts.LineWidth > 0 {
			*ops = append(*ops, semantic.Operation{Operator: "w", Operands: []semantic.Operand{semantic.NumberOperand{Value: opts.LineWidth}}})
		}
		if opts.LineCap != 0 {
			*ops = append(*ops, semantic.Operation{Operator: "J", Operands: []semantic.Operand{semantic.NumberOperand{Value: float64(opts.LineCap)}}})
		}
		if opts.LineJoin != 0 {
			*ops = append(*ops, semantic.Operation{Operator: "j", Operands: []semantic.Operand{semantic.NumberOperand{Value: float64(opts.LineJoin)}}})
		}
		if len(opts.DashPattern) > 0 {
			vals := make([]semantic.Operand, 0, len(opts.DashPattern))
			for _, v := range opts.DashPattern {
				vals = append(vals, semantic.NumberOperand{Value: v})
			}
			*ops = append(*ops, semantic.Operation{
				Operator: "d",
				Operands: []semantic.Operand{
					semantic.ArrayOperand{Values: vals},
					semantic.NumberOperand{Value: opts.DashPhase},
				},
			})
		}
	}
}

func (p *pageBuilderImpl) appendPathOps(ops *[]semantic.Operation, path *contentstream.Path) {
	for _, sp := range path.Subpaths {
		for _, point := range sp.Points {
			switch point.Type {
			case contentstream.PathMoveTo:
				*ops = append(*ops, semantic.Operation{
					Operator: "m",
					Operands: []semantic.Operand{semantic.NumberOperand{Value: point.X}, semantic.NumberOperand{Value: point.Y}},
				})
			case contentstream.PathLineTo:
				*ops = append(*ops, semantic.Operation{
					Operator: "l",
					Operands: []semantic.Operand{semantic.NumberOperand{Value: point.X}, semantic.NumberOperand{Value: point.Y}},
				})
			case contentstream.PathCurveTo:
				*ops = append(*ops, semantic.Operation{
					Operator: "c",
					Operands: []semantic.Operand{
						semantic.NumberOperand{Value: point.Control1X},
						semantic.NumberOperand{Value: point.Control1Y},
						semantic.NumberOperand{Value: point.Control2X},
						semantic.NumberOperand{Value: point.Control2Y},
						semantic.NumberOperand{Value: point.X},
						semantic.NumberOperand{Value: point.Y},
					},
				})
			case contentstream.PathClose:
				*ops = append(*ops, semantic.Operation{Operator: "h"})
			}
		}
		if sp.Closed {
			*ops = append(*ops, semantic.Operation{Operator: "h"})
		}
	}
}

func isZeroColor(c Color) bool {
	return c.R == 0 && c.G == 0 && c.B == 0 && c.A == 0
}

func colorOperands(c Color) []semantic.Operand {
	return []semantic.Operand{
		semantic.NumberOperand{Value: c.R},
		semantic.NumberOperand{Value: c.G},
		semantic.NumberOperand{Value: c.B},
	}
}

func paintOperator(fill, stroke bool) string {
	switch {
	case fill && stroke:
		return "B"
	case fill:
		return "f"
	default:
		return "S"
	}
}

func isStrokeMode(mode contentstream.TextRenderMode) bool {
	return mode == contentstream.TextStroke ||
		mode == contentstream.TextFillStroke ||
		mode == contentstream.TextStrokeClip ||
		mode == contentstream.TextFillStrokeClip
}

func normalizeRotation(deg int) int {
	deg %= 360
	if deg < 0 {
		deg += 360
	}
	return deg
}
