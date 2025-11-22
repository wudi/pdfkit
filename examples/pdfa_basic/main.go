package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/compliance/pdfa"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	b := builder.NewBuilder()

	fontData, err := os.ReadFile("testdata/Rubik-Regular.ttf")
	if err != nil {
		log.Fatalf("Failed to read font file: %v", err)
	}
	b.RegisterTrueTypeFont("Rubik", fontData)

	b.NewPage(595.28, 841.89).
		DrawText("PDF/A-1b Compliance Demo", 50, 800, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 24,
		}).
		DrawText("This document conforms to PDF/A-1b standards.", 50, 750, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		DrawText("Features:", 50, 720, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 14,
		}).
		DrawText("- Embedded Fonts", 70, 700, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		DrawText("- sRGB OutputIntent", 70, 680, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		DrawText("- No Encryption", 70, 660, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		Finish()

	doc, err := b.Build()
	if err != nil {
		log.Fatalf("Failed to build PDF: %v", err)
	}

	ctx := context.Background()
	enforcer := pdfa.NewEnforcer()
	if err := enforcer.Enforce(ctx, doc, pdfa.PDFA1B); err != nil {
		log.Fatalf("Failed to enforce PDF/A-1b: %v", err)
	}

	report, err := enforcer.Validate(ctx, doc, pdfa.PDFA1B)
	if err != nil {
		log.Fatalf("Failed to validate PDF: %v", err)
	}

	if report.Compliant {
		fmt.Println("Document is PDF/A-1b compliant!")
	} else {
		fmt.Println("Document is NOT compliant:")
		for _, v := range report.Violations {
			fmt.Printf("- [%s] %s: %s\n", v.Code, v.Location, v.Description)
		}
	}

	f, err := os.Create("pdfa_basic.pdf")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Println("Successfully created pdfa_basic.pdf")
}
