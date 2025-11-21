package pdfa

import (
	"github.com/wudi/pdfkit/cmm"
	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

// Level represents a PDF/A conformance level shared across writer and enforcer.
type Level int

const (
	PDFA1B Level = iota
	PDFA2B
	PDFA2U
	PDFA3B
	PDFA3U
	PDFA4
	PDFA4E
	PDFA4F
)

func (l Level) String() string {
	switch l {
	case PDFA1B:
		return "PDF/A-1b"
	case PDFA2B:
		return "PDF/A-2b"
	case PDFA2U:
		return "PDF/A-2u"
	case PDFA3B:
		return "PDF/A-3b"
	case PDFA3U:
		return "PDF/A-3u"
	case PDFA4:
		return "PDF/A-4"
	case PDFA4E:
		return "PDF/A-4e"
	case PDFA4F:
		return "PDF/A-4f"
	default:
		return "Unknown"
	}
}

// IsLevelA1 returns true if the level is PDF/A-1.
func (l Level) IsLevelA1() bool {
	return l == PDFA1B
}

// IsLevelA2 returns true if the level is PDF/A-2.
func (l Level) IsLevelA2() bool {
	return l == PDFA2B || l == PDFA2U
}

// IsLevelA3 returns true if the level is PDF/A-3.
func (l Level) IsLevelA3() bool {
	return l == PDFA3B || l == PDFA3U
}

// IsLevelA4 returns true if the level is PDF/A-4.
func (l Level) IsLevelA4() bool {
	return l == PDFA4 || l == PDFA4E || l == PDFA4F
}

// AllowsTransparency returns true if the level allows transparency (A-2+).
func (l Level) AllowsTransparency() bool {
	return !l.IsLevelA1()
}

// AllowsLayers returns true if the level allows optional content (A-2+).
func (l Level) AllowsLayers() bool {
	return !l.IsLevelA1()
}

// AllowsAttachment returns true if the level allows file attachments.
// A-1: No. A-2: Yes (PDF/A). A-3: Yes (Any). A-4: Yes (Any/PDF/A depending on f/e).
func (l Level) AllowsAttachment() bool {
	return !l.IsLevelA1()
}

// AllowsArbitraryAttachment returns true if the level allows non-PDF/A attachments.
func (l Level) AllowsArbitraryAttachment() bool {
	return l.IsLevelA3() || l == PDFA4 || l == PDFA4F
}

// PDFALevel is kept for compatibility; use Level instead.
type PDFALevel = Level

type Enforcer interface {
	Enforce(ctx compliance.Context, doc *semantic.Document, level Level) error
	Validate(ctx compliance.Context, doc *semantic.Document, level Level) (*compliance.Report, error)
}

type enforcerImpl struct{}

func NewEnforcer() Enforcer { return &enforcerImpl{} }

func (e *enforcerImpl) Enforce(ctx compliance.Context, doc *semantic.Document, level Level) error {
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
				DestOutputProfile:         DefaultICCProfile,
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
			if !isForbiddenAnnotation(annot, level) {
				validAnnots = append(validAnnots, annot)
			}
		}
		p.Annotations = validAnnots
	}

	return nil
}

