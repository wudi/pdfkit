package fonts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseOpenTypeTableDirectory(t *testing.T) {
	var buf bytes.Buffer
	// Offset Table
	binary.Write(&buf, binary.BigEndian, uint32(0x4F54544F)) // OTTO
	binary.Write(&buf, binary.BigEndian, uint16(1))          // NumTables
	binary.Write(&buf, binary.BigEndian, uint16(0))          // SearchRange
	binary.Write(&buf, binary.BigEndian, uint16(0))          // EntrySelector
	binary.Write(&buf, binary.BigEndian, uint16(0))          // RangeShift

	// Table Record
	buf.WriteString("CFF ")                                  // Tag
	binary.Write(&buf, binary.BigEndian, uint32(0x12345678)) // CheckSum
	binary.Write(&buf, binary.BigEndian, uint32(30))         // Offset
	binary.Write(&buf, binary.BigEndian, uint32(100))        // Length

	// Data (padding to reach offset 30)
	padding := make([]byte, 30-buf.Len())
	buf.Write(padding)

	// Table Data
	buf.Write(make([]byte, 100))

	tables, err := ParseOpenTypeTableDirectory(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseOpenTypeTableDirectory failed: %v", err)
	}

	if len(tables) != 1 {
		t.Fatalf("Expected 1 table, got %d", len(tables))
	}

	cff, ok := tables["CFF "]
	if !ok {
		t.Fatal("Expected CFF table")
	}
	if cff.Offset != 30 {
		t.Errorf("Expected offset 30, got %d", cff.Offset)
	}
	if cff.Length != 100 {
		t.Errorf("Expected length 100, got %d", cff.Length)
	}
}
