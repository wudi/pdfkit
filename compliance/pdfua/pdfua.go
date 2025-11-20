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
	// TODO: Implement PDF/UA enforcement
	return nil
}

func (e *enforcerImpl) Validate(ctx compliance.Context, doc *semantic.Document) (*compliance.Report, error) {
	report := &compliance.Report{
		Standard:   "PDF/UA-1",
		Violations: []compliance.Violation{},
	}

	// PDF/UA requires Tagged PDF
	if doc.StructTree == nil {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA001",
			Description: "Document must be tagged (StructTree missing)",
			Location:    "Catalog",
		})
	}

	// Title required
	if doc.Info == nil || doc.Info.Title == "" {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA002",
			Description: "Document title is required",
			Location:    "Info Dictionary",
		})
	}

	// Language required
	if doc.Lang == "" {
		report.Violations = append(report.Violations, compliance.Violation{
			Code:        "UA003",
			Description: "Document language is required",
			Location:    "Catalog",
		})
	}

	report.Compliant = len(report.Violations) == 0
	return report, nil
}
