package layout

import (
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
	text := extractText(n)
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
	e.checkPageBreak(lineHeight)

	e.currentPage.DrawText(text, e.cursorX, e.cursorY-fontSize, builder.TextOptions{
		Font:     e.DefaultFont,
		FontSize: fontSize,
	})
	e.cursorY -= lineHeight
}

func (e *Engine) renderHTMLParagraph(n *html.Node) {
	text := extractText(n)
	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight
	e.renderTextWrapped(text, e.cursorX, fontSize, lineHeight)
	e.renderParagraphSpacing()
}

func (e *Engine) renderHTMLListItem(n *html.Node) {
	text := extractText(n)
	e.ensurePage()

	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight

	// Simple bullet
	e.checkPageBreak(lineHeight)
	e.currentPage.DrawText("•", e.cursorX, e.cursorY-fontSize, builder.TextOptions{
		Font:     e.DefaultFont,
		FontSize: fontSize,
	})

	// Indent text
	indent := 15.0
	e.renderTextWrapped(text, e.cursorX+indent, fontSize, lineHeight)
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
			Font:     e.DefaultFont, // Ideally monospace
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
