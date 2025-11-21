package fonts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseType1(t *testing.T) {
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

	font, err := ParseType1("TestFont", buf.Bytes())
	if err != nil {
		t.Fatalf("ParseType1 failed: %v", err)
	}

	if font.BaseFont != "TestFont" {
		t.Errorf("Expected BaseFont TestFont, got %s", font.BaseFont)
	}
	if font.Descriptor.ItalicAngle != -12 {
		t.Errorf("Expected ItalicAngle -12, got %f", font.Descriptor.ItalicAngle)
	}
	expectedBBox := [4]float64{-50, -200, 1000, 900}
	if font.Descriptor.FontBBox != expectedBBox {
		t.Errorf("Expected FontBBox %v, got %v", expectedBBox, font.Descriptor.FontBBox)
	}
	if font.Descriptor.Ascent != 900 {
		t.Errorf("Expected Ascent 900, got %f", font.Descriptor.Ascent)
	}
	if font.Descriptor.Descent != -200 {
		t.Errorf("Expected Descent -200, got %f", font.Descriptor.Descent)
	}
}
