package layout

import (
	"bufio"
	"strings"

	"pdflib/builder"
)

// RenderMarkdown renders a markdown string to the PDF.
func (e *Engine) RenderMarkdown(text string) error {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			e.renderParagraphSpacing()
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			e.renderHeader(trimmed)
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			e.renderListItem(trimmed)
		} else {
			e.renderParagraph(trimmed)
		}
	}
	if e.currentPage != nil {
		e.currentPage.Finish()
	}
	return scanner.Err()
}

func (e *Engine) renderHeader(line string) {
	level := 0
	for _, c := range line {
		if c == '#' {
			level++
		} else {
			break
		}
	}
	text := strings.TrimSpace(line[level:])
	
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

func (e *Engine) renderListItem(line string) {
	text := strings.TrimSpace(line[2:])
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

func (e *Engine) renderParagraph(line string) {
	e.ensurePage()
	fontSize := e.DefaultFontSize
	lineHeight := fontSize * e.LineHeight
	e.renderTextWrapped(line, e.cursorX, fontSize, lineHeight)
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
