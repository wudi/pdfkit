package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run merge.go <output.pdf> <input1.pdf> <input2.pdf> ...")
		os.Exit(1)
	}

	outputPath := os.Args[1]
	inputPaths := os.Args[2:]

	b := builder.NewBuilder()

	for _, inputPath := range inputPaths {
		fmt.Printf("Processing %s...\n", inputPath)
		f, err := os.Open(inputPath)
		if err != nil {
			fmt.Printf("Error opening %s: %v\n", inputPath, err)
			os.Exit(1)
		}
		// We defer close, but inside a loop it's better to close explicitly or use a function.
		// However, for a simple script, defer is okay if not too many files.
		// But to be safe, let's wrap in a func or close manually.
		func() {
			defer f.Close()

			// Parse the input PDF
			pipeline := ir.NewDefault()
			doc, err := pipeline.Parse(context.Background(), f)
			if err != nil {
				fmt.Printf("Error parsing %s: %v\n", inputPath, err)
				os.Exit(1)
			}

			// Add pages to the builder
			for _, page := range doc.Pages {
				b.AddPage(page)
			}
		}()
	}

	// Build the new document
	newDoc, err := b.Build()
	if err != nil {
		fmt.Printf("Error building new document: %v\n", err)
		os.Exit(1)
	}

	// Write the output PDF
	outFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating output file %s: %v\n", outputPath, err)
		os.Exit(1)
	}
	defer outFile.Close()

	w := writer.NewWriter()
	cfg := writer.Config{
		Version:     writer.PDF17,
		Compression: 9,
	}

	fmt.Printf("Writing to %s...\n", outputPath)
	err = w.Write(context.Background(), newDoc, outFile, cfg)
	if err != nil {
		fmt.Printf("Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done!")
}
