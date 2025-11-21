package streaming_test

import (
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/streaming"
)

func TestParsePatternColorSpace(t *testing.T) {
	doc := &raw.Document{}

	// Case 1: Colored Pattern [/Pattern]
	coloredPatternArr := &raw.ArrayObj{
		Items: []raw.Object{
			raw.NameObj{Val: "Pattern"},
		},
	}

	cs := streaming.ParseColorSpace(doc, coloredPatternArr)
	if cs == nil {
		t.Fatal("Expected PatternColorSpace, got nil")
	}
	pcs, ok := cs.(*semantic.PatternColorSpace)
	if !ok {
		t.Fatalf("Expected *semantic.PatternColorSpace, got %T", cs)
	}
	if pcs.ColorSpaceName() != "Pattern" {
		t.Errorf("Expected name 'Pattern', got %s", pcs.ColorSpaceName())
	}
	if pcs.Underlying != nil {
		t.Error("Expected nil Underlying for colored pattern")
	}

	// Case 2: Uncolored Pattern [/Pattern /DeviceRGB]
	uncoloredPatternArr := &raw.ArrayObj{
		Items: []raw.Object{
			raw.NameObj{Val: "Pattern"},
			raw.NameObj{Val: "DeviceRGB"},
		},
	}

	cs = streaming.ParseColorSpace(doc, uncoloredPatternArr)
	if cs == nil {
		t.Fatal("Expected PatternColorSpace, got nil")
	}
	pcs, ok = cs.(*semantic.PatternColorSpace)
	if !ok {
		t.Fatalf("Expected *semantic.PatternColorSpace, got %T", cs)
	}
	if pcs.Underlying == nil {
		t.Fatal("Expected Underlying color space, got nil")
	}
	devCS, ok := pcs.Underlying.(*semantic.DeviceColorSpace)
	if !ok {
		t.Fatalf("Expected *semantic.DeviceColorSpace as underlying, got %T", pcs.Underlying)
	}
	if devCS.Name != "DeviceRGB" {
		t.Errorf("Expected DeviceRGB, got %s", devCS.Name)
	}
}

func TestParseOtherColorSpaces(t *testing.T) {
	doc := &raw.Document{}

	// Case 1: Separation [/Separation /Logo /DeviceRGB <Function>]
	sepArr := &raw.ArrayObj{
		Items: []raw.Object{
			raw.NameObj{Val: "Separation"},
			raw.NameObj{Val: "Logo"},
			raw.NameObj{Val: "DeviceRGB"},
			raw.NullObj{}, // Placeholder for function
		},
	}
	cs := streaming.ParseColorSpace(doc, sepArr)
	if _, ok := cs.(*semantic.SeparationColorSpace); !ok {
		t.Errorf("Expected SeparationColorSpace, got %T", cs)
	}

	// Case 2: DeviceN [/DeviceN [/Cyan /Magenta] /DeviceCMYK <Function>]
	devNArr := &raw.ArrayObj{
		Items: []raw.Object{
			raw.NameObj{Val: "DeviceN"},
			&raw.ArrayObj{Items: []raw.Object{raw.NameObj{Val: "Cyan"}, raw.NameObj{Val: "Magenta"}}},
			raw.NameObj{Val: "DeviceCMYK"},
			raw.NullObj{},
		},
	}
	cs = streaming.ParseColorSpace(doc, devNArr)
	if _, ok := cs.(*semantic.DeviceNColorSpace); !ok {
		t.Errorf("Expected DeviceNColorSpace, got %T", cs)
	}

	// Case 3: Indexed [/Indexed /DeviceRGB 255 <Lookup>]
	idxArr := &raw.ArrayObj{
		Items: []raw.Object{
			raw.NameObj{Val: "Indexed"},
			raw.NameObj{Val: "DeviceRGB"},
			raw.NumberObj{I: 255, IsInt: true},
			raw.StringObj{Bytes: []byte("lookup")},
		},
	}
	cs = streaming.ParseColorSpace(doc, idxArr)
	if _, ok := cs.(*semantic.IndexedColorSpace); !ok {
		t.Errorf("Expected IndexedColorSpace, got %T", cs)
	}
}
