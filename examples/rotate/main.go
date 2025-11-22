package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	// Create a sample PDF to rotate
	b := builder.NewBuilder()
	b.NewPage(595, 842).
		DrawText("This page will be rotated 90 degrees.", 100, 700, builder.TextOptions{FontSize: 24}).
		Finish()
	b.NewPage(595, 842).
		DrawText("This page will be rotated 180 degrees.", 100, 700, builder.TextOptions{FontSize: 24}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		panic(err)
	}

	// Rotate pages
	if len(doc.Pages) > 0 {
		doc.Pages[0].Rotate = 90
	}
	if len(doc.Pages) > 1 {
		doc.Pages[1].Rotate = 180
	}

	// Write the output
	f, err := os.Create("rotated.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		panic(err)
	}

	fmt.Println("Successfully created rotated.pdf")
}
