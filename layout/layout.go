package layout

import (
	"strings"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
)

// Engine handles the layout and rendering of structured content (Markdown/HTML) into PDF pages.
type Engine struct {
	b builder.PDFBuilder

	// Configuration
	DefaultFont     string
	DefaultFontSize float64
	LineHeight      float64 // Multiplier, e.g., 1.2
	Margins         Margins

	// State
	currentPage builder.PageBuilder
	cursorX     float64
	cursorY     float64
	pageWidth   float64
	pageHeight  float64
}

// Margins defines page margins in points.
type Margins struct {
	Top, Bottom, Left, Right float64
}

// NewEngine creates a new layout engine.
func NewEngine(b builder.PDFBuilder) *Engine {
	return &Engine{
		b:               b,
		DefaultFont:     "Helvetica",
		DefaultFontSize: 12,
		LineHeight:      1.2,
		Margins: Margins{
			Top:    50,
			Bottom: 50,
			Left:   50,
			Right:  50,
		},
		pageWidth:  595.28, // A4 width
		pageHeight: 841.89, // A4 height
	}
}

// SetPageSize sets the dimensions for new pages.
func (e *Engine) SetPageSize(width, height float64) {
	e.pageWidth = width
	e.pageHeight = height
}

// ensurePage makes sure there is a current page and the cursor is valid.
func (e *Engine) ensurePage() {
	if e.currentPage == nil {
		e.newPage()
	}
}

// newPage starts a new page and resets the cursor.
func (e *Engine) newPage() {
	e.currentPage = e.b.NewPage(e.pageWidth, e.pageHeight)
	e.cursorX = e.Margins.Left
	e.cursorY = e.pageHeight - e.Margins.Top
}

// checkPageBreak checks if there is enough space for height; if not, adds a new page.
func (e *Engine) checkPageBreak(height float64) {
	if e.currentPage == nil {
		e.newPage()
		return
	}
	if e.cursorY-height < e.Margins.Bottom {
		e.currentPage.Finish()
		e.newPage()
	}
}

// TextSpan represents a segment of text with specific styling.
type TextSpan struct {
	Text     string
	Font     string
	FontSize float64
	Link     string
	Color    builder.Color
}

func (e *Engine) renderParagraphSpacing() {
	if e.currentPage != nil {
		e.cursorY -= e.DefaultFontSize * e.LineHeight
	}
}

func (e *Engine) renderTextWrapped(text string, x float64, fontSize, lineHeight float64) {
	e.renderSpans([]TextSpan{{
		Text:     text,
		Font:     e.DefaultFont,
		FontSize: fontSize,
	}}, x, lineHeight)
}

func (e *Engine) renderSpans(spans []TextSpan, x, lineHeight float64) {
	if len(spans) == 0 {
		return
	}

	maxWidth := e.pageWidth - e.Margins.Right - x

	type wordSpan struct {
		text  string
		span  TextSpan
		width float64
	}

	var currentLine []wordSpan
	currentLineWidth := 0.0

	flushLine := func() {
		if len(currentLine) == 0 {
			return
		}
		e.checkPageBreak(lineHeight)

		curX := x
		for _, ws := range currentLine {
			opts := builder.TextOptions{
				Font:     ws.span.Font,
				FontSize: ws.span.FontSize,
				Color:    ws.span.Color,
			}
			e.currentPage.DrawText(ws.text, curX, e.cursorY-ws.span.FontSize, opts)

			if ws.span.Link != "" {
				ann := &semantic.LinkAnnotation{
					BaseAnnotation: semantic.BaseAnnotation{
						Subtype: "Link",
						RectVal: semantic.Rectangle{
							LLX: curX,
							LLY: e.cursorY - ws.span.FontSize,
							URX: curX + ws.width,
							URY: e.cursorY,
						},
						Border: []float64{0, 0, 0},
					},
					Action: semantic.URIAction{URI: ws.span.Link},
				}
				e.currentPage.AddAnnotation(ann)
			}

			curX += ws.width
		}
		e.cursorY -= lineHeight
		currentLine = nil
		currentLineWidth = 0
	}

	spaceWidths := make(map[string]float64)
	getSpaceWidth := func(font string, size float64) float64 {
		key := font
		if w, ok := spaceWidths[key]; ok {
			return w * size / 12.0
		}
		w := e.b.MeasureText(" ", 12, font) // Measure at 12 then scale
		spaceWidths[key] = w
		return w * size / 12.0
	}

	for _, span := range spans {
		if span.Text == "" {
			continue
		}

		font := span.Font
		if font == "" {
			font = e.DefaultFont
		}
		size := span.FontSize
		if size == 0 {
			size = e.DefaultFontSize
		}
		span.Font = font
		span.FontSize = size

		spaceW := getSpaceWidth(font, size)

		// Check for leading/trailing spaces in the original text
		hasLeadingSpace := span.Text[0] == ' ' || span.Text[0] == '\n' || span.Text[0] == '\t'
		hasTrailingSpace := span.Text[len(span.Text)-1] == ' ' || span.Text[len(span.Text)-1] == '\n' || span.Text[len(span.Text)-1] == '\t'

		words := strings.Fields(span.Text)
		if len(words) == 0 {
			// It was just whitespace
			if hasLeadingSpace || hasTrailingSpace {
				// Add a space if we are not at start of line
				if len(currentLine) > 0 {
					currentLine = append(currentLine, wordSpan{text: " ", span: span, width: spaceW})
					currentLineWidth += spaceW
				}
			}
			continue
		}

		if hasLeadingSpace && len(currentLine) > 0 {
			currentLine = append(currentLine, wordSpan{text: " ", span: span, width: spaceW})
			currentLineWidth += spaceW
		}

		for i, word := range words {
			w := e.b.MeasureText(word, size, font)

			if i > 0 {
				// Space between words
				if currentLineWidth+spaceW > maxWidth {
					flushLine()
				} else {
					currentLine = append(currentLine, wordSpan{text: " ", span: span, width: spaceW})
					currentLineWidth += spaceW
				}
			}

			if currentLineWidth+w > maxWidth {
				flushLine()
				currentLineWidth = w
				currentLine = append(currentLine, wordSpan{text: word, span: span, width: w})
			} else {
				currentLine = append(currentLine, wordSpan{text: word, span: span, width: w})
				currentLineWidth += w
			}
		}

		if hasTrailingSpace {
			if currentLineWidth+spaceW > maxWidth {
				flushLine()
			} else {
				currentLine = append(currentLine, wordSpan{text: " ", span: span, width: spaceW})
				currentLineWidth += spaceW
			}
		}
	}
	flushLine()
}
