package layout

import (
	"pdflib/builder"
	"strings"

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
