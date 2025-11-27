package layout

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// RenderHTML renders an HTML string to the PDF.
func (e *Engine) RenderHTML(source string) error {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return err
	}
	e.walkHTML(doc)
	if e.currentPage != nil {
		e.currentPage.Finish()
	}
	return nil
}

func (e *Engine) walkHTML(n *html.Node) {
	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			e.renderHTMLHeader(n)
			return // Don't traverse children normally, renderHeader handles text
		case atom.P:
			e.renderHTMLParagraph(n)
			return
		case atom.Li:
			e.renderHTMLListItem(n)
			return
		case atom.Blockquote:
			e.renderHTMLBlockquote(n)
			return
		case atom.Pre:
			e.renderHTMLPre(n)
			return
		case atom.Hr:
			e.renderHTMLHr(n)
			return
		case atom.Img:
			e.renderHTMLImage(n)
			return
		case atom.Table:
			e.renderHTMLTable(n)
			return
		case atom.Input:
			e.renderHTMLInput(n)
			return
		case atom.Textarea:
			e.renderHTMLTextarea(n)
			return
		case atom.Select:
			e.renderHTMLSelect(n)
			return
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.walkHTML(c)
	}
}

func (e *Engine) renderHTMLHeader(n *html.Node) {
	level := 1
	switch n.DataAtom {
	case atom.H1:
		level = 1
	case atom.H2:
		level = 2
	case atom.H3:
		level = 3
	default:
		level = 4
	}

	fontSize := e.DefaultFontSize * 2.0
	if level == 2 {
		fontSize = e.DefaultFontSize * 1.5
	} else if level >= 3 {
		fontSize = e.DefaultFontSize * 1.25
	}

	e.ensurePage()
	lineHeight := fontSize * e.LineHeight

	spans := e.extractSpans(n)
	for i := range spans {
		spans[i].FontSize = fontSize
		// Headers are typically bold
		spans[i].Font = e.resolveFont(e.DefaultFont, true, strings.Contains(spans[i].Font, "Oblique") || strings.Contains(spans[i].Font, "Italic"))
	}

	e.renderSpans(spans, e.cursorX, lineHeight)
}

func (e *Engine) renderHTMLParagraph(n *html.Node) {
	spans := e.extractSpans(n)
	e.ensurePage()
	// Use default font size for line height calculation
	lineHeight := e.DefaultFontSize * e.LineHeight
	e.renderSpans(spans, e.cursorX, lineHeight)
	e.renderParagraphSpacing()
}

func (e *Engine) renderHTMLListItem(n *html.Node) {
	spans := e.extractSpans(n)
	e.ensurePage()

	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight

	marker := "•"
	if n.Parent != nil && n.Parent.DataAtom == atom.Ol {
		index := 1
		for c := n.PrevSibling; c != nil; c = c.PrevSibling {
			if c.Type == html.ElementNode && c.DataAtom == atom.Li {
				index++
			}
		}
		marker = fmt.Sprintf("%d.", index)
	}

	e.checkPageBreak(lineHeight)
	e.currentPage.DrawText(marker, e.cursorX, e.cursorY-fontSize, builder.TextOptions{
		Font:     e.DefaultFont,
		FontSize: fontSize,
	})

	// Indent text
	indent := 15.0
	if len(marker) > 2 {
		indent += float64(len(marker)-1) * 5
	}
	e.renderSpans(spans, e.cursorX+indent, lineHeight)
}

func (e *Engine) renderHTMLBlockquote(n *html.Node) {
	oldLeft := e.Margins.Left
	e.Margins.Left += 20
	e.cursorX = e.Margins.Left

	// Traverse children to render paragraphs inside blockquote
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.walkHTML(c)
	}

	e.Margins.Left = oldLeft
	e.cursorX = e.Margins.Left
	e.renderParagraphSpacing()
}

func (e *Engine) renderHTMLPre(n *html.Node) {
	text := extractText(n)
	lines := strings.Split(text, "\n")

	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight

	// Indent code blocks
	indent := 20.0

	for _, line := range lines {
		// Trim newline at end of line if present (though Split handles most)
		line = strings.TrimRight(line, "\r")

		e.checkPageBreak(lineHeight)
		e.currentPage.DrawText(line, e.cursorX+indent, e.cursorY-fontSize, builder.TextOptions{
			Font:     "Courier", // Ideally monospace
			FontSize: fontSize,
		})
		e.cursorY -= lineHeight
	}
	e.renderParagraphSpacing()
}

