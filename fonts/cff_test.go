package fonts

import (
	"bytes"
	"testing"
)

func TestParseCFF(t *testing.T) {
	// Construct a minimal CFF
	// Header
	var buf bytes.Buffer
	buf.Write([]byte{1, 0, 4, 1}) // Major, Minor, HdrSize, OffSize

	// Name INDEX
	// Count=1, OffSize=1, Offsets=[1, 5], Data="Test"
	buf.Write([]byte{0, 1}) // Count
	buf.WriteByte(1)        // OffSize
	buf.WriteByte(1)        // Offset 1
	buf.WriteByte(5)        // Offset 2
	buf.WriteString("Test") // Data

	// Top DICT INDEX
	// Count=1, OffSize=1, Offsets=[1, 3], Data=[Op 123]
	buf.Write([]byte{0, 1}) // Count
	buf.WriteByte(1)        // OffSize
	buf.WriteByte(1)        // Offset 1
	buf.WriteByte(3)        // Offset 2
	// Dict data: 123 (integer)
	// 123 = 139 + (123-139) -> No, 123 is in range 32-246? No.
	// 32-246 -> val-139. 123 is < 139.
	// 0-21 operators.
	// Let's use short int format: 32-246.
	// Value 100: 100 + 139 = 239. Byte 239 -> 239-139 = 100.
	buf.WriteByte(239) // Int 100
	buf.WriteByte(14)  // Operator 14 (FontName? No, just a test op)

	// String INDEX (Empty)
	buf.Write([]byte{0, 0}) // Count 0

	// Global Subr INDEX (Empty)
	// Note: ParseCFF doesn't read Global Subr yet in my implementation, 
	// but usually it follows String INDEX.
	// My implementation stopped after String INDEX.

	cff, err := ParseCFF(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if len(cff.Names) != 1 || cff.Names[0] != "Test" {
		t.Errorf("Expected name Test, got %v", cff.Names)
	}

	if len(cff.TopDicts) != 1 {
		t.Fatalf("Expected 1 Top Dict, got %d", len(cff.TopDicts))
	}

	ops := cff.TopDicts[0][14]
	if len(ops) != 1 || ops[0].Int != 100 {
		t.Errorf("Expected operand 100 for op 14, got %v", ops)
	}
}
