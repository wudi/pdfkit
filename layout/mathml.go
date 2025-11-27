package layout

import (
	"strings"

	"github.com/wudi/pdfkit/builder"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type mathBox struct {
	width    float64
	height   float64
	ascent   float64
	descent  float64
	children []*mathBox
	node     *html.Node
	x, y     float64 // Relative to parent
	text     string  // For text nodes
	fontSize float64
}

func (e *Engine) renderMath(n *html.Node) {
	// 1. Measure
	box := e.measureMath(n, e.DefaultFontSize)
	if box == nil {
		return
	}

	// 2. Check page break
	e.checkPageBreak(box.height)

	// 3. Draw
	e.drawMathBox(box, e.cursorX, e.cursorY)

	// 4. Advance cursor
	e.cursorX += box.width

	// If <math> is top level, we treat it as block.
	if n.Parent == nil || n.Parent.DataAtom == atom.Body {
		e.cursorY -= box.height
		e.cursorX = e.Margins.Left
		e.renderParagraphSpacing()
	}
}

func (e *Engine) measureMath(n *html.Node, fontSize float64) *mathBox {
	box := &mathBox{node: n, fontSize: fontSize}

	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text == "" {
			return nil
		}
		box.text = text
		box.width = e.b.MeasureText(text, fontSize, e.DefaultFont)
		box.ascent = fontSize * 0.8  // Approx
		box.descent = fontSize * 0.2 // Approx
		box.height = box.ascent + box.descent
		return box
	}

	if n.Type != html.ElementNode {
		return nil
	}

	// Recurse
	var children []*mathBox
	// Handle script size reduction

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		fs := fontSize
		if (n.Data == "msup" || n.Data == "msub") && len(children) > 0 {
			fs = fontSize * 0.7
		}
		childBox := e.measureMath(c, fs)
		if childBox != nil {
			children = append(children, childBox)
		}
	}
	box.children = children

	// Layout based on tag
	switch n.Data {
	case "math", "mrow":
		// Horizontal layout
		var w, asc, desc float64
		for _, c := range children {
			c.x = w
			w += c.width
			if c.ascent > asc {
				asc = c.ascent
			}
			if c.descent > desc {
				desc = c.descent
			}
		}
		box.width = w
		box.ascent = asc
		box.descent = desc
		box.height = asc + desc
		// Align children to baseline
		for _, c := range children {
			c.y = 0
		}

	case "mi", "mn", "mo":
		// Text containers
		if len(children) > 0 {
			c := children[0]
			box.width = c.width
			box.ascent = c.ascent
			box.descent = c.descent
			box.height = c.height
			c.x = 0
			c.y = 0
		}

	case "mfrac":
		if len(children) >= 2 {
			num := children[0]
			den := children[1]

			w := num.width
			if den.width > w {
				w = den.width
			}
			box.width = w + 4 // Padding

			// Center align
			num.x = (box.width - num.width) / 2
			den.x = (box.width - den.width) / 2

			// Vertical positioning
			// Line thickness approx 1px -> 0.5pt
			lineH := 0.5

			num.y = num.descent + lineH + 2
			den.y = -(den.ascent + lineH + 2)

			box.ascent = num.y + num.ascent
			box.descent = -den.y + den.descent
			box.height = box.ascent + box.descent
		}

	case "msup":
		if len(children) >= 2 {
			base := children[0]
			sup := children[1]

			box.width = base.width + sup.width

			base.x = 0
			base.y = 0

			sup.x = base.width
			sup.y = base.ascent * 0.5 // Shift up

			box.ascent = base.ascent
			if sup.y+sup.ascent > box.ascent {
				box.ascent = sup.y + sup.ascent
			}
			box.descent = base.descent
			box.height = box.ascent + box.descent
		}

	case "msub":
		if len(children) >= 2 {
			base := children[0]
			sub := children[1]

			box.width = base.width + sub.width

			base.x = 0
			base.y = 0

			sub.x = base.width
			sub.y = -base.descent * 0.5 // Shift down

			box.ascent = base.ascent
			box.descent = base.descent
			if -sub.y+sub.descent > box.descent {
				box.descent = -sub.y + sub.descent
			}
			box.height = box.ascent + box.descent
		}

	case "msqrt":
		// Simple implementation: draw line over content
		// Content is usually an mrow
		var w, asc, desc float64
		for _, c := range children {
			c.x = w + 5 // Space for root symbol
			w += c.width
			if c.ascent > asc {
				asc = c.ascent
			}
			if c.descent > desc {
				desc = c.descent
			}
		}
		box.width = w + 5
		box.ascent = asc + 2 // Line above
		box.descent = desc
		box.height = box.ascent + box.descent

		for _, c := range children {
			c.y = 0
		}

	default:
		// Default to mrow behavior
		var w, asc, desc float64
		for _, c := range children {
			c.x = w
			w += c.width
			if c.ascent > asc {
				asc = c.ascent
			}
			if c.descent > desc {
				desc = c.descent
			}
		}
		box.width = w
		box.ascent = asc
		box.descent = desc
		box.height = asc + desc
	}

	return box
}

func (e *Engine) drawMathBox(box *mathBox, x, y float64) {
	if box == nil {
		return
	}

	// Draw text if present
	if box.text != "" {
		e.currentPage.DrawText(box.text, x, y, builder.TextOptions{
			Font:     e.DefaultFont,
			FontSize: box.fontSize,
		})
	}

	// Draw decorations based on tag
	if box.node != nil {
		switch box.node.Data {
		case "mfrac":
			// Draw fraction line
			// y is baseline.
			// We want line around y=0 relative to box?
			// Actually y passed here is the baseline of the box.
			// The children are positioned relative to this baseline.
			// num is at y + num.y (positive), den is at y + den.y (negative).
			// Line should be at y + something small.
			lineY := y + 2 // Slightly above baseline
			e.currentPage.DrawLine(x, lineY, x+box.width, lineY, builder.LineOptions{
				LineWidth:   0.5,
				StrokeColor: builder.Color{R: 0, G: 0, B: 0},
			})
		case "msqrt":
			// Draw root symbol and line
			// Line over content
			lineY := y + box.ascent - 1
			e.currentPage.DrawLine(x+2, lineY, x+box.width, lineY, builder.LineOptions{
				LineWidth:   0.5,
				StrokeColor: builder.Color{R: 0, G: 0, B: 0},
			})
			// Draw "V" part of root
			e.currentPage.DrawLine(x, y+box.ascent/2, x+2, y-box.descent, builder.LineOptions{
				LineWidth:   0.5,
				StrokeColor: builder.Color{R: 0, G: 0, B: 0},
			})
			e.currentPage.DrawLine(x+2, y-box.descent, x+5, lineY, builder.LineOptions{
				LineWidth:   0.5,
				StrokeColor: builder.Color{R: 0, G: 0, B: 0},
			})
		}
	}

	// Draw children
	for _, c := range box.children {
		e.drawMathBox(c, x+c.x, y+c.y)
	}
}
