package builder

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/wudi/pdfkit/ir/semantic"
)

// AppearanceGenerator handles the generation of appearance streams for form fields.
type AppearanceGenerator struct {
	Form *semantic.AcroForm
}

func NewAppearanceGenerator(form *semantic.AcroForm) *AppearanceGenerator {
	return &AppearanceGenerator{Form: form}
}

func (g *AppearanceGenerator) Generate(field semantic.FormField) (*semantic.XObject, error) {
	switch f := field.(type) {
	case *semantic.TextFormField:
		return g.generateTextAppearance(f)
	case *semantic.ButtonFormField:
		return g.generateButtonAppearance(f)
	default:
		return nil, fmt.Errorf("unsupported field type for appearance generation: %T", field)
	}
}

func (g *AppearanceGenerator) generateTextAppearance(field *semantic.TextFormField) (*semantic.XObject, error) {
	// 1. Parse DA string
	da := field.GetDefaultAppearance()
	fontName, fontSize, color := parseDA(da)
	if fontName == "" {
		fontName = "Helv" // Default
	}
	if fontSize == 0 {
		fontSize = 12
	}

	// 2. Calculate layout
	rect := field.FieldRect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	// 3. Build content stream
	var buf bytes.Buffer

	// /Tx BMC ... EMC
	buf.WriteString("/Tx BMC\n")
	buf.WriteString("q\n")

	// Clip to border (simplified)
	buf.WriteString(fmt.Sprintf("1 1 %.2f %.2f re W n\n", width-2, height-2))

	// Set font and color
	buf.WriteString(fmt.Sprintf("BT\n/%s %g Tf\n", fontName, fontSize))

	// Set color
	writeColor(&buf, color)

	// Position text
	// Handle Quadding (alignment)
	textWidth := g.measureText(field.Value, fontName, fontSize)

	x := 2.0 // Default padding
	switch field.Quadding {
	case 1: // Center
		x = (width - textWidth) / 2
	case 2: // Right
		x = width - textWidth - 2
	}

	buf.WriteString(fmt.Sprintf("%.2f 2 Td\n", x))

	// Draw text
	text := escapeText(field.Value)
	buf.WriteString(fmt.Sprintf("(%s) Tj\n", text))

	buf.WriteString("ET\n")
	buf.WriteString("Q\n")
	buf.WriteString("EMC\n")

	// 4. Create XObject
	xobj := &semantic.XObject{
		Subtype:   "Form",
		BBox:      semantic.Rectangle{LLX: 0, LLY: 0, URX: width, URY: height},
		Resources: g.Form.DefaultResources, // Inherit DR
		Data:      buf.Bytes(),
	}

	return xobj, nil
}

func (g *AppearanceGenerator) measureText(text string, fontName string, fontSize float64) float64 {
	// Try to find font in DefaultResources
	var font *semantic.Font
	if g.Form.DefaultResources != nil && g.Form.DefaultResources.Fonts != nil {
		font = g.Form.DefaultResources.Fonts[fontName]
	}

	if font == nil {
		// Fallback: assume average width of 0.5 em
		return float64(len(text)) * fontSize * 0.5
	}

	width := 0.0
	for _, r := range text {
		// Simple lookup for WinAnsi/Standard encoding
		// TODO: Handle complex encodings/CMaps
		w := 0
		if font.Widths != nil {
			w = font.Widths[int(r)]
		}
		if w == 0 {
			w = 500 // Default 500/1000 em
		}
		width += float64(w) / 1000.0 * fontSize
	}
	return width
}

