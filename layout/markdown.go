package layout

import (
	"strings"

	"github.com/wudi/pdfkit/builder"

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
		case *ast.Blockquote:
			e.renderMarkdownBlockquote(n, source)
		case *ast.FencedCodeBlock:
			e.renderMarkdownCodeBlock(n, source)
		case *ast.CodeBlock:
			e.renderMarkdownCodeBlock(n, source)
		case *ast.ThematicBreak:
			e.renderMarkdownThematicBreak(n, source)
		case *ast.HTMLBlock:
			// Treat HTML blocks as paragraphs for now, extracting text
			e.renderMarkdownParagraph(&ast.Paragraph{BaseBlock: n.BaseBlock}, source)
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
	text := e.extractInlineText(n, source)
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
			textContent = e.extractInlineText(p, source)
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
	text := e.extractInlineText(n, source)
	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight
	e.renderTextWrapped(text, e.cursorX, fontSize, lineHeight)
}

func (e *Engine) renderMarkdownBlockquote(n *ast.Blockquote, source []byte) {
	oldLeft := e.Margins.Left
	e.Margins.Left += 20
	e.cursorX = e.Margins.Left

	e.walkMarkdown(n, source)

	e.Margins.Left = oldLeft
	e.cursorX = e.Margins.Left
	e.renderParagraphSpacing()
}

func (e *Engine) renderMarkdownCodeBlock(n ast.Node, source []byte) {
	var lines []string
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		lines = append(lines, string(line.Value(source)))
	}

	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight

	// Indent code blocks
	indent := 20.0

	for _, line := range lines {
		// Trim newline at end of line if present
		line = strings.TrimRight(line, "\r\n")

		e.checkPageBreak(lineHeight)
		e.currentPage.DrawText(line, e.cursorX+indent, e.cursorY-fontSize, builder.TextOptions{
			Font:     e.DefaultFont, // Ideally monospace
			FontSize: fontSize,
		})
		e.cursorY -= lineHeight
	}
	e.renderParagraphSpacing()
}

func (e *Engine) renderMarkdownThematicBreak(n *ast.ThematicBreak, source []byte) {
	e.ensurePage()
	e.cursorY -= e.DefaultFontSize

	e.checkPageBreak(2)
	e.currentPage.DrawLine(e.Margins.Left, e.cursorY, e.pageWidth-e.Margins.Right, e.cursorY, builder.LineOptions{
		LineWidth:   1,
		StrokeColor: builder.Color{R: 0.5, G: 0.5, B: 0.5},
	})

	e.cursorY -= e.DefaultFontSize
}

func (e *Engine) extractInlineText(n ast.Node, source []byte) string {
	var sb strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			sb.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				sb.WriteByte(' ')
			}
		} else if _, ok := child.(*ast.CodeSpan); ok {
			sb.WriteString(string(child.Text(source)))
		} else if _, ok := child.(*ast.Emphasis); ok {
			sb.WriteString(e.extractInlineText(child, source))
		} else if _, ok := child.(*ast.Link); ok {
			sb.WriteString(e.extractInlineText(child, source))
		} else if img, ok := child.(*ast.Image); ok {
			sb.WriteString(string(img.Text(source)))
		} else if _, ok := child.(*ast.AutoLink); ok {
			sb.WriteString(string(child.Text(source)))
		} else if _, ok := child.(*ast.RawHTML); ok {
			sb.WriteString(string(child.Text(source)))
		} else {
			sb.WriteString(string(child.Text(source)))
		}
	}
	return sb.String()
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
		// Use actual font metrics
		width := e.b.MeasureText(currentLine+" "+word, fontSize, e.DefaultFont)
		if width <= maxWidth {
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
