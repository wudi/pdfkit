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

// Demonstrates how to embed a TrueType font and draw UTF-8 text with it.
func main() {
	out := "fonts.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	simHeiFontData, err := os.ReadFile("testdata/SimHei-Regular.ttf")
	if err != nil {
		exitErr("read SimHei-Regular.ttf", err)
	}

	notoSansFontData, err := os.ReadFile("testdata/NotoSansJP-Regular.ttf")
	if err != nil {
		exitErr("read NotoSansJP-Regular.ttf", err)
	}

	rubikFontData, err := os.ReadFile("testdata/Rubik-Regular.ttf")
	if err != nil {
		exitErr("read Rubik-Regular.ttf", err)
	}

	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{
		Title:   "Embedded Font Demo",
		Author:  "pdflib",
		Subject: "TrueType example",
	})
	b.RegisterTrueTypeFont("SimHei-Regular", simHeiFontData)
	b.RegisterTrueTypeFont("NotoSansJP-Regular", notoSansFontData)
	b.RegisterTrueTypeFont("Rubik-Regular", rubikFontData)

	page := b.NewPage(595, 842)
	page.DrawText("Embedded Fonts", 72, 760, builder.TextOptions{
		FontSize: 18,
		Color:    builder.Color{R: 0.2, G: 0.2, B: 0.2},
	})
	page.DrawText("你好，世界！", 72, 700, builder.TextOptions{
		Font:     "SimHei-Regular",
		FontSize: 48,
		Color:    builder.Color{R: 0.1, G: 0.1, B: 0.1},
	})
	page.DrawText("こんにちは世界", 72, 640, builder.TextOptions{
		Font:     "NotoSansJP-Regular",
		FontSize: 48,
		Color:    builder.Color{R: 0.1, G: 0.1, B: 0.1},
	})
	page.DrawText("مرحبا بالعالم", 72, 580, builder.TextOptions{
		Font:     "Rubik-Regular",
		FontSize: 48,
		Color:    builder.Color{R: 0.1, G: 0.1, B: 0.1},
	})
	page.DrawText("The greeting above uses the embedded TrueType font.", 72, 400, builder.TextOptions{FontSize: 12})
	page.Finish()

	doc, err := b.Build()
	if err != nil {
		exitErr("build doc", err)
	}

	var buf bytes.Buffer
	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, &buf, writer.Config{Deterministic: true, SubsetFonts: true}); err != nil {
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