func (g *AppearanceGenerator) generateButtonAppearance(field *semantic.ButtonFormField) (*semantic.XObject, error) {
	rect := field.FieldRect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	var buf bytes.Buffer
	buf.WriteString("q\n")

	if field.IsRadio {
		// Draw Circle
		// 1 g (white background)
		// 0 0 0 RG (black border)
		// 0.5 w (line width)
		buf.WriteString("1 g\n0 0 0 RG\n0.5 w\n")

		// Circle approximation using Bezier curves is complex to write manually.
		// We'll use a simplified approach or just a square for now if we want to be lazy,
		// but "Zero Compromise" suggests we should try.
		// Or we can use a font glyph if available (ZapfDingbats l/m/n).
		// Let's draw a circle path.
		cx, cy := width/2, height/2
		r := (min(width, height) / 2) - 1

		// Draw circle (simplified as 4 curves)
		magic := r * 0.551784
		buf.WriteString(fmt.Sprintf("%.2f %.2f m\n", cx+r, cy))
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+r, cy+magic, cx+magic, cy+r, cx, cy+r))
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-magic, cy+r, cx-r, cy+magic, cx-r, cy))
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-r, cy-magic, cx-magic, cy-r, cx, cy-r))
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+magic, cy-r, cx+r, cy-magic, cx+r, cy))
		buf.WriteString("B\n") // Fill and stroke

		if field.Checked {
			// Draw Dot (smaller circle, filled black)
			r2 := r / 2
			magic2 := r2 * 0.551784
			buf.WriteString("0 g\n")
			buf.WriteString(fmt.Sprintf("%.2f %.2f m\n", cx+r2, cy))
			buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+r2, cy+magic2, cx+magic2, cy+r2, cx, cy+r2))
			buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-magic2, cy+r2, cx-r2, cy+magic2, cx-r2, cy))
			buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx-r2, cy-magic2, cx-magic2, cy-r2, cx, cy-r2))
			buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n", cx+magic2, cy-r2, cx+r2, cy-magic2, cx+r2, cy))
			buf.WriteString("f\n") // Fill
		}
	} else if field.IsCheck {
		// Draw Checkbox (Square)
		// 1 g (white background)
		// 0 0 0 RG (black border)
		// 0.5 w (line width)
		buf.WriteString("1 g\n0 0 0 RG\n0.5 w\n")
		buf.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height))
		buf.WriteString("B\n") // Fill and stroke

		if field.Checked {
			// Draw X
			buf.WriteString("0 g\n") // Black
			buf.WriteString("1 w\n")
			padding := 3.0
			buf.WriteString(fmt.Sprintf("%.2f %.2f m %.2f %.2f l S\n", padding, padding, width-padding, height-padding))
			buf.WriteString(fmt.Sprintf("%.2f %.2f m %.2f %.2f l S\n", padding, height-padding, width-padding, padding))
		}
	} else {
		// Push Button (Label)
		// Draw Bevel effect?
		buf.WriteString("0.75 g\n") // Light gray
		buf.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height))
		buf.WriteString("f\n")

		// Draw Label (Caption)
		// Assuming field.OnState or Name is the label?
		// Usually PushButtons have a caption in MK dictionary (CA/RC).
		// We'll use field.Name as fallback or "Button"
		label := "Button"
		// TODO: Get actual caption from widget annotation MK dict if available

		// Center text
		fontSize := 12.0
		textWidth := g.measureText(label, "Helv", fontSize)
		x := (width - textWidth) / 2
		y := (height - fontSize) / 2

		buf.WriteString("0 g\n") // Black text
		buf.WriteString("BT\n/Helv 12 Tf\n")
		buf.WriteString(fmt.Sprintf("%.2f %.2f Td\n", x, y))
		buf.WriteString(fmt.Sprintf("(%s) Tj\n", escapeText(label)))
		buf.WriteString("ET\n")
	}

	buf.WriteString("Q\n")

	xobj := &semantic.XObject{
		Subtype:   "Form",
		BBox:      semantic.Rectangle{LLX: 0, LLY: 0, URX: width, URY: height},
		Resources: g.Form.DefaultResources,
		Data:      buf.Bytes(),
	}
	return xobj, nil
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func parseDA(da string) (fontName string, fontSize float64, color []float64) {
	parts := strings.Fields(da)
	for i := 0; i < len(parts); i++ {
		if strings.HasPrefix(parts[i], "/") {
			fontName = parts[i][1:]
			if i+1 < len(parts) {
				fmt.Sscanf(parts[i+1], "%f", &fontSize)
			}
		} else if parts[i] == "g" || parts[i] == "G" {
			if i >= 1 {
				var c float64
				fmt.Sscanf(parts[i-1], "%f", &c)
				color = []float64{c}
			}
		} else if parts[i] == "rg" || parts[i] == "RG" {
			if i >= 3 {
				var r, g, b float64
				fmt.Sscanf(parts[i-3], "%f", &r)
				fmt.Sscanf(parts[i-2], "%f", &g)
				fmt.Sscanf(parts[i-1], "%f", &b)
				color = []float64{r, g, b}
			}
		} else if parts[i] == "k" || parts[i] == "K" {
			if i >= 4 {
				var c, m, y, k float64
				fmt.Sscanf(parts[i-4], "%f", &c)
				fmt.Sscanf(parts[i-3], "%f", &m)
				fmt.Sscanf(parts[i-2], "%f", &y)
				fmt.Sscanf(parts[i-1], "%f", &k)
				color = []float64{c, m, y, k}
			}
		}
	}
	return
}

func writeColor(buf *bytes.Buffer, color []float64) {
	if len(color) == 1 {
		buf.WriteString(fmt.Sprintf("%.2f g\n", color[0]))
	} else if len(color) == 3 {
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f rg\n", color[0], color[1], color[2]))
	} else if len(color) == 4 {
		buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f k\n", color[0], color[1], color[2], color[3]))
	}
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}
