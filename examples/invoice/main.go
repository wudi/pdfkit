package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"pdflib/builder"
	"pdflib/ir/semantic"
	"pdflib/writer"
)

// A minimal invoice demo using the fluent builder API.
func main() {
	out := "invoice.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Invoice"})

	logo := solidLogo(48, 48, builder.Color{R: 0.82, G: 0.1, B: 0.16})

	// Page 1 — header + summary
	b.NewPage(595, 842). // A4 in points
				DrawText("Invoice #1234", 50, 800, builder.TextOptions{FontSize: 24, Color: builder.Color{R: 0.1, G: 0.1, B: 0.1}}).
				DrawImage(logo, 480, 780, float64(logo.Width), float64(logo.Height), builder.ImageOptions{}).
				DrawText("Bill To:", 50, 770, builder.TextOptions{FontSize: 12}).
				DrawLine(50, 760, 545, 760, builder.LineOptions{StrokeColor: builder.Color{B: 0.5}, LineWidth: 1}).
				DrawRectangle(50, 600, 495, 140, builder.RectOptions{Stroke: true, Fill: false, LineWidth: 1}).
				DrawText("Item", 60, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Qty", 300, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Amount", 450, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Consulting Services", 60, 680, builder.TextOptions{FontSize: 10}).
				DrawText("10", 300, 680, builder.TextOptions{FontSize: 10}).
				DrawText("$80.00/hr", 450, 680, builder.TextOptions{FontSize: 10}).
				DrawText("API Integration", 60, 660, builder.TextOptions{FontSize: 10}).
				DrawText("1", 300, 660, builder.TextOptions{FontSize: 10}).
				DrawText("$200.00", 450, 660, builder.TextOptions{FontSize: 10}).
				DrawText("Support Retainer", 60, 640, builder.TextOptions{FontSize: 10}).
				DrawText("3", 300, 640, builder.TextOptions{FontSize: 10}).
				DrawText("$100.00", 450, 640, builder.TextOptions{FontSize: 10}).
				DrawText("Subtotal: $1,000.00", 400, 620, builder.TextOptions{FontSize: 12}).
				DrawText("Total: $1,000.00", 400, 600, builder.TextOptions{FontSize: 14}).
				Finish()

	// Page 2 — notes and footer
	notes := strings.Repeat("Thank you for your business. ", 4)
	b.NewPage(595, 842).
		DrawText("Notes", 50, 800, builder.TextOptions{FontSize: 18}).
		DrawText(notes, 50, 780, builder.TextOptions{FontSize: 12}).
		DrawLine(50, 760, 545, 760, builder.LineOptions{StrokeColor: builder.Color{R: 0.3, G: 0.3, B: 0.3}, LineWidth: 0.5}).
		DrawText("Contact: billing@example.com", 50, 740, builder.TextOptions{FontSize: 11}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build doc: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	cfg := writer.Config{Deterministic: true, Linearize: true}
	if err := w.Write(context.Background(), doc, &buf, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "write pdf: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s (%d bytes)\n", out, buf.Len())
}

func solidLogo(w, h int, c builder.Color) *semantic.Image {
	px := make([]byte, 0, w*h*3)
	r := byte(clamp01(c.R) * 255)
	g := byte(clamp01(c.G) * 255)
	b := byte(clamp01(c.B) * 255)
	for i := 0; i < w*h; i++ {
		px = append(px, r, g, b)
	}
	return &semantic.Image{
		Width:            w,
		Height:           h,
		ColorSpace:       semantic.ColorSpace{Name: "DeviceRGB"},
		BitsPerComponent: 8,
		Data:             px,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
