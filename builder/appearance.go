package builder

import (
	"bytes"
	"fmt"
	"pdflib/ir/semantic"
	"strings"
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
	// TODO: Handle Quadding (alignment)
	// Simple left alignment with padding
	buf.WriteString("2 2 Td\n") 
	
	// Draw text
	text := escapeText(field.Value)
	buf.WriteString(fmt.Sprintf("(%s) Tj\n", text))
	
	buf.WriteString("ET\n")
	buf.WriteString("Q\n")
	buf.WriteString("EMC\n")
	
	// 4. Create XObject
	xobj := &semantic.XObject{
		Subtype: "Form",
		BBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: width, URY: height},
		Resources: g.Form.DefaultResources, // Inherit DR
		Data: buf.Bytes(),
	}
	
	return xobj, nil
}

func (g *AppearanceGenerator) generateButtonAppearance(field *semantic.ButtonFormField) (*semantic.XObject, error) {
	// Only handling Checkboxes for now
	if !field.IsCheck {
		return nil, nil // TODO: Push buttons and Radio buttons
	}

	rect := field.FieldRect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	var buf bytes.Buffer
	buf.WriteString("q\n")
	
	// Draw border/background (simplified)
	// 1 g (white background)
	// 0 0 0 RG (black border)
	// 0.5 w (line width)
	buf.WriteString("1 g\n0 0 0 RG\n0.5 w\n")
	buf.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height))
	buf.WriteString("B\n") // Fill and stroke

	if field.Checked {
		// Draw checkmark using ZapfDingbats if available, or simple lines
		// Assuming ZapfDingbats is available as /ZaDb
		// But we don't know the resource name for ZapfDingbats without looking at DR.
		// For now, let's draw a simple cross (X) using lines.
		
		buf.WriteString("0 g\n") // Black
		buf.WriteString("1 w\n")
		
		// Draw X
		padding := 3.0
		buf.WriteString(fmt.Sprintf("%.2f %.2f m %.2f %.2f l S\n", padding, padding, width-padding, height-padding))
		buf.WriteString(fmt.Sprintf("%.2f %.2f m %.2f %.2f l S\n", padding, height-padding, width-padding, padding))
	}

	buf.WriteString("Q\n")

	xobj := &semantic.XObject{
		Subtype: "Form",
		BBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: width, URY: height},
		Resources: g.Form.DefaultResources,
		Data: buf.Bytes(),
	}
	return xobj, nil
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