func (e *Engine) renderHTMLHr(n *html.Node) {
	e.ensurePage()
	e.cursorY -= e.DefaultFontSize

	e.checkPageBreak(2)
	e.currentPage.DrawLine(e.Margins.Left, e.cursorY, e.pageWidth-e.Margins.Right, e.cursorY, builder.LineOptions{
		LineWidth:   1,
		StrokeColor: builder.Color{R: 0.5, G: 0.5, B: 0.5},
	})

	e.cursorY -= e.DefaultFontSize
}

func (e *Engine) renderHTMLInput(n *html.Node) {
	e.ensurePage()
	fontSize := e.DefaultFontSize
	height := fontSize * 1.5
	width := 100.0 // Default width

	if val := getAttr(n, "width"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			width = v
		}
	}
	if val := getAttr(n, "height"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			height = v
		}
	}

	name := getAttr(n, "name")
	val := getAttr(n, "value")
	typ := getAttr(n, "type")
	if typ == "" {
		typ = "text"
	}

	e.checkPageBreak(height)
	rect := semantic.Rectangle{
		LLX: e.cursorX,
		LLY: e.cursorY - height,
		URX: e.cursorX + width,
		URY: e.cursorY,
	}

	var field semantic.FormField
	base := semantic.BaseFormField{
		Name:  name,
		Rect:  rect,
		Color: []float64{0.9, 0.9, 0.9}, // Light gray background
	}

	switch typ {
	case "checkbox":
		width = height // Square
		rect.URX = e.cursorX + width
		base.Rect = rect
		checked := hasAttr(n, "checked")
		bf := &semantic.ButtonFormField{
			BaseFormField: base,
			IsCheck:       true,
			Checked:       checked,
			OnState:       "Yes",
		}
		if checked {
			bf.AppearanceState = "Yes"
		} else {
			bf.AppearanceState = "Off"
		}
		field = bf
	case "radio":
		width = height // Square
		rect.URX = e.cursorX + width
		base.Rect = rect
		checked := hasAttr(n, "checked")
		bf := &semantic.ButtonFormField{
			BaseFormField: base,
			IsRadio:       true,
			Checked:       checked,
			OnState:       val,
		}
		if checked {
			bf.AppearanceState = val
		} else {
			bf.AppearanceState = "Off"
		}
		field = bf
	case "submit":
		base.Color = []float64{0.8, 0.8, 0.8}
		field = &semantic.ButtonFormField{
			BaseFormField: base,
			IsPush:        true,
		}
		// Draw label
		e.currentPage.DrawText(val, e.cursorX+5, e.cursorY-fontSize-2, builder.TextOptions{
			Font:     e.DefaultFont,
			FontSize: fontSize,
		})
	default: // text, password, etc.
		field = &semantic.TextFormField{
			BaseFormField: base,
			Value:         val,
		}
	}

	e.currentPage.AddFormField(field)

	// Draw visual box
	e.currentPage.DrawRectangle(rect.LLX, rect.LLY, rect.URX-rect.LLX, rect.URY-rect.LLY, builder.RectOptions{
		Fill:      true,
		FillColor: builder.Color{R: base.Color[0], G: base.Color[1], B: base.Color[2]},
		Stroke:    true,
		LineWidth: 1,
	})

	e.cursorY -= height + 5 // Spacing
}

func (e *Engine) renderHTMLTextarea(n *html.Node) {
	e.ensurePage()
	fontSize := e.DefaultFontSize
	height := fontSize * 4 // Multiline
	width := 200.0

	if val := getAttr(n, "width"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			width = v
		}
	}
	if val := getAttr(n, "height"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			height = v
		}
	}

	name := getAttr(n, "name")
	val := extractText(n)

	e.checkPageBreak(height)
	rect := semantic.Rectangle{
		LLX: e.cursorX,
		LLY: e.cursorY - height,
		URX: e.cursorX + width,
		URY: e.cursorY,
	}

	field := &semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name:  name,
			Rect:  rect,
			Color: []float64{0.9, 0.9, 0.9},
			Flags: 4096, // Multiline
		},
		Value: val,
	}

	e.currentPage.AddFormField(field)

	e.currentPage.DrawRectangle(rect.LLX, rect.LLY, rect.URX-rect.LLX, rect.URY-rect.LLY, builder.RectOptions{
		Fill:      true,
		FillColor: builder.Color{R: 0.9, G: 0.9, B: 0.9},
		Stroke:    true,
		LineWidth: 1,
	})

	e.cursorY -= height + 5
}

