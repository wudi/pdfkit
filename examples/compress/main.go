package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <input.pdf> <output.pdf>")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]

	// 1. Read the input file
	fileBytes, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	// 2. Parse the PDF
	pipeline := ir.NewDefault()
	doc, err := pipeline.Parse(context.Background(), bytes.NewReader(fileBytes))
	if err != nil {
		log.Fatalf("Failed to parse PDF: %v", err)
	}

	// 3. Configure compression
	// Compression level 9 (max) and ObjectStreams enabled for best results
	cfg := writer.Config{
		Compression:   9,
		ObjectStreams: true,
	}

	// 4. Write the compressed PDF
	outFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, outFile, cfg); err != nil {
		log.Fatalf("Failed to write compressed PDF: %v", err)
	}

	// Calculate savings
	inInfo, _ := os.Stat(inputPath)
	outInfo, _ := outFile.Stat()

	fmt.Printf("Successfully compressed '%s' to '%s'\n", inputPath, outputPath)
	fmt.Printf("Original size: %d bytes\n", inInfo.Size())
	fmt.Printf("Compressed size: %d bytes\n", outInfo.Size())
	if inInfo.Size() > 0 {
		fmt.Printf("Reduction: %.2f%%\n", 100.0*(1.0-float64(outInfo.Size())/float64(inInfo.Size())))
	}
}
