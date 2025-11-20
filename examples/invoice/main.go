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

func main() {
	out := "invoice.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Invoice"})

	// Colors
	primaryBlue := builder.Color{R: 0.0, G: 0.5, B: 0.8}
	textDark := builder.Color{R: 0.2, G: 0.2, B: 0.2}
	textGrey := builder.Color{R: 0.5, G: 0.5, B: 0.5}
	bgGrey := builder.Color{R: 0.96, G: 0.96, B: 0.96}
	watermarkColor := builder.Color{R: 0.92, G: 0.92, B: 0.92}

	// Page setup
	width, height := 595.0, 842.0
	margin := 40.0

	pb := b.NewPage(width, height)

	// --- Watermark ---
	// "Paid" text rotated 45 degrees in the center
	pb.DrawText("Paid", width/2-100, height/2-50, builder.TextOptions{
		FontSize: 120,
		Color:    watermarkColor,
		Rotate:   45,
		Font:     "Helvetica-Bold",
	})

	// --- Header ---
	// Logo (Top Left)
	logo := solidLogo(40, 40, primaryBlue)
	pb.DrawImage(logo, margin, height-80, 40, 40, builder.ImageOptions{})
	pb.DrawText("SlicedInvoices", margin+50, height-70, builder.TextOptions{
		FontSize: 20,
		Color:    primaryBlue,
		Font:     "Helvetica-BoldOblique",
	})

	// "Invoice" Title (Top Right)
	pb.DrawText("Invoice", width-margin-100, height-70, builder.TextOptions{
		FontSize: 24,
		Color:    textDark,
		Font:     "Helvetica-Bold",
	})

	// --- Info Table (Top Right) ---
	// Invoice Number, Order Number, etc.
	infoTableY := height - 110.0
	infoTableX := width - margin - 250.0

	infoData := [][]string{
		{"Invoice Number", "INV-3337"},
		{"Order Number", "12345"},
		{"Invoice Date", "January 25, 2016"},
		{"Due Date", "January 31, 2016"},
		{"Total Due", "$93.50"},
	}

	// We construct the table manually using DrawTable
	// Note: The builder.Table struct expects Columns widths.
	pb.DrawTable(builder.Table{
		Columns: []float64{100, 150},
		Rows: func() []builder.TableRow {
			rows := make([]builder.TableRow, len(infoData))
			for i, d := range infoData {
				bg := builder.Color{}
				font := "Helvetica"
				if d[0] == "Total Due" {
					bg = bgGrey
					font = "Helvetica-Bold"
				}
				rows[i] = builder.TableRow{
					Cells: []builder.TableCell{
						{Text: d[0], Font: font, FontSize: 9, TextColor: textDark, BackgroundColor: bg, BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8}, BorderWidth: 0.5, Padding: &builder.CellPadding{Top: 4, Bottom: 4, Left: 5, Right: 5}},
						{Text: d[1], Font: font, FontSize: 9, TextColor: textDark, BackgroundColor: bg, BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8}, BorderWidth: 0.5, Padding: &builder.CellPadding{Top: 4, Bottom: 4, Left: 5, Right: 5}},
					},
				}
			}
			return rows
		}(),
	}, builder.TableOptions{
		X:           infoTableX,
		Y:           infoTableY,
		BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8},
		BorderWidth: 0.5,
	})

	// --- From Section (Left) ---
	currentY := height - 130.0
	pb.DrawText("From:", margin, currentY, builder.TextOptions{FontSize: 10, Font: "Helvetica-Bold", Color: textDark})
	currentY -= 15
	pb.DrawText("DEMO - Sliced Invoices", margin, currentY, builder.TextOptions{FontSize: 10, Color: primaryBlue})
	currentY -= 12
	pb.DrawText("Suite 5A-1204", margin, currentY, builder.TextOptions{FontSize: 10, Color: textGrey})
	currentY -= 12
	pb.DrawText("123 Somewhere Street", margin, currentY, builder.TextOptions{FontSize: 10, Color: textGrey})
	currentY -= 12
	pb.DrawText("Your City AZ 12345", margin, currentY, builder.TextOptions{FontSize: 10, Color: textGrey})
	currentY -= 12
	pb.DrawText("admin@slicedinvoices.com", margin, currentY, builder.TextOptions{FontSize: 10, Color: textDark})

	// --- To Section (Left) ---
	currentY -= 40
	pb.DrawText("To:", margin, currentY, builder.TextOptions{FontSize: 10, Font: "Helvetica-Bold", Color: textDark})
	currentY -= 15
	pb.DrawText("Test Business", margin, currentY, builder.TextOptions{FontSize: 10, Color: textDark})
	currentY -= 12
	pb.DrawText("123 Somewhere St", margin, currentY, builder.TextOptions{FontSize: 10, Color: textGrey})
	currentY -= 12
	pb.DrawText("Melbourne, VIC 3000", margin, currentY, builder.TextOptions{FontSize: 10, Color: textGrey})
	currentY -= 12
	pb.DrawText("test@test.com", margin, currentY, builder.TextOptions{FontSize: 10, Color: textDark})

	// --- Items Table ---
	tableY := currentY - 50.0

	// Columns: Hrs/Qty (10%), Service (45%), Rate/Price (15%), Adjust (15%), Sub Total (15%)
	// Total width = 595 - 80 = 515
	colWidths := []float64{50, 235, 80, 70, 80}

	headers := []string{"Hrs/Qty", "Service", "Rate/Price", "Adjust", "Sub Total"}
	headerRow := builder.TableRow{Cells: make([]builder.TableCell, len(headers))}
	for i, h := range headers {
		align := builder.HAlignRight
		if i == 1 {
			align = builder.HAlignLeft
		} else if i == 0 {
			align = builder.HAlignCenter
		}
		headerRow.Cells[i] = builder.TableCell{
			Text:            h,
			Font:            "Helvetica-Bold",
			FontSize:        10,
			BackgroundColor: bgGrey,
			TextColor:       textDark,
			HAlign:          align,
			Padding:         &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5},
			BorderColor:     builder.Color{R: 0.8, G: 0.8, B: 0.8},
			BorderWidth:     0.5,
		}
	}

	// Item Row
	itemRow := builder.TableRow{Cells: []builder.TableCell{
		{Text: "1.00", HAlign: builder.HAlignCenter, FontSize: 10, TextColor: textDark, Padding: &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5}},
		{Text: "Web Design\nThis is a sample description...", FontSize: 10, TextColor: textDark, Padding: &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5}},
		{Text: "$85.00", HAlign: builder.HAlignRight, FontSize: 10, TextColor: textDark, Padding: &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5}},
		{Text: "0.00%", HAlign: builder.HAlignRight, FontSize: 10, TextColor: textDark, Padding: &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5}},
		{Text: "$85.00", HAlign: builder.HAlignRight, FontSize: 10, TextColor: textDark, Padding: &builder.CellPadding{Top: 6, Bottom: 6, Left: 5, Right: 5}},
	}}
	// Add borders to item row
	for i := range itemRow.Cells {
		itemRow.Cells[i].BorderColor = builder.Color{R: 0.8, G: 0.8, B: 0.8}
		itemRow.Cells[i].BorderWidth = 0.5
	}

	pb.DrawTable(builder.Table{
		Columns:    colWidths,
		Rows:       []builder.TableRow{headerRow, itemRow},
		HeaderRows: 1,
	}, builder.TableOptions{
		X:           margin,
		Y:           tableY,
		BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8},
		BorderWidth: 0.5,
	})

	// --- Totals Section ---
	// We can use another table for this, right aligned.
	// It should be under the main table.
	// We need to estimate where the main table ended.
	// Header height ~24, Item height ~36 (due to newline). Total ~60.
	totalsY := tableY - 70.0

	totalsData := [][]string{
		{"Sub Total", "$85.00"},
		{"Tax", "$8.50"},
		{"Total", "$93.50"},
	}

	totalsRows := make([]builder.TableRow, len(totalsData))
	for i, row := range totalsData {
		font := "Helvetica"
		if row[0] == "Total" {
			font = "Helvetica-Bold"
		}
		totalsRows[i] = builder.TableRow{
			Cells: []builder.TableCell{
				{Text: row[0], Font: font, FontSize: 10, HAlign: builder.HAlignRight, TextColor: textDark, Padding: &builder.CellPadding{Top: 4, Bottom: 4, Left: 5, Right: 5}, BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8}, BorderWidth: 0.5},
				{Text: row[1], Font: font, FontSize: 10, HAlign: builder.HAlignRight, TextColor: textDark, Padding: &builder.CellPadding{Top: 4, Bottom: 4, Left: 5, Right: 5}, BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8}, BorderWidth: 0.5},
			},
		}
	}

	pb.DrawTable(builder.Table{
		Columns: []float64{100, 100},
		Rows:    totalsRows,
	}, builder.TableOptions{
		X:           width - margin - 200,
		Y:           totalsY,
		BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8},
		BorderWidth: 0.5,
	})

	// --- Footer ---
	footerY := 150.0
	pb.DrawText("ANZ Bank", margin, footerY, builder.TextOptions{FontSize: 10, Color: textDark})
	pb.DrawText("ACC # 1234 1234", margin, footerY-12, builder.TextOptions{FontSize: 10, Color: textDark})
	pb.DrawText("BSB # 4321 432", margin, footerY-24, builder.TextOptions{FontSize: 10, Color: textDark})

	bottomY := 50.0
	pb.DrawText("Payment is due within 30 days from date of invoice. Late payment is subject to fees of 5% per month.", margin, bottomY, builder.TextOptions{FontSize: 9, Color: textDark})
	pb.DrawText("Thanks for choosing DEMO - Sliced Invoices | admin@slicedinvoices.com", margin, bottomY-12, builder.TextOptions{FontSize: 9, Color: primaryBlue})
	pb.DrawText("Page 1/1", margin, bottomY-24, builder.TextOptions{FontSize: 9, Color: textGrey})

	pb.Finish()

	doc, err := b.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build doc: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	w := (&writer.WriterBuilder{}).Build()
	cfg := writer.Config{Deterministic: true, Linearize: true}
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

func solidLogo(w, h int, c builder.Color) *semantic.Image {
	px := make([]byte, 0, w*h*3)
	r := byte(clamp01(c.R) * 255)
	g := byte(clamp01(c.G) * 255)
	b := byte(clamp01(c.B) * 255)
	for i := 0; i < w*h; i++ {
		px = append(px, r, g, b)
	}
	return &semantic.Image{
		Width:            w,
		Height:           h,
		ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
		BitsPerComponent: 8,
		Data:             px,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
