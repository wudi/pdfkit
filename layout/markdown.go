package layout

import (
	"pdflib/builder"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// RenderMarkdown renders a markdown string to the PDF using goldmark.
func (e *Engine) RenderMarkdown(source string) error {
	md := goldmark.New()
	src := []byte(source)
	doc := md.Parser().Parse(text.NewReader(src))

	return e.walkMarkdown(doc, src)
}

func (e *Engine) walkMarkdown(node ast.Node, source []byte) error {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			e.renderMarkdownHeader(n, source)
		case *ast.Paragraph:
			e.renderMarkdownParagraph(n, source)
		case *ast.List:
			e.walkMarkdown(n, source)
		case *ast.ListItem:
			e.renderMarkdownListItem(n, source)
		case *ast.Text:
			// Text nodes are usually handled within block elements, but if we encounter one here...
		}
	}
	
	if e.currentPage != nil {
		e.currentPage.Finish()
	}
	return nil
}

func (e *Engine) renderMarkdownHeader(n *ast.Heading, source []byte) {
	text := string(n.Text(source))
	level := n.Level

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

func (e *Engine) renderMarkdownListItem(n *ast.ListItem, source []byte) {
	// Get the text content of the list item. 
	// List items usually contain a paragraph or other blocks.
	// For simplicity in this engine, we'll extract text from the first child if it's a paragraph or text.
	
	var textContent string
	if child := n.FirstChild(); child != nil {
		if p, ok := child.(*ast.Paragraph); ok {
			textContent = string(p.Text(source))
		} else if t, ok := child.(*ast.Text); ok {
			textContent = string(t.Segment.Value(source))
		} else {
			// Fallback: try to get text from whatever it is
			textContent = string(child.Text(source))
		}
	}

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
	e.renderTextWrapped(textContent, e.cursorX+indent, fontSize, lineHeight)
}

func (e *Engine) renderMarkdownParagraph(n *ast.Paragraph, source []byte) {
	// Concatenate all text segments in the paragraph
	var sb strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			sb.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				sb.WriteByte(' ')
			}
		} else if _, ok := child.(*ast.CodeSpan); ok {
			// Handle code spans as plain text for now
			sb.WriteString(string(child.Text(source)))
		} else if _, ok := child.(*ast.Emphasis); ok {
			// Handle emphasis as plain text for now
			sb.WriteString(string(child.Text(source)))
		} else {
			sb.WriteString(string(child.Text(source)))
		}
	}
	
	text := sb.String()
	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight
	e.renderTextWrapped(text, e.cursorX, fontSize, lineHeight)
}

func (e *Engine) renderParagraphSpacing() {
	if e.currentPage != nil {
		e.cursorY -= e.DefaultFontSize * e.LineHeight
	}
}

func (e *Engine) renderTextWrapped(text string, x float64, fontSize, lineHeight float64) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	}

	maxWidth := e.pageWidth - e.Margins.Right - x
	currentLine := words[0]

	for _, word := range words[1:] {
		// Rough estimation of width: 0.6 * fontSize * length (very approximate)
		// Ideally we need font metrics here.
		// Since we don't have easy access to font metrics in this simple engine yet,
		// we'll use a heuristic.
		if e.estimateWidth(currentLine+" "+word, fontSize) <= maxWidth {
			currentLine += " " + word
		} else {
			e.checkPageBreak(lineHeight)
			e.currentPage.DrawText(currentLine, x, e.cursorY-fontSize, builder.TextOptions{
				Font:     e.DefaultFont,
				FontSize: fontSize,
			})
			e.cursorY -= lineHeight
			currentLine = word
		}
	}
	e.checkPageBreak(lineHeight)
	e.currentPage.DrawText(currentLine, x, e.cursorY-fontSize, builder.TextOptions{
		Font:     e.DefaultFont,
		FontSize: fontSize,
	})
	e.cursorY -= lineHeight
}

func (e *Engine) estimateWidth(text string, fontSize float64) float64 {
	// Very rough estimate assuming average char width is 0.5 em
	return float64(len(text)) * fontSize * 0.5
}
