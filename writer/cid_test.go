package writer

import (
	"bytes"
	"context"
	"pdflib/ir/semantic"
	"strings"
	"testing"
)

func TestCIDFontEmbedding(t *testing.T) {
	// Manually construct a Type 0 (Composite) Font
	cidInfo := semantic.CIDSystemInfo{
		Registry:   "Adobe",
		Ordering:   "Identity",
		Supplement: 0,
	}

	descendant := &semantic.CIDFont{
		Subtype:       "CIDFontType2",
		BaseFont:      "TestCIDFont",
		CIDSystemInfo: cidInfo,
		DW:            1000,
		W: map[int]int{
			10: 500,
			11: 500,
			12: 500,
			20: 300,
		},
		Descriptor: &semantic.FontDescriptor{
			FontName: "TestCIDFont",
			Flags:    4,
			FontBBox: [4]float64{0, 0, 1000, 1000},
		},
	}

	font := &semantic.Font{
		Subtype:        "Type0",
		BaseFont:       "TestCIDFont-Identity-H",
		Encoding:       "Identity-H",
		DescendantFont: descendant,
		ToUnicode: map[int][]rune{
			10: {'A'},
			11: {'B'},
			12: {'C'},
			20: {' '},
		},
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": font,
					},
				},
				Contents: []semantic.ContentStream{
					{RawBytes: []byte("/F1 12 Tf <000A000B000C> Tj")},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := NewWriter()
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	// Verify Type 0 Font
	if !strings.Contains(output, "/Type /Font") {
		t.Error("Missing Font object")
	}
	if !strings.Contains(output, "/Subtype /Type0") {
		t.Error("Missing Subtype Type0")
	}
	if !strings.Contains(output, "/BaseFont /TestCIDFont-Identity-H") {
		t.Error("Missing BaseFont")
	}
	if !strings.Contains(output, "/Encoding /Identity-H") {
		t.Error("Missing Encoding")
	}
	if !strings.Contains(output, "/DescendantFonts [") {
		t.Error("Missing DescendantFonts")
	}
	if !strings.Contains(output, "/ToUnicode") {
		t.Error("Missing ToUnicode")
	}

	// Verify Descendant CIDFont
	if !strings.Contains(output, "/Subtype /CIDFontType2") {
		t.Error("Missing CIDFontType2")
	}
	if !strings.Contains(output, "/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>") {
		t.Error("Missing/Incorrect CIDSystemInfo")
	}
	if !strings.Contains(output, "/DW 1000") {
		t.Error("Missing DW")
	}

	// Verify W array
	// 10, 11, 12 have width 500. 20 has width 300.
	// encodeCIDWidths logic:
	// It groups consecutive CIDs with same width.
	// 10-12: 500 -> 10 12 500
	// 20: 300 -> 20 20 300
	if !strings.Contains(output, "/W [10 12 500 20 20 300]") {
		t.Errorf("Incorrect W array. Expected [10 12 500 20 20 300], got something else in output")
	}

	// Verify ToUnicode CMap
	// We can't easily check the stream content without decompressing or parsing,
	// but we can check if the stream object exists and has correct length/filter if applicable.
	// The writer uses FlateDecode by default if compression is on, but here we used default config (Compression 0?).
	// Config{} has Compression 0.
	// So it should be plain text.
	if !strings.Contains(output, "/CIDInit /ProcSet findresource begin") {
		t.Error("ToUnicode CMap content missing or compressed")
	}
	if !strings.Contains(output, "beginbfchar") {
		t.Error("ToUnicode missing beginbfchar")
	}
	if !strings.Contains(output, "<000A> <0041>") { // 10 -> A
		t.Error("ToUnicode mapping 10->A missing")
	}
}
