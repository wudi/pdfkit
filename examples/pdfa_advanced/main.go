package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/compliance/pdfa"
	"github.com/wudi/pdfkit/ir/semantic"
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
		DrawText("INVOICE", 50, 800, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 24,
			Color:    builder.Color{R: 0.2, G: 0.2, B: 0.6},
		}).
		DrawText(fmt.Sprintf("Date: %s", time.Now().Format("2006-01-02")), 400, 800, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		DrawText("Invoice #: INV-2023-001", 400, 780, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 12,
		}).
		DrawLine(50, 770, 545, 770, builder.LineOptions{
			StrokeColor: builder.Color{R: 0, G: 0, B: 0},
			LineWidth:   1,
		}).
		DrawTable(builder.Table{
			Columns:    []float64{250, 80, 80, 85},
			HeaderRows: 1,
			Rows: []builder.TableRow{
				{Cells: []builder.TableCell{
					{Text: "Description", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
					{Text: "Quantity", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
					{Text: "Unit Price", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
					{Text: "Total", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
				}},
				{Cells: []builder.TableCell{
					{Text: "Consulting Services", Font: "Rubik"},
					{Text: "10", Font: "Rubik"},
					{Text: "$150.00", Font: "Rubik"},
					{Text: "$1,500.00", Font: "Rubik"},
				}},
				{Cells: []builder.TableCell{
					{Text: "Software License", Font: "Rubik"},
					{Text: "1", Font: "Rubik"},
					{Text: "$500.00", Font: "Rubik"},
					{Text: "$500.00", Font: "Rubik"},
				}},
				{Cells: []builder.TableCell{
					{Text: "Total", Font: "Rubik", ColSpan: 3, HAlign: builder.HAlignRight},
					{Text: "$2,000.00", Font: "Rubik"},
				}},
			},
		}, builder.TableOptions{
			X: 50, Y: 750,
			RowHeight: 25,
		}).
		Finish()

	xmlContent := `<invoice>
	<id>INV-2023-001</id>
	<date>2023-11-22</date>
	<total>2000.00</total>
	<currency>USD</currency>
</invoice>`

	b.AddEmbeddedFile(semantic.EmbeddedFile{
		Name:         "factur-x.xml",
		Description:  "Invoice Data",
		Subtype:      "text/xml",
		Data:         []byte(xmlContent),
		Relationship: "Alternative",
	})

	b.SetInfo(&semantic.DocumentInfo{
		Title:    "Invoice INV-2023-001",
		Author:   "PDFKit Demo",
		Subject:  "ZUGFeRD Compliance Example",
		Creator:  "PDFKit",
		Producer: "PDFKit",
	})

	doc, err := b.Build()
	if err != nil {
		log.Fatalf("Failed to build PDF: %v", err)
	}

	ctx := context.Background()
	enforcer := pdfa.NewEnforcer()
	if err := enforcer.Enforce(ctx, doc, pdfa.PDFA3B); err != nil {
		log.Fatalf("Failed to enforce PDF/A-3b: %v", err)
	}

	report, err := enforcer.Validate(ctx, doc, pdfa.PDFA3B)
	if err != nil {
		log.Fatalf("Failed to validate PDF: %v", err)
	}

	if report.Compliant {
		fmt.Println("Document is PDF/A-3b compliant!")
	} else {
		fmt.Println("Document is NOT compliant:")
		for _, v := range report.Violations {
			fmt.Printf("- [%s] %s: %s\n", v.Code, v.Location, v.Description)
		}
	}

	f, err := os.Create("pdfa_advanced.pdf")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Println("Successfully created pdfa_advanced.pdf")
}