func (e *Engine) renderHTMLSelect(n *html.Node) {
	e.ensurePage()
	fontSize := e.DefaultFontSize
	height := fontSize * 1.5
	width := 120.0

	if val := getAttr(n, "width"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			width = v
		}
	}
	if val := getAttr(n, "height"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			height = v
		}
	}

	name := getAttr(n, "name")

	var options []string
	var selected []string

	// Parse options
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == atom.Option {
			val := getAttr(c, "value")
			if val == "" {
				val = extractText(c)
			}
			options = append(options, val)
			if hasAttr(c, "selected") {
				selected = append(selected, val)
			}
		}
	}

	e.checkPageBreak(height)
	rect := semantic.Rectangle{
		LLX: e.cursorX,
		LLY: e.cursorY - height,
		URX: e.cursorX + width,
		URY: e.cursorY,
	}

	field := &semantic.ChoiceFormField{
		BaseFormField: semantic.BaseFormField{
			Name:  name,
			Rect:  rect,
			Color: []float64{0.9, 0.9, 0.9},
		},
		Options:  options,
		Selected: selected,
		IsCombo:  true, // Default to combo box style
	}

	e.currentPage.AddFormField(field)

	e.currentPage.DrawRectangle(rect.LLX, rect.LLY, rect.URX-rect.LLX, rect.URY-rect.LLY, builder.RectOptions{
		Fill:      true,
		FillColor: builder.Color{R: 0.9, G: 0.9, B: 0.9},
		Stroke:    true,
		LineWidth: 1,
	})

	e.cursorY -= height + 5
}

func extractText(n *html.Node) string {
	var sb strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		} else if n.Type == html.ElementNode && n.DataAtom == atom.Br {
			sb.WriteString("\n")
		} else if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			for _, attr := range n.Attr {
				if attr.Key == "alt" {
					sb.WriteString(attr.Val)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(sb.String())
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func (e *Engine) extractSpans(n *html.Node) []TextSpan {
	var spans []TextSpan
	e.walkSpans(n, TextStyle{Font: e.DefaultFont, FontSize: e.DefaultFontSize}, &spans)
	return spans
}

type TextStyle struct {
	Font          string
	FontSize      float64
	Bold          bool
	Italic        bool
	Link          string
	Underline     bool
	Strikethrough bool
}

func (e *Engine) walkSpans(n *html.Node, style TextStyle, spans *[]TextSpan) {
	if n.Type == html.TextNode {
		text := n.Data
		// Clean up newlines/spaces?
		// HTML collapses whitespace.
		text = strings.ReplaceAll(text, "\n", " ")

		fontName := e.resolveFont(style.Font, style.Bold, style.Italic)
		*spans = append(*spans, TextSpan{
			Text:          text,
			Font:          fontName,
			FontSize:      style.FontSize,
			Link:          style.Link,
			Underline:     style.Underline,
			Strikethrough: style.Strikethrough,
		})
		return
	}

	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.B, atom.Strong:
			style.Bold = true
		case atom.I, atom.Em:
			style.Italic = true
		case atom.U, atom.Ins:
			style.Underline = true
		case atom.S, atom.Strike, atom.Del:
			style.Strikethrough = true
		case atom.Code, atom.Kbd, atom.Samp, atom.Tt:
			style.Font = "Courier"
		case atom.A:
			for _, a := range n.Attr {
				if a.Key == "href" {
					style.Link = a.Val
					break
				}
			}
		case atom.Br:
			*spans = append(*spans, TextSpan{Text: "\n"})
			return
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.walkSpans(c, style, spans)
	}
}

func (e *Engine) resolveFont(base string, bold, italic bool) string {
	// Simple mapping for standard fonts
	if base == "Helvetica" || base == "Arial" {
		if bold && italic {
			return "Helvetica-BoldOblique"
		}
		if bold {
			return "Helvetica-Bold"
		}
		if italic {
			return "Helvetica-Oblique"
		}
		return "Helvetica"
	}
	if base == "Times" || base == "Times New Roman" {
		if bold && italic {
			return "Times-BoldItalic"
		}
		if bold {
			return "Times-Bold"
		}
		if italic {
			return "Times-Italic"
		}
		return "Times-Roman"
	}
	if base == "Courier" {
		if bold && italic {
			return "Courier-BoldOblique"
		}
		if bold {
			return "Courier-Bold"
		}
		if italic {
			return "Courier-Oblique"
		}
		return "Courier"
	}
	// Fallback: append suffixes?
	return base
}

func (e *Engine) renderHTMLImage(n *html.Node) {
	src := getAttr(n, "src")
	if src == "" {
		return
	}

	var r io.Reader
	var closer io.Closer

	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		resp, err := http.Get(src)
		if err != nil {
			e.renderImageError(n, "Network Error")
			return
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			e.renderImageError(n, fmt.Sprintf("HTTP %d", resp.StatusCode))
			return
		}
		r = resp.Body
		closer = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			e.renderImageError(n, "File Not Found")
			return
		}
		r = f
		closer = f
	}
	defer closer.Close()

	img, _, err := image.Decode(r)
	if err != nil {
		e.renderImageError(n, "Decode Error")
		return
	}

	semImg := imageToSemantic(img)

	w := float64(semImg.Width)
	h := float64(semImg.Height)

	if val := getAttr(n, "width"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			w = v
		}
	}
	if val := getAttr(n, "height"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			h = v
		}
	}

	// Scale to fit page if too large
	maxWidth := e.pageWidth - e.Margins.Left - e.Margins.Right
	if w > maxWidth {
		scale := maxWidth / w
		w = maxWidth
		h *= scale
	}

	e.ensurePage()
	e.checkPageBreak(h)

	e.currentPage.DrawImage(semImg, e.cursorX, e.cursorY-h, w, h, builder.ImageOptions{})
	e.cursorY -= h
	e.renderParagraphSpacing()
}

