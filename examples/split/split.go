package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run split.go <input.pdf> <output_dir>")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputDir := os.Args[2]

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Processing %s...\n", inputPath)
	f, err := os.Open(inputPath)
	if err != nil {
		fmt.Printf("Error opening %s: %v\n", inputPath, err)
		os.Exit(1)
	}
	defer f.Close()

	// Parse the input PDF
	pipeline := ir.NewDefault()
	doc, err := pipeline.Parse(context.Background(), f)
	if err != nil {
		fmt.Printf("Error parsing %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Found %d pages.\n", len(doc.Pages))

	for i, page := range doc.Pages {
		pageNum := i + 1
		outputFilename := fmt.Sprintf("page_%d.pdf", pageNum)
		outputPath := filepath.Join(outputDir, outputFilename)

		fmt.Printf("Extracting page %d to %s...\n", pageNum, outputPath)

		// Create a new builder for the single page
		b := builder.NewBuilder()
		b.AddPage(page)

		newDoc, err := b.Build()
		if err != nil {
			fmt.Printf("Error building document for page %d: %v\n", pageNum, err)
			continue
		}

		// Write the single-page PDF
		outFile, err := os.Create(outputPath)
		if err != nil {
			fmt.Printf("Error creating output file %s: %v\n", outputPath, err)
			continue
		}

		w := writer.NewWriter()
		cfg := writer.Config{
			Version:     writer.PDF17,
			Compression: 9,
		}

		err = w.Write(context.Background(), newDoc, outFile, cfg)
		outFile.Close() // Close immediately after writing
		if err != nil {
			fmt.Printf("Error writing page %d: %v\n", pageNum, err)
		}
	}

	fmt.Println("Done!")
}
