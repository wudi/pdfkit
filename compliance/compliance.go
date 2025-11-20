package compliance

import (
	"context"
	"pdflib/ir/semantic"
)

// Context is an alias for context.Context to allow for future expansion.
type Context = context.Context

// Violation represents a compliance violation.
type Violation struct {
	Code        string
	Description string
	Location    string
}

// Report details compliance status.
type Report struct {
	Compliant  bool
	Standard   string // e.g., "PDF/A-1b", "PDF/X-4"
	Violations []Violation
}

// Validator checks document compliance against a standard.
type Validator interface {
	Validate(ctx Context, doc *semantic.Document) (*Report, error)
}