func (e *enforcerImpl) Validate(ctx compliance.Context, doc *semantic.Document, level Level) (*compliance.Report, error) {
	report := &compliance.Report{
		Standard:   level.String(),
		Violations: []compliance.Violation{},
	}

	// 1. Encryption forbidden
	if doc.Encrypted {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "ENC001",
			Description: "Encryption is forbidden in PDF/A",
			Location:    "Document",
		})
	}

	// 2. OutputIntent required
	if len(doc.OutputIntents) == 0 {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "INT001",
			Description: "OutputIntent is required",
			Location:    "Catalog",
		})
	} else {
		// Validate OutputIntent profile
		for _, intent := range doc.OutputIntents {
			if intent.DestOutputProfile != nil {
				if _, err := cmm.NewICCProfile(intent.DestOutputProfile); err != nil {
					report.Violations = append(report.Violations, compliance.Violation{
						Code:        "INT002",
						Description: "Invalid OutputIntent ICC profile: " + err.Error(),
						Location:    "OutputIntent",
					})
				}
			}
		}
	}

	// 3. Font embedding required
	visitedFonts := make(map[*semantic.Font]bool)

	// 4. Transparency and Layers (if forbidden)
	checkTransparency := !level.AllowsTransparency()
	checkLayers := !level.AllowsLayers()

	for i, p := range doc.Pages {
		if err := checkCancelled(ctx); err != nil {
			return nil, err
		}
		pageLoc := "Page " + string(rune(i+1))

		if p.Resources != nil {
			// Check Fonts
			for name, font := range p.Resources.Fonts {
				if visitedFonts[font] {
					continue
				}
				visitedFonts[font] = true

				if !isFontEmbedded(font) {
					report.Violations = append(report.Violations, compliance.Violation{
						Code:        "FNT001",
						Description: "Font must be embedded: " + font.BaseFont,
						Location:    pageLoc + " Resource " + name,
					})
				}
			}

			// Check Transparency in ExtGState
			if checkTransparency {
				for name, gs := range p.Resources.ExtGStates {
					if isTransparent(gs) {
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "TRN001",
							Description: "Transparency is forbidden in " + level.String(),
							Location:    pageLoc + " ExtGState " + name,
						})
					}
				}
			}

			// Check Layers (Optional Content)
			if checkLayers {
				for name, prop := range p.Resources.Properties {
					if _, ok := prop.(*semantic.OptionalContentGroup); ok {
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "LYR001",
							Description: "Optional Content (Layers) is forbidden in " + level.String(),
							Location:    pageLoc + " Property " + name,
						})
					}
					if _, ok := prop.(*semantic.OptionalContentMembership); ok {
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "LYR001",
							Description: "Optional Content (Layers) is forbidden in " + level.String(),
							Location:    pageLoc + " Property " + name,
						})
					}
				}
			}

			// Check XObjects for Transparency (SMask)
			if checkTransparency {
				for name, xobj := range p.Resources.XObjects {
					if xobj.SMask != nil {
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "TRN002",
							Description: "Image SMask (Transparency) is forbidden in " + level.String(),
							Location:    pageLoc + " XObject " + name,
						})
					}
					if xobj.Group != nil {
						// Transparency Group XObject
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "TRN003",
							Description: "Transparency Group XObject is forbidden in " + level.String(),
							Location:    pageLoc + " XObject " + name,
						})
					}
				}
			}
		}

		// 5. Annotations
		for _, annot := range p.Annotations {
			if isForbiddenAnnotation(annot, level) {
				report.Violations = append(report.Violations, compliance.Violation{
					Code:        "ACT001",
					Description: "Forbidden annotation type or action for " + level.String() + ": " + annot.Base().Subtype,
					Location:    pageLoc,
				})
			}
		}
	}

	// 6. Embedded Files
	if len(doc.EmbeddedFiles) > 0 {
		if !level.AllowsAttachment() {
			report.Violations = append(report.Violations, compliance.Violation{
				Code:        "ATT001",
				Description: "Embedded files are forbidden in " + level.String(),
				Location:    "Catalog",
			})
		} else if !level.AllowsArbitraryAttachment() {
			// Must check if attachments are PDF/A compliant
			for _, ef := range doc.EmbeddedFiles {
				if ef.Subtype != "application/pdf" {
					// If not PDF, it must be associated with a relationship (PDF/A-3)
					// But if we are in PDF/A-2, only PDF/A compliant PDFs are allowed.
					// Since we can't easily validate the embedded file recursively here without loading it,
					// we check for metadata or assume non-compliant if it's not PDF.
					if level.IsLevelA2() {
						report.Violations = append(report.Violations, compliance.Violation{
							Code:        "ATT002",
							Description: "Embedded file must be PDF/A compliant in " + level.String(),
							Location:    "EmbeddedFile " + ef.Name,
						})
					}
				}
				// Ideally, we would parse `ef.Data` as a PDF and validate it recursively.
				// For now, we flag non-PDF files in A-2.
			}
		}
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}

func isTransparent(gs semantic.ExtGState) bool {
	if gs.SoftMask != nil {
		return true
	}
	if gs.StrokeAlpha != nil && *gs.StrokeAlpha < 1.0 {
		return true
	}
	if gs.FillAlpha != nil && *gs.FillAlpha < 1.0 {
		return true
	}
	if gs.BlendMode != "" && gs.BlendMode != "Normal" && gs.BlendMode != "Compatible" {
		return true
	}
	return false
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

func isForbiddenAnnotation(a semantic.Annotation, level Level) bool {
	subtype := a.Base().Subtype

	// Common forbidden types for A-1
	if level.IsLevelA1() {
		switch subtype {
		case "Movie", "Sound", "Screen", "3D", "FileAttachment":
			return true
		}
	} else {
		// A-2+ allows more
		switch subtype {
		case "Movie", "Sound": // Still forbidden in A-2 (Screen used instead)
			return true
		}
	}

	// Check for JavaScript in URI (simplified check)
	if link, ok := a.(*semantic.LinkAnnotation); ok {
		if len(link.URI) > 11 && link.URI[:11] == "javascript:" {
			return true
		}
		if checkAction(link.Action, level) {
			return true
		}
	}

	if screen, ok := a.(*semantic.ScreenAnnotation); ok {
		if checkAction(screen.Action, level) {
			return true
		}
	}

	return false
}

func checkAction(a semantic.Action, level Level) bool {
	if a == nil {
		return false
	}
	switch a.ActionType() {
	case "Launch", "Sound", "Movie", "ResetForm", "ImportData", "JavaScript":
		return true
	}
	return false
}

func checkCancelled(ctx compliance.Context) error {
	select {
	case <-ctx.Done():
		return &ValidationCancelledError{}
	default:
		return nil
	}
}

type ValidationCancelledError struct{}

func (e *ValidationCancelledError) Error() string { return "validation cancelled" }
