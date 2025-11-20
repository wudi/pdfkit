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

func extractText(n *html.Node) string {
	var sb strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(sb.String())
}