func (e *Engine) renderImageError(n *html.Node, msg string) {
	alt := getAttr(n, "alt")
	text := fmt.Sprintf("[Image: %s (%s)]", alt, msg)
	if alt == "" {
		text = fmt.Sprintf("[Image: %s]", msg)
	}
	e.renderTextWrapped(text, e.cursorX, e.DefaultFontSize, e.DefaultFontSize*e.LineHeight)
}

func (e *Engine) renderHTMLTable(n *html.Node) {
	var rows []builder.TableRow
	var headerRows int

	// Parse table attributes
	borderW := 1.0
	if val := getAttr(n, "border"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			borderW = v
		}
	}

	padding := 5.0
	if val := getAttr(n, "cellpadding"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			padding = v
		}
	}

	processRow := func(tr *html.Node, isHeader bool) {
		var cells []builder.TableCell
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
				txt := extractText(c)
				colSpan := 1
				if val := getAttr(c, "colspan"); val != "" {
					if v, err := strconv.Atoi(val); err == nil {
						colSpan = v
					}
				}

				bgColor := builder.Color{}
				if isHeader || c.DataAtom == atom.Th {
					bgColor = builder.Color{R: 0.9, G: 0.9, B: 0.9}
				}

				cells = append(cells, builder.TableCell{
					Text:            txt,
					ColSpan:         colSpan,
					BackgroundColor: bgColor,
					BorderWidth:     borderW,
					Padding:         &builder.CellPadding{Top: padding, Bottom: padding, Left: padding, Right: padding},
				})
			}
		}
		if len(cells) > 0 {
			rows = append(rows, builder.TableRow{Cells: cells})
			if isHeader {
				headerRows++
			}
		}
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.DataAtom == atom.Tr {
				isHeader := false
				if n.Parent != nil && n.Parent.DataAtom == atom.Thead {
					isHeader = true
				}
				processRow(n, isHeader)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)

	if len(rows) == 0 {
		return
	}

	maxCols := 0
	for _, r := range rows {
		cols := 0
		for _, c := range r.Cells {
			cols += c.ColSpan
		}
		if cols > maxCols {
			maxCols = cols
		}
	}

	if maxCols == 0 {
		return
	}

	availWidth := e.pageWidth - e.Margins.Left - e.Margins.Right
	colWidth := availWidth / float64(maxCols)
	colWidths := make([]float64, maxCols)
	for i := range colWidths {
		colWidths[i] = colWidth
	}

	e.ensurePage()

	var finalY float64
	e.currentPage = e.currentPage.DrawTable(builder.Table{
		Columns:    colWidths,
		Rows:       rows,
		HeaderRows: headerRows,
	}, builder.TableOptions{
		X:            e.cursorX,
		Y:            e.cursorY,
		BorderWidth:  borderW,
		BorderColor:  builder.Color{R: 0, G: 0, B: 0},
		DefaultFont:  e.DefaultFont,
		DefaultSize:  e.DefaultFontSize,
		LeftMargin:   e.Margins.Left,
		TopMargin:    e.Margins.Top,
		BottomMargin: e.Margins.Bottom,
		FinalY:       &finalY,
	})

	e.cursorY = finalY
	e.renderParagraphSpacing()
}

func imageToSemantic(img image.Image) *semantic.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	data := make([]byte, w*h*3)
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			data[idx] = byte(r >> 8)
			data[idx+1] = byte(g >> 8)
			data[idx+2] = byte(b >> 8)
			idx += 3
		}
	}
	return &semantic.Image{
		Width:            w,
		Height:           h,
		BitsPerComponent: 8,
		ColorSpace:       semantic.DeviceColorSpace{Name: "DeviceRGB"},
		Data:             data,
	}
}
