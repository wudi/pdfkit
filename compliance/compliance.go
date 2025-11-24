package compliance

import (
	"context"

	"github.com/wudi/pdfkit/ir/semantic"
)

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
	Validate(ctx context.Context, doc *semantic.Document) (*Report, error)
}
