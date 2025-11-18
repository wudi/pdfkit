package pdfa

import "pdflib/ir/semantic"

// Level represents a PDF/A conformance level shared across writer and enforcer.
type Level int

const (
	PDFA1B Level = iota
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
	return nil
}

func (e *enforcerImpl) Validate(ctx Context, doc *semantic.Document, level Level) (*ComplianceReport, error) {
	return &ComplianceReport{Compliant: true, Level: level}, nil
}

type Context interface{ Done() <-chan struct{} }
