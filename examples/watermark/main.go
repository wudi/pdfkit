package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"

	"pdflib/builder"
	"pdflib/ir/semantic"
	"pdflib/writer"
)

// Demonstrates how to add a diagonal watermark to every page of a PDF built
// with the fluent builder API.
func main() {
	out := "watermark.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Watermark Demo", Author: "pdflib"})

	// Page content that will sit behind the watermark.
	b.NewPage(595, 842).
		DrawText("Quarterly Results", 50, 780, builder.TextOptions{FontSize: 28}).
		DrawLine(50, 770, 545, 770, builder.LineOptions{StrokeColor: builder.Color{R: 0.4, G: 0.4, B: 0.4}, LineWidth: 0.5}).
		DrawText("Revenue", 50, 740, builder.TextOptions{FontSize: 18}).
		DrawText("$4.2M", 50, 720, builder.TextOptions{FontSize: 14, Color: builder.Color{R: 0.1, G: 0.6, B: 0.1}}).
		DrawText("Expenses", 50, 690, builder.TextOptions{FontSize: 18}).
		DrawText("$3.1M", 50, 670, builder.TextOptions{FontSize: 14, Color: builder.Color{R: 0.8, G: 0.2, B: 0.2}}).
		DrawText("Summary", 50, 630, builder.TextOptions{FontSize: 18}).
		DrawText("Numbers shown here are sample values for demonstration only.", 50, 610, builder.TextOptions{FontSize: 12}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		exitErr("build doc", err)
	}

	addWatermark(doc, "CONFIDENTIAL", 45, 0.15)

	var buf bytes.Buffer
	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, &buf, writer.Config{Deterministic: true}); err != nil {
		exitErr("write pdf", err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		exitErr("write file", err)
	}
	fmt.Printf("Wrote %s (%d bytes)\n", out, buf.Len())
}

func exitErr(msg string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
	os.Exit(1)
}

func addWatermark(doc *semantic.Document, text string, degrees float64, alpha float64) {
	if doc == nil {
		return
	}
	for _, page := range doc.Pages {
		applyWatermarkToPage(page, text, degrees, alpha)
	}
}

func applyWatermarkToPage(page *semantic.Page, text string, degrees float64, alpha float64) {
	if page == nil || text == "" {
		return
	}

	ensureResources(page)

	const gsName = "GSWatermark"
	page.Resources.ExtGStates[gsName] = semantic.ExtGState{FillAlpha: float64Ptr(alpha)}
	ensureFont(page, "F1")

	width := page.MediaBox.URX - page.MediaBox.LLX
	height := page.MediaBox.URY - page.MediaBox.LLY
	centerX := page.MediaBox.LLX + width/2
	centerY := page.MediaBox.LLY + height/2

	rad := degrees * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)

	// New operations are isolated in their own content stream for clarity.
	ops := []semantic.Operation{
		{Operator: "q"},
		{Operator: "BT"},
		{Operator: "gs", Operands: []semantic.Operand{semantic.NameOperand{Value: gsName}}},
		{Operator: "rg", Operands: []semantic.Operand{
			semantic.NumberOperand{Value: 0.7},
			semantic.NumberOperand{Value: 0.7},
			semantic.NumberOperand{Value: 0.7},
		}},
		{Operator: "Tf", Operands: []semantic.Operand{
			semantic.NameOperand{Value: "F1"},
			semantic.NumberOperand{Value: 72},
		}},
		{Operator: "Tm", Operands: []semantic.Operand{
			semantic.NumberOperand{Value: cos},
			semantic.NumberOperand{Value: sin},
			semantic.NumberOperand{Value: -sin},
			semantic.NumberOperand{Value: cos},
			semantic.NumberOperand{Value: centerX},
			semantic.NumberOperand{Value: centerY},
		}},
		{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte(text)}}},
		{Operator: "ET"},
		{Operator: "Q"},
	}

	page.Contents = append(page.Contents, semantic.ContentStream{Operations: ops})
}

func ensureResources(page *semantic.Page) {
	if page.Resources == nil {
		page.Resources = &semantic.Resources{}
	}
	if page.Resources.Fonts == nil {
		page.Resources.Fonts = make(map[string]*semantic.Font)
	}
	if page.Resources.ExtGStates == nil {
		page.Resources.ExtGStates = make(map[string]semantic.ExtGState)
	}
}

func ensureFont(page *semantic.Page, name string) {
	if _, ok := page.Resources.Fonts[name]; ok {
		return
	}
	page.Resources.Fonts[name] = &semantic.Font{BaseFont: "Helvetica"}
}

func float64Ptr(v float64) *float64 {
	return &v
}
