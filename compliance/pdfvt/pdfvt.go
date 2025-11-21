package pdfvt

import (
	"github.com/wudi/pdfkit/compliance"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
)

type Level int

const (
	PDFVT1 Level = iota
	PDFVT2
)

func (l Level) String() string {
	switch l {
	case PDFVT1:
		return "PDF/VT-1"
	case PDFVT2:
		return "PDF/VT-2"
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
	hasGTSPDFVT := false
	for _, intent := range doc.OutputIntents {
		if intent.S == "GTS_PDFVT" {
			hasGTSPDFVT = true
			break
		}
	}
	if !hasGTSPDFVT {
		doc.OutputIntents = append(doc.OutputIntents, semantic.OutputIntent{
			S:                         "GTS_PDFVT",
			OutputConditionIdentifier: "CGATS TR 001", // Example
			Info:                      "CGATS TR 001",
			DestOutputProfile:         nil,
		})
	}

	// 3. Create DPartRoot if missing
	if doc.DPartRoot == nil {
		doc.DPartRoot = &semantic.DPartRoot{}
	}

	return nil
}

func (e *enforcerImpl) Validate(ctx compliance.Context, doc *semantic.Document) (*compliance.Report, error) {
	report := &compliance.Report{
		Standard:   "PDF/VT",
		Violations: []compliance.Violation{},
	}

	// 1. Encryption Forbidden
	if doc.Encrypted {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "ENC001",
			Description: "Encryption is forbidden in PDF/VT",
			Location:    "Document",
		})
	}

	// 2. OutputIntent Required (GTS_PDFVT)
	hasGTSPDFVT := false
	for _, intent := range doc.OutputIntents {
		if intent.S == "GTS_PDFVT" {
			hasGTSPDFVT = true
			break
		}
	}
	if !hasGTSPDFVT {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "INT001",
			Description: "OutputIntent (GTS_PDFVT) is required",
			Location:    "Catalog",
		})
	}

	// 3. DPartRoot Required
	if doc.DPartRoot == nil {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "VT001",
			Description: "DPartRoot is required",
			Location:    "Catalog",
		})
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}
