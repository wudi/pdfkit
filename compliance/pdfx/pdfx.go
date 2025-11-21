package pdfx

import (
	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type Level int

const (
	PDFX1a Level = iota
	PDFX3
	PDFX4
)

func (l Level) String() string {
	switch l {
	case PDFX1a:
		return "PDF/X-1a"
	case PDFX3:
		return "PDF/X-3"
	case PDFX4:
		return "PDF/X-4"
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
	// 1. Remove Encryption
	if doc.Encrypted {
		doc.Encrypted = false
		doc.Permissions = raw.Permissions{}
		doc.OwnerPassword = ""
		doc.UserPassword = ""
	}

	// 2. Add OutputIntent if missing
	hasGTSPDFX := false
	for _, intent := range doc.OutputIntents {
		if intent.S == "GTS_PDFX" {
			hasGTSPDFX = true
			break
		}
	}
	if !hasGTSPDFX {
		doc.OutputIntents = append(doc.OutputIntents, semantic.OutputIntent{
			S:                         "GTS_PDFX",
			OutputConditionIdentifier: "CGATS TR 001", // Example
			Info:                      "CGATS TR 001",
			DestOutputProfile:         nil, // Should be valid profile
		})
	}

	// 3. Set Trapped key
	if doc.Info == nil {
		doc.Info = &semantic.DocumentInfo{}
	}
	if doc.Info.Trapped == "" {
		doc.Info.Trapped = "False" // Default to False
	}

	// 4. Ensure TrimBox
	for _, p := range doc.Pages {
		if p.TrimBox == (semantic.Rectangle{}) && p.ArtBox == (semantic.Rectangle{}) {
			// Default to CropBox or MediaBox
			if p.CropBox != (semantic.Rectangle{}) {
				p.TrimBox = p.CropBox
			} else {
				p.TrimBox = p.MediaBox
			}
		}
	}

	return nil
}

func (e *enforcerImpl) Validate(ctx compliance.Context, doc *semantic.Document) (*compliance.Report, error) {
	return e.ValidateLevel(ctx, doc, PDFX1a)
}

func (e *enforcerImpl) ValidateLevel(ctx compliance.Context, doc *semantic.Document, level Level) (*compliance.Report, error) {
	report := &compliance.Report{
		Standard:   level.String(),
		Violations: []compliance.Violation{},
	}

	// 1. Encryption Forbidden
	if doc.Encrypted {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "ENC001",
			Description: "Encryption is forbidden in PDF/X",
			Location:    "Document",
		})
	}

	// 2. OutputIntent Required (GTS_PDFX)
	hasGTSPDFX := false
	for _, intent := range doc.OutputIntents {
		if intent.S == "GTS_PDFX" {
			hasGTSPDFX = true
			break
		}
	}
	if !hasGTSPDFX {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "INT001",
			Description: "OutputIntent (GTS_PDFX) is required",
			Location:    "Catalog",
		})
	}

	// 3. Trapped key in Info dictionary
	if doc.Info == nil || doc.Info.Trapped == "" {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "INF001",
			Description: "Info dictionary must contain 'Trapped' key",
			Location:    "Trailer",
		})
	} else {
		if doc.Info.Trapped != "True" && doc.Info.Trapped != "False" && doc.Info.Trapped != "Unknown" {
			report.Violations = append(report.Violations, compliance.Violation{
				Code:        "INF002",
				Description: "Trapped key must be True, False, or Unknown",
				Location:    "Info",
			})
		}
	}

	// 4. Page Boxes (TrimBox/BleedBox)
	for i, p := range doc.Pages {
		if p.TrimBox == (semantic.Rectangle{}) && p.ArtBox == (semantic.Rectangle{}) {
			report.Violations = append(report.Violations, compliance.Violation{
				Code:        "BOX001",
				Description: "TrimBox or ArtBox should be defined",
				Location:    "Page " + string(rune(i+1)),
			})
		}
	}

	// 5. Colors (PDF/X-1a: CMYK/Spot only)
	if level == PDFX1a {
		for i, p := range doc.Pages {
			if p.Resources == nil {
				continue
			}
			for name, cs := range p.Resources.ColorSpaces {
				if !isValidPDFX1aColorSpace(cs) {
					report.Violations = append(report.Violations, compliance.Violation{
						Code:        "CLR001",
						Description: "Invalid ColorSpace for PDF/X-1a: " + cs.ColorSpaceName(),
						Location:    "Page " + string(rune(i+1)) + " Resource " + name,
					})
				}
			}
		}
	}

	// 6. Transparency (Forbidden in X-1a and X-3)
	if level == PDFX1a || level == PDFX3 {
		for i, p := range doc.Pages {
			if p.Resources == nil {
				continue
			}
			for name, gs := range p.Resources.ExtGStates {
				if isTransparent(gs) {
					report.Violations = append(report.Violations, compliance.Violation{
						Code:        "TRN001",
						Description: "Transparency is forbidden in " + level.String(),
						Location:    "Page " + string(rune(i+1)) + " ExtGState " + name,
					})
				}
			}
		}
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}

func isValidPDFX1aColorSpace(cs semantic.ColorSpace) bool {
	switch cs.ColorSpaceName() {
	case "DeviceCMYK", "DeviceGray", "Separation", "DeviceN":
		return true
	case "Pattern":
		return true // Patterns need recursive check, assuming OK for now
	default:
		return false // RGB, Lab, ICCBased (unless CMYK output intent matches)
	}
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
