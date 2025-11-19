package builder

import (
	"fmt"

	"pdflib/contentstream"
	"pdflib/fonts"
	"pdflib/ir/raw"
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
	SetEncryption(ownerPassword, userPassword string, perms raw.Permissions, encryptMetadata bool) PDFBuilder
	RegisterFont(name string, font *semantic.Font) PDFBuilder
	RegisterTrueTypeFont(name string, data []byte) PDFBuilder
	AddEmbeddedFile(file semantic.EmbeddedFile) PDFBuilder
	Build() (*semantic.Document, error)
}

// PageBuilder provides a fluent API for page construction.
type PageBuilder interface {
	DrawText(text string, x, y float64, opts TextOptions) PageBuilder
	DrawPath(path *contentstream.Path, opts PathOptions) PageBuilder
	DrawImage(img *semantic.Image, x, y, width, height float64, opts ImageOptions) PageBuilder
	DrawRectangle(x, y, width, height float64, opts RectOptions) PageBuilder
	DrawLine(x1, y1, x2, y2 float64, opts LineOptions) PageBuilder
	DrawTable(table Table, opts TableOptions) PageBuilder
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
	Tag          string
	MCID         *int
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

// Table defines a matrix of cells to draw.
type Table struct {
	Columns    []float64
	Rows       []TableRow
	HeaderRows int
}

// TableRow wraps a slice of cells.
type TableRow struct {
	Cells []TableCell
}

// TableCell configures individual table cell rendering.
type TableCell struct {
	Text            string
	Font            string
	FontSize        float64
	Padding         *CellPadding
	BackgroundColor Color
	TextColor       Color
	BorderColor     Color
	BorderWidth     float64
	ColSpan         int
	HAlign          HAlign
	VAlign          VAlign
	Tag             string
}

// CellPadding defines per-side padding.
type CellPadding struct {
	Top, Right, Bottom, Left float64
}

// TableOptions configures table rendering.
type TableOptions struct {
	X             float64
	Y             float64
	RowHeight     float64
	CellPadding   float64
	BorderColor   Color
	BorderWidth   float64
	HeaderFill    Color
	RepeatHeaders bool
	Tagged        bool
	BottomMargin  float64
	DefaultFont   string
	DefaultSize   float64
	TopMargin     float64
	LeftMargin    float64
}

// HAlign controls horizontal text alignment within a cell.
type HAlign string

const (
	HAlignLeft   HAlign = "left"
	HAlignCenter HAlign = "center"
	HAlignRight  HAlign = "right"
)

// VAlign controls vertical text alignment within a cell.
type VAlign string

const (
	VAlignTop    VAlign = "top"
	VAlignMiddle VAlign = "middle"
	VAlignBottom VAlign = "bottom"
)

type fontResource struct {
	font      *semantic.Font
	runeToCID map[rune]int
}

type builderImpl struct {
	pages         []*semantic.Page
	info          *semantic.DocumentInfo
	metadata      []byte
	lang          string
	marked        bool
	pageLabels    map[int]string
	outlines      []Outline
	ownerPassword string
	userPassword  string
	permissions   raw.Permissions
	encrypted     bool
	encryptMeta   bool
	fonts         map[string]fontResource
	defaultFont   string
	xobjectCount  int
	xobjectNames  map[*semantic.Image]string
	fontErr       error
	structTree    *semantic.StructureTree
	mcidCounters  map[*semantic.Page]int
	embeddedFiles []semantic.EmbeddedFile
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
	p := &semantic.Page{MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: w, URY: h}}
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

func (b *builderImpl) SetEncryption(ownerPassword, userPassword string, perms raw.Permissions, encryptMetadata bool) PDFBuilder {
	b.ownerPassword = ownerPassword
	b.userPassword = userPassword
	b.permissions = perms
	b.encrypted = true
	b.encryptMeta = encryptMetadata
	return b
}

func (b *builderImpl) RegisterFont(name string, font *semantic.Font) PDFBuilder {
	return b.addFont(name, font)
}

func (b *builderImpl) RegisterTrueTypeFont(name string, data []byte) PDFBuilder {
	font, err := fonts.LoadTrueType(name, data)
	if err != nil {
		b.fontErr = err
		return b
	}
	return b.addFont(name, font)
}

func (b *builderImpl) AddEmbeddedFile(file semantic.EmbeddedFile) PDFBuilder {
	if file.Name == "" || len(file.Data) == 0 {
		return b
	}
	if file.Relationship == "" {
		file.Relationship = "Unspecified"
	}
	copyFile := file
	if len(file.Data) > 0 {
		copyFile.Data = append([]byte(nil), file.Data...)
	}
	b.embeddedFiles = append(b.embeddedFiles, copyFile)
	return b
}

func (b *builderImpl) addFont(name string, font *semantic.Font) PDFBuilder {
	if b.fonts == nil {
		b.fonts = make(map[string]fontResource)
	}
	if font == nil {
		return b
	}
	b.fonts[name] = fontResource{font: font, runeToCID: runeToCID(font)}
	if b.defaultFont == "" {
		b.defaultFont = name
	}
	return b
}

func (b *builderImpl) Build() (*semantic.Document, error) {
	if b.fontErr != nil {
		return nil, b.fontErr
	}
	pageIndexByPtr := make(map[*semantic.Page]int, len(b.pages))
	for i, p := range b.pages {
		p.Index = i
		pageIndexByPtr[p] = i
	}
	doc := &semantic.Document{
		Pages:  b.pages,
		Info:   b.info,
		Lang:   b.lang,
		Marked: b.marked || b.structTree != nil,
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
	if b.structTree != nil {
		doc.StructTree = b.structTree
	}
	if b.encrypted {
		doc.OwnerPassword = b.ownerPassword
		doc.UserPassword = b.userPassword
		doc.Permissions = b.permissions
		doc.Encrypted = true
		doc.MetadataEncrypted = b.encryptMeta
	}
	if len(b.embeddedFiles) > 0 {
		doc.EmbeddedFiles = make([]semantic.EmbeddedFile, len(b.embeddedFiles))
		for i, ef := range b.embeddedFiles {
			copyEf := ef
			if len(ef.Data) > 0 {
				copyEf.Data = append([]byte(nil), ef.Data...)
			}
			doc.EmbeddedFiles[i] = copyEf
		}
	}
	return doc, nil
}

func (p *pageBuilderImpl) DrawText(text string, x, y float64, opts TextOptions) PageBuilder {
	ops := p.ensureContentOps()
	res := p.ensureResources()

	font, fontName, cmap := p.parent.fontForName(opts.Font)
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

	tagged := false
	if opts.MCID != nil {
		tag := opts.Tag
		if tag == "" {
			tag = "Span"
		}
		tagged = true
		*ops = append(*ops, semantic.Operation{
			Operator: "BDC",
			Operands: []semantic.Operand{
				semantic.NameOperand{Value: tag},
				semantic.DictOperand{Values: map[string]semantic.Operand{
					"MCID": semantic.NumberOperand{Value: float64(*opts.MCID)},
				}},
			},
		})
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
		Operands: []semantic.Operand{semantic.StringOperand{Value: encodeText(text, font, cmap)}},
	})
	*ops = append(*ops, semantic.Operation{Operator: "ET"})
	if tagged {
		*ops = append(*ops, semantic.Operation{Operator: "EMC"})
	}
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

func (p *pageBuilderImpl) DrawTable(table Table, opts TableOptions) PageBuilder {
	if len(table.Columns) == 0 || len(table.Rows) == 0 {
		return p
	}
	cur := p
	borderColor := opts.BorderColor
	if isZeroColor(borderColor) {
		borderColor = Color{}
	}
	borderWidth := opts.BorderWidth
	if borderWidth == 0 {
		borderWidth = 0.5
	}
	cellPad := opts.CellPadding
	if cellPad == 0 {
		cellPad = 4
	}
	defaultSize := opts.DefaultSize
	if defaultSize == 0 {
		defaultSize = 12
	}
	if opts.X == 0 && opts.LeftMargin > 0 {
		opts.X = opts.LeftMargin
	}
	repeatHeaders := opts.RepeatHeaders
	headerCount := table.HeaderRows
	if headerCount > len(table.Rows) {
		headerCount = len(table.Rows)
	}
	if headerCount > 0 && !opts.RepeatHeaders {
		repeatHeaders = true
	}
	bottomMargin := opts.BottomMargin
	pageHeight := cur.page.MediaBox.URY - cur.page.MediaBox.LLY
	if opts.Y == 0 {
		opts.Y = cur.page.MediaBox.URY
		if opts.TopMargin > 0 {
			opts.Y -= opts.TopMargin
		}
	}
	rowHeights := make([]float64, len(table.Rows))
	resolvePadding := func(pad *CellPadding) CellPadding {
		if pad != nil {
			return *pad
		}
		return CellPadding{Top: cellPad, Right: cellPad, Bottom: cellPad, Left: cellPad}
	}
	for i, row := range table.Rows {
		var h float64
		for _, cell := range row.Cells {
			size := cell.FontSize
			if size == 0 {
				size = defaultSize
			}
			pad := resolvePadding(cell.Padding)
			cellH := size*1.2 + pad.Top + pad.Bottom
			if cellH > h {
				h = cellH
			}
		}
		if opts.RowHeight > h {
			h = opts.RowHeight
		}
		if h == 0 {
			h = defaultSize*1.2 + 2*cellPad
		}
		rowHeights[i] = h
	}
	spanWidth := func(startCol, span int) float64 {
		if span <= 0 {
			span = 1
		}
		end := startCol + span
		if end > len(table.Columns) {
			end = len(table.Columns)
		}
		width := 0.0
		for i := startCol; i < end; i++ {
			width += table.Columns[i]
		}
		return width
	}
	pageIndex := func(pb *pageBuilderImpl) int {
		for i, page := range pb.parent.pages {
			if page == pb.page {
				return i
			}
		}
		return pb.page.Index
	}
	clonePage := func() *pageBuilderImpl {
		next := pbFromBuilder(cur.parent.NewPage(cur.page.MediaBox.URX, cur.page.MediaBox.URY))
		if next == nil {
			return cur
		}
		next.page.CropBox = cur.page.CropBox
		next.page.TrimBox = cur.page.TrimBox
		next.page.BleedBox = cur.page.BleedBox
		next.page.ArtBox = cur.page.ArtBox
		next.page.Rotate = cur.page.Rotate
		next.page.UserUnit = cur.page.UserUnit
		return next
	}

	curY := opts.Y
	var tableElem *semantic.StructureElement
	if opts.Tagged {
		cur.parent.marked = true
		tableElem = &semantic.StructureElement{Type: "Table"}
		cur.parent.ensureStructTree().Kids = append(cur.parent.ensureStructTree().Kids, tableElem)
	}
	var renderRow func(row TableRow, height float64, isHeader bool, allowBreak bool)
	var renderHeaders func()
	renderHeaders = func() {
		if !repeatHeaders || headerCount == 0 {
			return
		}
		for i := 0; i < headerCount; i++ {
			renderRow(table.Rows[i], rowHeights[i], true, false)
		}
	}
	renderRow = func(row TableRow, height float64, isHeader bool, allowBreak bool) {
		if allowBreak && curY-height < bottomMargin {
			cur = clonePage()
			curY = opts.Y
			renderHeaders()
			if curY-height < bottomMargin && pageHeight > 0 {
				curY = bottomMargin + height
			}
		}
		rowElem := (*semantic.StructureElement)(nil)
		pageIdx := pageIndex(cur)
		if tableElem != nil {
			pageCopy := pageIdx
			rowElem = &semantic.StructureElement{Type: "TR", PageIndex: &pageCopy}
			tableElem.Kids = append(tableElem.Kids, semantic.StructureItem{Element: rowElem})
		}
		x := opts.X
		for col := 0; col < len(table.Columns) && col < len(row.Cells); col++ {
			cell := row.Cells[col]
			span := cell.ColSpan
			if span <= 0 {
				span = 1
			}
			width := spanWidth(col, span)
			pad := resolvePadding(cell.Padding)
			fill := cell.BackgroundColor
			if isHeader && isZeroColor(fill) && !isZeroColor(opts.HeaderFill) {
				fill = opts.HeaderFill
			}
			if !isZeroColor(fill) {
				cur.DrawRectangle(x, curY-height, width, height, RectOptions{
					Fill:      true,
					FillColor: fill,
				})
			}
			bw := cell.BorderWidth
			if bw == 0 {
				bw = borderWidth
			}
			bc := cell.BorderColor
			if isZeroColor(bc) {
				bc = borderColor
			}
			if bw > 0 {
				cur.DrawRectangle(x, curY-height, width, height, RectOptions{
					Stroke:      true,
					StrokeColor: bc,
					LineWidth:   bw,
				})
			}
			tag := cell.Tag
			if tag == "" {
				if isHeader {
					tag = "TH"
				} else {
					tag = "TD"
				}
			}
			mcidPtr := (*int)(nil)
			if tableElem != nil {
				mcid := cur.parent.nextMCID(cur.page)
				mcidPtr = &mcid
			}
			textColor := cell.TextColor
			if isZeroColor(textColor) {
				textColor = Color{}
			}
			size := cell.FontSize
			if size == 0 {
				size = defaultSize
			}
			hAlign := cell.HAlign
			if hAlign == "" {
				hAlign = HAlignLeft
			}
			vAlign := cell.VAlign
			if vAlign == "" {
				vAlign = VAlignTop
			}
			textX := x + pad.Left
			textY := curY - pad.Top - size
			if hAlign != HAlignLeft {
				txtWidth := measureTextWidth(cur.parent, cell.Text, size, cell.Font)
				available := width - pad.Left - pad.Right
				switch hAlign {
				case HAlignCenter:
					textX = x + pad.Left + (available-txtWidth)/2
				case HAlignRight:
					textX = x + width - pad.Right - txtWidth
				}
			}
			if vAlign != VAlignTop {
				switch vAlign {
				case VAlignMiddle:
					textY = curY - (height / 2) - (size / 2)
				case VAlignBottom:
					textY = curY - height + pad.Bottom
				}
			}
			cur.DrawText(cell.Text, textX, textY, TextOptions{
				Font:     cell.Font,
				FontSize: size,
				Color:    textColor,
				Tag:      tag,
				MCID:     mcidPtr,
			})
			if rowElem != nil && mcidPtr != nil {
				cellPage := pageIdx
				cellElem := &semantic.StructureElement{Type: tag, PageIndex: &cellPage}
				cellElem.Kids = append(cellElem.Kids, semantic.StructureItem{
					MCID:      mcidPtr,
					PageIndex: &cellPage,
				})
				rowElem.Kids = append(rowElem.Kids, semantic.StructureItem{Element: cellElem})
			}
			x += width
			col += span - 1
		}
		curY -= height
	}

	for i, row := range table.Rows {
		renderRow(row, rowHeights[i], i < headerCount, true)
	}
	return cur
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

func (b *builderImpl) fontForName(name string) (*semantic.Font, string, map[rune]int) {
	if name == "" {
		name = b.defaultFont
		if name == "" {
			name = defaultFontResource
		}
	}
	if b.fonts == nil {
		b.fonts = make(map[string]fontResource)
	}
	if f, ok := b.fonts[name]; ok {
		return f.font, name, f.runeToCID
	}
	font := &semantic.Font{BaseFont: defaultBaseFont}
	res := fontResource{font: font}
	b.fonts[name] = res
	return font, name, res.runeToCID
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

func (b *builderImpl) ensureStructTree() *semantic.StructureTree {
	if b.structTree == nil {
		b.structTree = &semantic.StructureTree{}
	}
	return b.structTree
}

func (b *builderImpl) nextMCID(page *semantic.Page) int {
	if b.mcidCounters == nil {
		b.mcidCounters = make(map[*semantic.Page]int)
	}
	id := b.mcidCounters[page]
	b.mcidCounters[page] = id + 1
	return id
}

func runeToCID(font *semantic.Font) map[rune]int {
	if font == nil || len(font.ToUnicode) == 0 {
		return nil
	}
	m := make(map[rune]int)
	for cid, runes := range font.ToUnicode {
		for _, r := range runes {
			if _, exists := m[r]; !exists {
				m[r] = cid
			}
		}
	}
	return m
}

func encodeText(text string, font *semantic.Font, cmap map[rune]int) []byte {
	if font != nil && font.Subtype == "Type0" && font.Encoding == "Identity-H" && len(cmap) > 0 {
		buf := make([]byte, 0, len(text)*2)
		for _, r := range text {
			cid, ok := cmap[r]
			if !ok {
				cid = 0
			}
			buf = append(buf, byte(cid>>8), byte(cid))
		}
		return buf
	}
	return []byte(text)
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

func pbFromBuilder(pb PageBuilder) *pageBuilderImpl {
	if v, ok := pb.(*pageBuilderImpl); ok {
		return v
	}
	return nil
}

// measureTextWidth approximates text width in user units.
func measureTextWidth(b *builderImpl, text string, fontSize float64, fontName string) float64 {
	font, _, _ := b.fontForName(fontName)
	if font == nil || len(font.Widths) == 0 {
		if fontSize == 0 {
			fontSize = 12
		}
		return float64(len(text)) * fontSize * 0.5
	}
	widthSum := 0.0
	for _, r := range text {
		code := int(r)
		if font.Subtype == "Type0" && font.Encoding == "Identity-H" {
			// For CID fonts, widths are keyed by CID, which we derive from ToUnicode.
			if cmap := b.fonts[fontName].runeToCID; cmap != nil {
				if cid, ok := cmap[r]; ok {
					code = cid
				}
			}
		}
		if w, ok := font.Widths[code]; ok {
			widthSum += float64(w)
		} else {
			widthSum += 500 // default width in glyph space
		}
	}
	if fontSize == 0 {
		fontSize = 12
	}
	return (widthSum / 1000) * fontSize
}
