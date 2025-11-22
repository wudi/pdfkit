package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	b := builder.NewBuilder()

	// Load Font
	fontData, err := os.ReadFile("testdata/Rubik-Regular.ttf")
	if err != nil {
		log.Fatalf("Failed to read font file: %v", err)
	}
	b.RegisterTrueTypeFont("Rubik", fontData)

	// Page Setup
	width, height := 595.28, 841.89 // A4
	pb := b.NewPage(width, height)

	// --- Header ---
	pb.DrawRectangle(0, height-100, width, 100, builder.RectOptions{
		Fill:      true,
		FillColor: builder.Color{R: 0.1, G: 0.1, B: 0.3},
	})
	pb.DrawText("EXECUTIVE DASHBOARD", 40, height-60, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 30,
		Color:    builder.Color{R: 1, G: 1, B: 1},
	})
	pb.DrawText(fmt.Sprintf("Generated: %s", time.Now().Format("2006-01-02")), 40, height-85, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 12,
		Color:    builder.Color{R: 0.8, G: 0.8, B: 0.8},
	})

	// --- KPI Cards ---
	drawCard(pb, 40, height-200, 150, 80, "Revenue", "$52,400", builder.Color{R: 0.2, G: 0.6, B: 0.3})
	drawCard(pb, 220, height-200, 150, 80, "New Users", "1,240", builder.Color{R: 0.2, G: 0.4, B: 0.8})
	drawCard(pb, 400, height-200, 150, 80, "Growth", "+15%", builder.Color{R: 0.8, G: 0.4, B: 0.1})

	// --- Bar Chart ---
	chartY := height - 450
	pb.DrawText("Monthly Performance", 40, chartY+130, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 16,
		Color:    builder.Color{R: 0.2, G: 0.2, B: 0.2},
	})

	// Chart Axes
	pb.DrawLine(40, chartY, 40, chartY+100, builder.LineOptions{StrokeColor: builder.Color{R: 0, G: 0, B: 0}, LineWidth: 1})
	pb.DrawLine(40, chartY, 500, chartY, builder.LineOptions{StrokeColor: builder.Color{R: 0, G: 0, B: 0}, LineWidth: 1})

	// Bars
	data := []float64{40, 65, 80, 55, 90, 70}
	labels := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"}
	barWidth := 40.0
	gap := 30.0

	for i, val := range data {
		x := 60.0 + float64(i)*(barWidth+gap)
		pb.DrawRectangle(x, chartY, barWidth, val, builder.RectOptions{
			Fill:      true,
			FillColor: builder.Color{R: 0.3, G: 0.5, B: 0.7},
		})
		pb.DrawText(labels[i], x+5, chartY-15, builder.TextOptions{
			Font:     "Rubik",
			FontSize: 10,
		})
	}

	// --- Table ---
	tableY := chartY - 60
	pb.DrawText("Recent Transactions", 40, tableY, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 16,
		Color:    builder.Color{R: 0.2, G: 0.2, B: 0.2},
	})

	pb.DrawTable(builder.Table{
		Columns:    []float64{100, 200, 100, 100},
		HeaderRows: 1,
		Rows: []builder.TableRow{
			{Cells: []builder.TableCell{
				{Text: "ID", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
				{Text: "Description", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
				{Text: "Date", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
				{Text: "Amount", Font: "Rubik", BackgroundColor: builder.Color{R: 0.9, G: 0.9, B: 0.9}},
			}},
			{Cells: []builder.TableCell{
				{Text: "TXN-001", Font: "Rubik"},
				{Text: "Server Hosting", Font: "Rubik"},
				{Text: "2023-11-20", Font: "Rubik"},
				{Text: "$150.00", Font: "Rubik"},
			}},
			{Cells: []builder.TableCell{
				{Text: "TXN-002", Font: "Rubik"},
				{Text: "Domain Renewal", Font: "Rubik"},
				{Text: "2023-11-21", Font: "Rubik"},
				{Text: "$25.00", Font: "Rubik"},
			}},
			{Cells: []builder.TableCell{
				{Text: "TXN-003", Font: "Rubik"},
				{Text: "Software License", Font: "Rubik"},
				{Text: "2023-11-22", Font: "Rubik"},
				{Text: "$500.00", Font: "Rubik"},
			}},
		},
	}, builder.TableOptions{
		X: 40, Y: tableY - 20,
		RowHeight:   25,
		BorderColor: builder.Color{R: 0.8, G: 0.8, B: 0.8},
		BorderWidth: 0.5,
	})

	// --- Form Section ---
	formY := 150.0
	pb.DrawText("Manager Approval", 40, formY, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 14,
		Color:    builder.Color{R: 0.2, G: 0.2, B: 0.2},
	})

	// Text Field
	pb.DrawText("Comments:", 40, formY-30, builder.TextOptions{Font: "Rubik", FontSize: 12})
	pb.DrawRectangle(120, formY-40, 300, 30, builder.RectOptions{Stroke: true, StrokeColor: builder.Color{R: 0.6, G: 0.6, B: 0.6}})

	pb.AddFormField(&semantic.TextFormField{
		BaseFormField: semantic.BaseFormField{
			Name:  "Comments",
			Rect:  semantic.Rectangle{LLX: 120, LLY: formY - 40, URX: 420, URY: formY - 10},
			Flags: 0,
		},
		Value: "",
	})

	// Checkbox
	pb.DrawText("Approved:", 450, formY-30, builder.TextOptions{Font: "Rubik", FontSize: 12})
	pb.DrawRectangle(520, formY-40, 20, 20, builder.RectOptions{Stroke: true, StrokeColor: builder.Color{R: 0.6, G: 0.6, B: 0.6}})

	pb.AddFormField(&semantic.ButtonFormField{
		BaseFormField: semantic.BaseFormField{
			Name:  "Approved",
			Rect:  semantic.Rectangle{LLX: 520, LLY: formY - 40, URX: 540, URY: formY - 20},
			Flags: 0,
		},
		IsCheck: true,
		Checked: false,
		OnState: "Yes",
	})

	// --- Footer & Link ---
	pb.DrawLine(40, 50, 555, 50, builder.LineOptions{StrokeColor: builder.Color{R: 0.8, G: 0.8, B: 0.8}, LineWidth: 1})
	pb.DrawText("Generated by PDFKit - ", 40, 30, builder.TextOptions{Font: "Rubik", FontSize: 10, Color: builder.Color{R: 0.5, G: 0.5, B: 0.5}})

	// Link Annotation
	linkRect := semantic.Rectangle{LLX: 150, LLY: 20, URX: 250, URY: 40}
	pb.DrawText("Visit Repository", 150, 30, builder.TextOptions{Font: "Rubik", FontSize: 10, Color: builder.Color{R: 0, G: 0, B: 1}})
	pb.AddAnnotation(&semantic.LinkAnnotation{
		BaseAnnotation: semantic.BaseAnnotation{
			Subtype: "Link",
			RectVal: linkRect,
		},
		Action: &semantic.URIAction{
			URI: "https://github.com/wudi/pdfkit",
		},
	})

	pb.Finish()

	// Build and Write
	doc, err := b.Build()
	if err != nil {
		log.Fatalf("Failed to build PDF: %v", err)
	}

	f, err := os.Create("dashboard.pdf")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Println("Successfully created dashboard.pdf")
}

func drawCard(pb builder.PageBuilder, x, y, w, h float64, title, value string, color builder.Color) {
	// Background
	pb.DrawRectangle(x, y, w, h, builder.RectOptions{
		Fill:      true,
		FillColor: color,
	})

	// Title
	pb.DrawText(title, x+10, y+h-25, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 12,
		Color:    builder.Color{R: 1, G: 1, B: 1}, // White
	})

	// Value
	pb.DrawText(value, x+10, y+20, builder.TextOptions{
		Font:     "Rubik",
		FontSize: 24,
		Color:    builder.Color{R: 1, G: 1, B: 1}, // White
	})
}
