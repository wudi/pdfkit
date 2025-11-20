package layout

import (
	"pdflib/builder"
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
