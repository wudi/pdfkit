package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/extractor"
	"github.com/wudi/pdfkit/ir"
)

// Parses a PDF and uses the high-level extractor to dump page text.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./examples/extract_text <pdf-path>\n")
		os.Exit(2)
	}

	if err := run(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "extract text: %v\n", err)
		os.Exit(1)
	}
}

func run(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	pipe := ir.NewDefault()
	doc, err := pipe.Parse(context.Background(), file)
	if err != nil {
		return fmt.Errorf("parse pdf: %w", err)
	}

	dec := doc.Decoded()
	if dec == nil {
		return errors.New("pipeline produced no decoded document")
	}

	ext, err := extractor.New(dec)
	if err != nil {
		return fmt.Errorf("init extractor: %w", err)
	}

	pages, err := ext.ExtractText()
	if err != nil {
		return fmt.Errorf("extract text: %w", err)
	}

	if len(pages) == 0 {
		fmt.Printf("No text content found in %s\n", path)
		return nil
	}

	fmt.Printf("Extracted text for %d page(s) in %s\n\n", len(pages), path)
	for _, page := range pages {
		label := fmt.Sprintf("Page %d", page.Page+1)
		if page.Label != "" {
			label = fmt.Sprintf("%s (page %d)", page.Label, page.Page+1)
		}
		fmt.Printf("%s:\n%s\n\n", label, page.Content)
	}

	return nil
}
