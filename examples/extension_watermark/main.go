package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"

	"pdflib/builder"
	"pdflib/extensions"
	"pdflib/ir/semantic"
	"pdflib/writer"
)

// WatermarkExtension implements the extensions.Extension interface
// to add a watermark during the Transform phase.
type WatermarkExtension struct {
	Text    string
	Degrees float64
	Alpha   float64
}

func (e *WatermarkExtension) Name() string {
	return "WatermarkExtension"
}

func (e *WatermarkExtension) Phase() extensions.Phase {
	return extensions.PhaseTransform
}

func (e *WatermarkExtension) Priority() int {
	return 100 // Run after other transforms
}

func (e *WatermarkExtension) Execute(ctx extensions.Context, doc *semantic.Document) error {
	if doc == nil {
		return nil
	}
	for _, page := range doc.Pages {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled")
		default:
		}
		e.applyWatermarkToPage(page)
	}
	return nil
}

func (e *WatermarkExtension) applyWatermarkToPage(page *semantic.Page) {
	if page == nil || e.Text == "" {
		return
	}

	ensureResources(page)

	const gsName = "GSWatermark"
	page.Resources.ExtGStates[gsName] = semantic.ExtGState{FillAlpha: float64Ptr(e.Alpha)}
	ensureFont(page, "F1")

	width := page.MediaBox.URX - page.MediaBox.LLX
	height := page.MediaBox.URY - page.MediaBox.LLY
	centerX := page.MediaBox.LLX + width/2
	centerY := page.MediaBox.LLY + height/2

	rad := e.Degrees * math.Pi / 180
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
		{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte(e.Text)}}},
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

func main() {
	out := "extension_watermark.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// 1. Build a basic document
	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Extension Watermark Demo", Author: "pdflib"})
	b.NewPage(595, 842).
		DrawText("Confidential Report", 50, 780, builder.TextOptions{FontSize: 24}).
		DrawText("This document is watermarked via an extension.", 50, 750, builder.TextOptions{FontSize: 12}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		exitErr("build doc", err)
	}

	// 2. Setup Extension Hub
	hub := extensions.NewHub()
	ext := &WatermarkExtension{
		Text:    "EXTENSION",
		Degrees: 45,
		Alpha:   0.1,
	}
	if err := hub.Register(ext); err != nil {
		exitErr("register extension", err)
	}

	// 3. Execute Extensions
	ctx := context.Background()
	if err := hub.Execute(ctx, doc); err != nil {
		exitErr("execute extensions", err)
	}

	// 4. Write the document
	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	if err := w.Write(ctx, doc, &buf, writer.Config{Deterministic: true}); err != nil {
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
