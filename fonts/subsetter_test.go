package fonts

import (
	"os"
	"testing"
)

func TestSubsetTrueType(t *testing.T) {
	// Load a real font file from testdata
	fontData, err := os.ReadFile("../testdata/Rubik-Regular.ttf")
	if err != nil {
		t.Skip("Rubik-Regular.ttf not found, skipping test")
	}

	// Define used GIDs (arbitrary set)
	usedGIDs := map[int]bool{
		0:  true, // .notdef
		36: true, // A
		37: true, // B
		38: true, // C
	}

	subsetData, err := SubsetTrueType(fontData, usedGIDs)
	if err != nil {
		t.Fatalf("SubsetTrueType failed: %v", err)
	}

	if len(subsetData) == 0 {
		t.Fatal("Subset data is empty")
	}

	if len(subsetData) >= len(fontData) {
		t.Errorf("Subset data size (%d) is not smaller than original (%d)", len(subsetData), len(fontData))
	}

	// Basic validation of the subsetted font
	p := &ttParser{data: subsetData}
	if err := p.ParseDirectory(); err != nil {
		t.Fatalf("Failed to parse subsetted font directory: %v", err)
	}

	if !p.HasTable("glyf") {
		t.Error("Subsetted font missing glyf table")
	}
	if !p.HasTable("loca") {
		t.Error("Subsetted font missing loca table")
	}
	if !p.HasTable("head") {
		t.Error("Subsetted font missing head table")
	}
}
