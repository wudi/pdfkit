package fonts

import (
	"sort"

	"pdflib/ir/semantic"
)

type Subset struct {
	OriginalToSubset map[int]int // Map original CID -> new CID (identity for now)
	SubsetToOriginal map[int]int // Map new CID -> original CID
	UsedCIDs         []int       // List of used CIDs
	GlyphSet         map[int]bool
}

type Planner struct {
	Subsets map[*semantic.Font]*Subset
}

func NewPlanner() *Planner {
	return &Planner{
		Subsets: make(map[*semantic.Font]*Subset),
	}
}

func (p *Planner) Plan(analyzer *Analyzer) {
	for font, used := range analyzer.UsedGlyphs {
		glyphSet := make(map[int]bool)
		for cid := range used {
			glyphSet[cid] = true
		}
		if shaped := shapeRunsForFont(font, analyzer.TextRuns[font]); len(shaped) > 0 {
			for gid := range shaped {
				glyphSet[gid] = true
			}
		}

		subset := &Subset{
			OriginalToSubset: make(map[int]int),
			SubsetToOriginal: make(map[int]int),
			GlyphSet:         glyphSet,
		}

		// For now, we keep original CIDs (Identity mapping).
		// A real subsetter would renumber them to 0..N to save space.
		for cid := range glyphSet {
			subset.OriginalToSubset[cid] = cid
			subset.SubsetToOriginal[cid] = cid
			subset.UsedCIDs = append(subset.UsedCIDs, cid)
		}
		sort.Ints(subset.UsedCIDs)

		p.Subsets[font] = subset
	}
}

// Subsetter applies the subsetting plan to the fonts.
type Subsetter struct{}

func NewSubsetter() *Subsetter {
	return &Subsetter{}
}

func (s *Subsetter) Apply(doc *semantic.Document, planner *Planner) {
	for font, subset := range planner.Subsets {
		// 1. Filter Widths
		newWidths := make(map[int]int)
		for _, cid := range subset.UsedCIDs {
			if w, ok := font.Widths[cid]; ok {
				newWidths[cid] = w
			} else {
				// Use default width if available in descendant
				if font.DescendantFont != nil {
					newWidths[cid] = font.DescendantFont.DW
				}
			}
		}
		font.Widths = newWidths
		if font.DescendantFont != nil {
			font.DescendantFont.W = newWidths
		}

		// 2. Filter ToUnicode
		if font.ToUnicode != nil {
			newToUnicode := make(map[int][]rune)
			for _, cid := range subset.UsedCIDs {
				if r, ok := font.ToUnicode[cid]; ok {
					newToUnicode[cid] = r
				}
			}
			font.ToUnicode = newToUnicode
		}

		// 3. Subset FontFile
		if font.Descriptor != nil && len(font.Descriptor.FontFile) > 0 && font.Descriptor.FontFileType == "FontFile2" {
			// Identity-H means CID=GID, so the glyph set directly maps to TrueType glyph IDs.
			usedGIDs := make(map[int]bool, len(subset.GlyphSet))
			for gid := range subset.GlyphSet {
				usedGIDs[gid] = true
			}

			newFontData, err := SubsetTrueType(font.Descriptor.FontFile, usedGIDs)
			if err == nil && len(newFontData) < len(font.Descriptor.FontFile) {
				font.Descriptor.FontFile = newFontData
			}
		}
	}
}
