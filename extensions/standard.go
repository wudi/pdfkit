package extensions

import (
	"fmt"
	"strings"

	"pdflib/ir/semantic"
	"pdflib/pdfa"
)

// BasicInspector implements a simple document inspector.
type BasicInspector struct{}

func (i *BasicInspector) Name() string  { return "BasicInspector" }
func (i *BasicInspector) Phase() Phase  { return PhaseInspect }
func (i *BasicInspector) Priority() int { return 100 }
func (i *BasicInspector) Execute(ctx Context, doc *semantic.Document) error {
	_, err := i.Inspect(ctx, doc)
	return err
}

func (i *BasicInspector) Inspect(ctx Context, doc *semantic.Document) (*InspectionReport, error) {
	report := &InspectionReport{
		PageCount: len(doc.Pages),
		Metadata:  make(map[string]interface{}),
		Version:   "1.7", // Default assumption if not tracked elsewhere
		Encrypted: doc.Encrypted,
		Tagged:    doc.Marked,
	}

	if doc.Info != nil {
		if doc.Info.Title != "" {
			report.Metadata["Title"] = doc.Info.Title
		}
		if doc.Info.Author != "" {
			report.Metadata["Author"] = doc.Info.Author
		}
		if doc.Info.Producer != "" {
			report.Metadata["Producer"] = doc.Info.Producer
		}
	}

	fontCount := 0
	imageCount := 0
	for _, p := range doc.Pages {
		if p.Resources != nil {
			fontCount += len(p.Resources.Fonts)
			for _, xo := range p.Resources.XObjects {
				if xo.Subtype == "Image" {
					imageCount++
				}
			}
		}
	}
	report.FontCount = fontCount
	report.ImageCount = imageCount

	return report, nil
}

// JSSanitizer removes JavaScript actions from annotations.
type JSSanitizer struct{}

func (s *JSSanitizer) Name() string  { return "JSSanitizer" }
func (s *JSSanitizer) Phase() Phase  { return PhaseSanitize }
func (s *JSSanitizer) Priority() int { return 100 }
func (s *JSSanitizer) Execute(ctx Context, doc *semantic.Document) error {
	_, err := s.Sanitize(ctx, doc)
	return err
}

func (s *JSSanitizer) Sanitize(ctx Context, doc *semantic.Document) (*SanitizationReport, error) {
	report := &SanitizationReport{}

	for i, p := range doc.Pages {
		var cleanAnnots []semantic.Annotation
		for _, annot := range p.Annotations {
			isJS := false
			if link, ok := annot.(*semantic.LinkAnnotation); ok {
				if strings.HasPrefix(link.URI, "javascript:") {
					isJS = true
				}
			}
			if isJS {
				report.ItemsRemoved++
				report.Actions = append(report.Actions, SanitizationAction{
					Type:        "RemoveAnnotation",
					Description: fmt.Sprintf("Removed JavaScript annotation on page %d", i+1),
					ObjectRef:   annot.Reference(),
				})
				continue
			}
			cleanAnnots = append(cleanAnnots, annot)
		}
		p.Annotations = cleanAnnots
	}

	return report, nil
}

// PDFAValidator wraps the pdfa package to provide validation.
type PDFAValidator struct {
	Level pdfa.Level
}

func (v *PDFAValidator) Name() string  { return "PDFAValidator" }
func (v *PDFAValidator) Phase() Phase  { return PhaseValidate }
func (v *PDFAValidator) Priority() int { return 100 }
func (v *PDFAValidator) Execute(ctx Context, doc *semantic.Document) error {
	_, err := v.Validate(ctx, doc)
	return err
}

func (v *PDFAValidator) Validate(ctx Context, doc *semantic.Document) (*ValidationReport, error) {
	enforcer := pdfa.NewEnforcer()
	report, err := enforcer.Validate(ctx, doc, v.Level)
	if err != nil {
		return nil, err
	}

	valReport := &ValidationReport{
		Valid: report.Compliant,
	}

	for _, viol := range report.Violations {
		valReport.Errors = append(valReport.Errors, ValidationError{
			Code:     viol.Code,
			Message:  viol.Description,
			Location: viol.Location,
		})
	}

	return valReport, nil
}
