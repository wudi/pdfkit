package writer

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/fonts"
	"github.com/wudi/pdfkit/ir/semantic"
)

func TestType1Embedding(t *testing.T) {
	// Create a mock PFB file
	asciiData := []byte(`%!PS-AdobeFont-1.0: TestFont 1.0
%%Title: TestFont
/FontName /TestFont def
/ItalicAngle -12 def
/FontBBox {-50 -200 1000 900} readonly def
currentdict end
currentfile eexec
`)
	binaryData := []byte{0xDE, 0xAD, 0xBE, 0xEF} // Mock encrypted data
	trailerData := []byte("0000000000000000000000000000000000000000000000000000000000000000\ncleartomark")

	var buf bytes.Buffer

	// Segment 1: ASCII
	buf.WriteByte(0x80)
	buf.WriteByte(0x01)
	binary.Write(&buf, binary.LittleEndian, uint32(len(asciiData)))
	buf.Write(asciiData)

	// Segment 2: Binary
	buf.WriteByte(0x80)
	buf.WriteByte(0x02)
	binary.Write(&buf, binary.LittleEndian, uint32(len(binaryData)))
	buf.Write(binaryData)

	// Segment 3: ASCII (Trailer)
	buf.WriteByte(0x80)
	buf.WriteByte(0x01)
	binary.Write(&buf, binary.LittleEndian, uint32(len(trailerData)))
	buf.Write(trailerData)

	// Segment 4: EOF
	buf.WriteByte(0x80)
	buf.WriteByte(0x03)

	pfbData := buf.Bytes()
	font, err := fonts.ParseType1("TestFont", pfbData)
	if err != nil {
		t.Fatalf("ParseType1 failed: %v", err)
	}

	// Manually set some widths for testing
	font.Widths = map[int]int{
		'A': 600,
		'B': 620,
	}

	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 595, URY: 842},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": font,
					},
				},
				Contents: []semantic.ContentStream{
					{
						Operations: []semantic.Operation{
							{Operator: "BT"},
							{Operator: "Tf", Operands: []semantic.Operand{semantic.NameOperand{Value: "F1"}, semantic.NumberOperand{Value: 12}}},
							{Operator: "Td", Operands: []semantic.Operand{semantic.NumberOperand{Value: 100}, semantic.NumberOperand{Value: 700}}},
							{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte("AB")}}},
							{Operator: "ET"},
						},
					},
				},
			},
		},
	}

	var out bytes.Buffer
	w := NewWriter()
	err = w.Write(context.Background(), doc, &out, Config{})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := out.String()

	// Verify Font object
	if !strings.Contains(output, "/Type /Font") {
		t.Error("Output missing Font object")
	}
	if !strings.Contains(output, "/Subtype /Type1") {
		t.Error("Output missing Subtype Type1")
	}
	if !strings.Contains(output, "/BaseFont /TestFont") {
		t.Error("Output missing BaseFont TestFont")
	}
	if !strings.Contains(output, "/FirstChar 65") { // 'A'
		t.Error("Output missing FirstChar 65")
	}
	if !strings.Contains(output, "/LastChar 66") { // 'B'
		t.Error("Output missing LastChar 66")
	}
	if !strings.Contains(output, "/Widths [600 620]") {
		t.Error("Output missing Widths [600 620]")
	}

	// Verify FontDescriptor
	if !strings.Contains(output, "/Type /FontDescriptor") {
		t.Error("Output missing FontDescriptor object")
	}
	if !strings.Contains(output, "/FontName /TestFont") {
		t.Error("Output missing FontName in descriptor")
	}
	if !strings.Contains(output, "/FontFile") {
		t.Error("Output missing FontFile stream reference")
	}
	// Check for FontBBox
	if !strings.Contains(output, "/FontBBox [-50 -200 1000 900]") {
		t.Error("Output missing FontBBox or incorrect format")
	}
}
