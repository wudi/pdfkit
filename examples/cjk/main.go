package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"pdflib/builder"
	"pdflib/writer"
)

func main() {
	// This example assumes it is run from the project root, so it can access testdata/
	// go run examples/cjk/main.go

	b := builder.NewBuilder()

	// Load SimHei (Chinese)
	simheiData, err := os.ReadFile("testdata/SimHei-Regular.ttf")
	if err != nil {
		panic(fmt.Errorf("failed to read SimHei font: %w", err))
	}
	b.RegisterTrueTypeFont("SimHei", simheiData)

	// Load NotoSansJP (Japanese)
	notoData, err := os.ReadFile("testdata/NotoSansJP-Regular.ttf")
	if err != nil {
		panic(fmt.Errorf("failed to read NotoSansJP font: %w", err))
	}
	b.RegisterTrueTypeFont("NotoSansJP", notoData)

	pageBuilder := b.NewPage(595, 842) // A4 size

	// Draw Chinese Text
	pageBuilder.DrawText("你好，世界", 50, 750, builder.TextOptions{
		Font:     "SimHei",
		FontSize: 24,
	})

	// Draw Japanese Text
	pageBuilder.DrawText("こんにちは世界", 50, 700, builder.TextOptions{
		Font:     "NotoSansJP",
		FontSize: 24,
	})

	pageBuilder.Finish()

	doc, err := b.Build()
	if err != nil {
		panic(err)
	}

	// Verify that the fonts are correctly embedded as Type0 Identity-H (internal check)
	page := doc.Pages[0]
	simhei := page.Resources.Fonts["SimHei"]
	if simhei.Subtype != "Type0" || simhei.Encoding != "Identity-H" {
		fmt.Printf("Warning: SimHei is not Type0 Identity-H. Got Subtype=%s, Encoding=%s\n", simhei.Subtype, simhei.Encoding)
	} else {
		fmt.Println("Verified: SimHei is Type0 Identity-H")
	}

	noto := page.Resources.Fonts["NotoSansJP"]
	if noto.Subtype != "Type0" || noto.Encoding != "Identity-H" {
		fmt.Printf("Warning: NotoSansJP is not Type0 Identity-H. Got Subtype=%s, Encoding=%s\n", noto.Subtype, noto.Encoding)
	} else {
		fmt.Println("Verified: NotoSansJP is Type0 Identity-H")
	}

	// Write the PDF
	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	// Enable SubsetFonts for efficient CJK support
	cfg := writer.Config{
		SubsetFonts: true,
	}
	if err := w.Write(context.Background(), doc, &buf, cfg); err != nil {
		panic(err)
	}

	outFile := "cjk_example.pdf"
	if err := os.WriteFile(outFile, buf.Bytes(), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Successfully generated %s (%d bytes)\n", outFile, buf.Len())
}
