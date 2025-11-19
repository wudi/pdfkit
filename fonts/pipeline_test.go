package fonts

import (
	"os"
	"testing"

	"pdflib/ir/semantic"
)

func TestSubsettingPipeline_Integration(t *testing.T) {
	// Load a real font file
	fontData, err := os.ReadFile("../testdata/Rubik-Regular.ttf")
	if err != nil {
		t.Skip("Rubik-Regular.ttf not found, skipping test")
	}

	// Create a semantic font
	font := &semantic.Font{
		Subtype:  "TrueType",
		BaseFont: "Rubik-Regular",
		Encoding: "WinAnsiEncoding", // Simple encoding for test
		Descriptor: &semantic.FontDescriptor{
			FontFile:     fontData,
			FontFileType: "FontFile2",
		},
		// Minimal ToUnicode for "f" and "i" mapping
		ToUnicode: map[int][]rune{
			'f': {'f'},
			'i': {'i'},
		},
	}

	// Create a document with one page using this font
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": font,
					},
				},
				Contents: []semantic.ContentStream{
					{
						Operations: []semantic.Operation{
							{
								Operator: "BT",
							},
							{
								Operator: "Tf",
								Operands: []semantic.Operand{
									semantic.NameOperand{Value: "F1"},
									semantic.NumberOperand{Value: 12},
								},
							},
							{
								Operator: "Tj",
								Operands: []semantic.Operand{
									semantic.StringOperand{Value: []byte("fi")},
								},
							},
							{
								Operator: "ET",
							},
						},
					},
				},
			},
		},
	}

	// 1. Analyze
	analyzer := NewAnalyzer()
	analyzer.Analyze(doc)

	if len(analyzer.UsedGlyphs[font]) == 0 {
		t.Fatal("Analyzer failed to find used glyphs")
	}
	if !analyzer.UsedGlyphs[font]['f'] || !analyzer.UsedGlyphs[font]['i'] {
		t.Error("Analyzer missed basic glyphs 'f' or 'i'")
	}

	// 2. Plan (Shaping + Closure)
	planner := NewPlanner()
	planner.Plan(analyzer)

	subset := planner.Subsets[font]
	if subset == nil {
		t.Fatal("Planner failed to create subset")
	}

	// Check if we have more glyphs than just 'f' and 'i' (and .notdef)
	// If 'fi' ligature is present and triggered by shaping or closure, count should be higher.
	// Note: 'f' is 102, 'i' is 105 in ASCII.
	// We expect at least 3 glyphs (0, f, i).
	// If ligature is found, we might see another one.

	t.Logf("Subset has %d glyphs", len(subset.GlyphSet))
	for gid := range subset.GlyphSet {
		t.Logf("  GID: %d", gid)
	}

	// 3. Apply (Subset generation)
	subsetter := NewSubsetter()
	subsetter.Apply(doc, planner)

	if len(font.Descriptor.FontFile) >= len(fontData) {
		// It might not be strictly smaller if the subset is almost the whole font or overhead,
		// but for "fi" it should be much smaller than a full font.
		t.Errorf("Subsetted font size (%d) not smaller than original (%d)", len(font.Descriptor.FontFile), len(fontData))
	}
}
