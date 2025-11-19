package pdfa

import (
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
)

// Level represents a PDF/A conformance level shared across writer and enforcer.
type Level int

const (
	PDFA1B Level = iota
	PDFA3B
)

// PDFALevel is kept for compatibility; use Level instead.
type PDFALevel = Level

type Violation struct{ Code, Description, Location string }

type ComplianceReport struct {
	Compliant  bool
	Level      Level
	Violations []Violation
}

type Enforcer interface {
	Enforce(ctx Context, doc *semantic.Document, level Level) error
	Validate(ctx Context, doc *semantic.Document, level Level) (*ComplianceReport, error)
}

type enforcerImpl struct{}

func NewEnforcer() Enforcer { return &enforcerImpl{} }

func (e *enforcerImpl) Enforce(ctx Context, doc *semantic.Document, level Level) error {
	// 1. Remove encryption
	if doc.Encrypted {
		doc.Encrypted = false
		doc.Permissions = raw.Permissions{}
		doc.OwnerPassword = ""
		doc.UserPassword = ""
	}

	// 2. Add OutputIntent if missing
	if len(doc.OutputIntents) == 0 {
		doc.OutputIntents = []semantic.OutputIntent{
			{
				S:                         "GTS_PDFA1",
				OutputConditionIdentifier: "sRGB IEC61966-2.1",
				Info:                      "sRGB IEC61966-2.1",
				// Note: A real implementation should embed a valid ICC profile here.
				DestOutputProfile: nil,
			},
		}
	}

	// 3. Remove forbidden annotations
	for _, p := range doc.Pages {
		if err := checkCancelled(ctx); err != nil {
			return err
		}
		if len(p.Annotations) == 0 {
			continue
		}
		var validAnnots []semantic.Annotation
		for _, annot := range p.Annotations {
			if !isForbiddenAnnotation(annot) {
				validAnnots = append(validAnnots, annot)
			}
		}
		p.Annotations = validAnnots
	}

	return nil
}

func (e *enforcerImpl) Validate(ctx Context, doc *semantic.Document, level Level) (*ComplianceReport, error) {
	report := &ComplianceReport{
		Level:      level,
		Violations: []Violation{},
	}

	// 1. Encryption forbidden
	if doc.Encrypted {
		report.Violations = append(report.Violations, Violation{
			Code:        "ENC001",
			Description: "Encryption is forbidden in PDF/A",
			Location:    "Document",
		})
	}

	// 2. OutputIntent required
	if len(doc.OutputIntents) == 0 {
		report.Violations = append(report.Violations, Violation{
			Code:        "INT001",
			Description: "OutputIntent is required",
			Location:    "Catalog",
		})
	}

	// 3. Font embedding required
	// We need to check all fonts used in the document.
	// Since fonts are stored in Page Resources, we iterate pages.
	// To avoid duplicate checks, we track visited fonts by pointer or name.
	visitedFonts := make(map[*semantic.Font]bool)

	for i, p := range doc.Pages {
		if err := checkCancelled(ctx); err != nil {
			return nil, err
		}
		if p.Resources == nil {
			continue
		}
		for name, font := range p.Resources.Fonts {
			if visitedFonts[font] {
				continue
			}
			visitedFonts[font] = true

			if !isFontEmbedded(font) {
				report.Violations = append(report.Violations, Violation{
					Code:        "FNT001",
					Description: "Font must be embedded: " + font.BaseFont,
					Location:    "Page " + string(rune(i+1)) + " Resource " + name,
				})
			}
		}

		// 4. Forbidden Actions (Launch, Sound, Movie, JavaScript)
		// Check Annotations
		for _, annot := range p.Annotations {
			if isForbiddenAnnotation(annot) {
				report.Violations = append(report.Violations, Violation{
					Code:        "ACT001",
					Description: "Forbidden annotation type or action: " + annot.Subtype,
					Location:    "Page " + string(rune(i+1)),
				})
			}
		}
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}

func isFontEmbedded(f *semantic.Font) bool {
	if f == nil {
		return false
	}
	// Type3 fonts are defined by streams in the PDF, effectively embedded.
	if f.Subtype == "Type3" {
		return true
	}
	// Standard 14 fonts must also be embedded in PDF/A.
	if f.Descriptor != nil && len(f.Descriptor.FontFile) > 0 {
		return true
	}
	// Check descendant for Type0
	if f.Subtype == "Type0" && f.DescendantFont != nil {
		if f.DescendantFont.Descriptor != nil && len(f.DescendantFont.Descriptor.FontFile) > 0 {
			return true
		}
	}
	return false
}

func isForbiddenAnnotation(a semantic.Annotation) bool {
	switch a.Subtype {
	case "Movie", "Sound", "Screen", "3D":
		return true
	}
	// Check for JavaScript in URI (simplified check)
	// Real implementation would check the Action dictionary
	if len(a.URI) > 11 && a.URI[:11] == "javascript:" {
		return true
	}
	return false
}

func checkCancelled(ctx Context) error {
	select {
	case <-ctx.Done():
		return &ValidationCancelledError{}
	default:
		return nil
	}
}

type ValidationCancelledError struct{}

func (e *ValidationCancelledError) Error() string { return "validation cancelled" }

type Context interface{ Done() <-chan struct{} }
