package pdfua

import (
	"pdflib/compliance"
	"pdflib/ir/semantic"
)

type Level int

const (
	PDFUA1 Level = iota
)

func (l Level) String() string {
	switch l {
	case PDFUA1:
		return "PDF/UA-1"
	default:
		return "Unknown"
	}
}

type Enforcer interface {
	compliance.Validator
	Enforce(ctx compliance.Context, doc *semantic.Document, level Level) error
}

type enforcerImpl struct{}

func NewEnforcer() Enforcer { return &enforcerImpl{} }

func (e *enforcerImpl) Enforce(ctx compliance.Context, doc *semantic.Document, level Level) error {
	// 1. Set Marked=true
	doc.Marked = true

	// 2. Set Title if missing
	if doc.Info == nil {
		doc.Info = &semantic.DocumentInfo{}
	}
	if doc.Info.Title == "" {
		doc.Info.Title = "Untitled"
	}

	// 3. Set Lang if missing
	if doc.Lang == "" {
		doc.Lang = "en" // Default to English
	}

	// 4. Create StructTreeRoot if missing (empty)
	if doc.StructTree == nil {
		doc.StructTree = &semantic.StructureTree{
			Type: "StructTreeRoot",
		}
	}

	return nil
}

func (e *enforcerImpl) Validate(ctx compliance.Context, doc *semantic.Document) (*compliance.Report, error) {
	report := &compliance.Report{
		Standard:   "PDF/UA-1",
		Violations: []compliance.Violation{},
	}

	// 1. Tagged PDF (Marked=true and StructTree exists)
	if !doc.Marked {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA001",
			Description: "Document must be marked (MarkInfo dictionary with Marked=true)",
			Location:    "Catalog",
		})
	}
	if doc.StructTree == nil {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA002",
			Description: "Document must be tagged (StructTree missing)",
			Location:    "Catalog",
		})
	}

	// 2. Title required
	if doc.Info == nil || doc.Info.Title == "" {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA003",
			Description: "Document title is required",
			Location:    "Info Dictionary",
		})
	}

	// 3. Language required
	if doc.Lang == "" {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA004",
			Description: "Document language is required",
			Location:    "Catalog",
		})
	}

	// 4. Fonts must be embedded
	visitedFonts := make(map[*semantic.Font]bool)
	for i, p := range doc.Pages {
		if p.Resources == nil {
			continue
		}
		for name, font := range p.Resources.Fonts {
			if visitedFonts[font] {
				continue
			}
			visitedFonts[font] = true
			if !isFontEmbedded(font) {
				report.Violations = append(report.Violations, compliance.Violation{
					Code:        "UA005",
					Description: "Font must be embedded: " + font.BaseFont,
					Location:    "Page " + string(rune(i+1)) + " Resource " + name,
				})
			}
		}
	}

	// 5. Check Structure (Alt text for Figures)
	if doc.StructTree != nil {
		checkStructure(doc.StructTree.K, report)
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}

func checkStructure(elements []*semantic.StructureElement, report *compliance.Report) {
	for _, elem := range elements {
		if elem == nil {
			continue
		}
		// Check Figure Alt text
		if elem.S == "Figure" && elem.Alt == "" {
			report.Violations = append(report.Violations, compliance.Violation{
				Code:        "UA006",
				Description: "Figure missing Alternative Text",
				Location:    "StructElem " + elem.S,
			})
		}

		// Recurse
		var children []*semantic.StructureElement
		for _, item := range elem.K {
			if item.Element != nil {
				children = append(children, item.Element)
			}
		}
		checkStructure(children, report)
	}
}

func isFontEmbedded(f *semantic.Font) bool {
	if f == nil {
		return false
	}
	if f.Subtype == "Type3" {
		return true
	}
	if f.Descriptor != nil && len(f.Descriptor.FontFile) > 0 {
		return true
	}
	if f.Subtype == "Type0" && f.DescendantFont != nil {
		if f.DescendantFont.Descriptor != nil && len(f.DescendantFont.Descriptor.FontFile) > 0 {
			return true
		}
	}
	return false
}
