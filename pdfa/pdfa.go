package pdfa

import "pdflib/ir/semantic"

type PDFALevel int
const ( PDFA1B PDFALevel = iota )

type Violation struct { Code, Description, Location string }

type ComplianceReport struct { Compliant bool; Level PDFALevel; Violations []Violation }

type Enforcer interface { Enforce(ctx Context, doc *semantic.Document, level PDFALevel) error; Validate(ctx Context, doc *semantic.Document, level PDFALevel) (*ComplianceReport, error) }

type enforcerImpl struct{}

func NewEnforcer() Enforcer { return &enforcerImpl{} }
func (e *enforcerImpl) Enforce(ctx Context, doc *semantic.Document, level PDFALevel) error { return nil }
func (e *enforcerImpl) Validate(ctx Context, doc *semantic.Document, level PDFALevel) (*ComplianceReport, error) { return &ComplianceReport{Compliant:true, Level:level}, nil }

type Context interface{ Done() <-chan struct{} }
