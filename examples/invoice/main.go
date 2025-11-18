package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

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

	b.NewPage(595, 842). // A4 in points
				DrawText("Invoice #1234", 50, 800, builder.TextOptions{FontSize: 24, Color: builder.Color{R: 0.1, G: 0.1, B: 0.1}}).
				DrawText("Bill To:", 50, 770, builder.TextOptions{FontSize: 12}).
				DrawLine(50, 760, 545, 760, builder.LineOptions{StrokeColor: builder.Color{B: 0.5}, LineWidth: 1}).
				DrawRectangle(50, 600, 495, 120, builder.RectOptions{Stroke: true, Fill: false, LineWidth: 1}).
				DrawText("Item", 60, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Qty", 300, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Amount", 450, 700, builder.TextOptions{FontSize: 10}).
				DrawText("Subtotal: $100.00", 400, 620, builder.TextOptions{FontSize: 12}).
				DrawText("Total: $100.00", 400, 600, builder.TextOptions{FontSize: 14}).
				Finish()

	doc, err := b.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build doc: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	cfg := writer.Config{Deterministic: true}
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
