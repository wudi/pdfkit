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

// Option defines a configuration option for the Engine.
type Option func(*Engine)

// WithDefaultFont sets the default font.
func WithDefaultFont(font string) Option {
	return func(e *Engine) {
		e.DefaultFont = font
	}
}

// WithDefaultFontSize sets the default font size.
func WithDefaultFontSize(size float64) Option {
	return func(e *Engine) {
		e.DefaultFontSize = size
	}
}

// WithLineHeight sets the line height multiplier.
func WithLineHeight(height float64) Option {
	return func(e *Engine) {
		e.LineHeight = height
	}
}

// WithMargins sets the page margins.
func WithMargins(margins Margins) Option {
	return func(e *Engine) {
		e.Margins = margins
	}
}

// WithPageSize sets the page dimensions.
func WithPageSize(width, height float64) Option {
	return func(e *Engine) {
		e.pageWidth = width
		e.pageHeight = height
	}
}

// WithPaperSize sets the page dimensions using a standard paper size.
func WithPaperSize(size builder.PaperSize) Option {
	return func(e *Engine) {
		e.pageWidth = size.Weight
		e.pageHeight = size.Height
	}
}

// NewEngine creates a new layout engine with optional configuration.
func NewEngine(b builder.PDFBuilder, opts ...Option) *Engine {
	e := &Engine{
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
		pageWidth:  builder.A4.Weight,
		pageHeight: builder.A4.Height,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
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
	Text          string
	Font          string
	FontSize      float64
	Link          string
	Color         builder.Color
	Underline     bool
	Strikethrough bool
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

			if ws.span.Underline {
				e.currentPage.DrawLine(curX, e.cursorY-ws.span.FontSize-2, curX+ws.width, e.cursorY-ws.span.FontSize-2, builder.LineOptions{
					StrokeColor: ws.span.Color,
					LineWidth:   1,
				})
			}
			if ws.span.Strikethrough {
				midY := e.cursorY - ws.span.FontSize/2 + 2
				e.currentPage.DrawLine(curX, midY, curX+ws.width, midY, builder.LineOptions{
					StrokeColor: ws.span.Color,
					LineWidth:   1,
				})
			}

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

		// Tokenize preserving spaces
		var tokens []string
		var currentToken strings.Builder
		for _, r := range span.Text {
			if r == ' ' || r == '\n' || r == '\t' {
				if currentToken.Len() > 0 {
					tokens = append(tokens, currentToken.String())
					currentToken.Reset()
				}
				tokens = append(tokens, " ")
			} else {
				currentToken.WriteRune(r)
			}
		}
		if currentToken.Len() > 0 {
			tokens = append(tokens, currentToken.String())
		}

		for _, token := range tokens {
			if token == " " {
				if currentLineWidth+spaceW > maxWidth {
					flushLine()
				} else {
					currentLine = append(currentLine, wordSpan{text: " ", span: span, width: spaceW})
					currentLineWidth += spaceW
				}
				continue
			}

			w := e.b.MeasureText(token, size, font)

			if currentLineWidth+w > maxWidth {
				// Check if the word itself is longer than the line
				if w > maxWidth {
					// Character-level wrapping
					flushLine()
					var subToken strings.Builder
					subWidth := 0.0
					for _, r := range token {
						rw := e.b.MeasureText(string(r), size, font)
						if subWidth+rw > maxWidth {
							// Flush current sub-token
							if subToken.Len() > 0 {
								currentLine = append(currentLine, wordSpan{text: subToken.String(), span: span, width: subWidth})
								flushLine()
							}
							subToken.Reset()
							subWidth = 0
						}
						subToken.WriteRune(r)
						subWidth += rw
					}
					if subToken.Len() > 0 {
						currentLine = append(currentLine, wordSpan{text: subToken.String(), span: span, width: subWidth})
						currentLineWidth = subWidth
					}
				} else {
					flushLine()
					currentLine = append(currentLine, wordSpan{text: token, span: span, width: w})
					currentLineWidth = w
				}
			} else {
				currentLine = append(currentLine, wordSpan{text: token, span: span, width: w})
				currentLineWidth += w
			}
		}
	}
	flushLine()
}
