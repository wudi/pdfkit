package extensions

import (
	"sort"

	"pdflib/ir/raw"
	"pdflib/ir/semantic"
)

type Phase int

const (
	PhaseInspect Phase = iota
	PhaseSanitize
	PhaseTransform
	PhaseValidate
)

func (p Phase) String() string { return []string{"Inspect", "Sanitize", "Transform", "Validate"}[p] }

type Extension interface {
	Name() string
	Phase() Phase
	Priority() int
	Execute(ctx Context, doc *semantic.Document) error
}

// Inspector is an extension that inspects the document and produces a report.
type Inspector interface {
	Extension
	Inspect(ctx Context, doc *semantic.Document) (*InspectionReport, error)
}

// Sanitizer is an extension that cleans up the document.
type Sanitizer interface {
	Extension
	Sanitize(ctx Context, doc *semantic.Document) (*SanitizationReport, error)
}

// Transformer is an extension that modifies the document structure.
type Transformer interface {
	Extension
	Transform(ctx Context, doc *semantic.Document) error
}

// Validator is an extension that validates the document against a standard.
type Validator interface {
	Extension
	Validate(ctx Context, doc *semantic.Document) (*ValidationReport, error)
}

type InspectionReport struct {
	PageCount  int
	FontCount  int
	ImageCount int
	FileSize   int64
	Version    string
	Encrypted  bool
	Linearized bool
	Tagged     bool
	Metadata   map[string]interface{}
}

type SanitizationReport struct {
	ItemsRemoved int
	ItemsFixed   int
	Actions      []SanitizationAction
}

type SanitizationAction struct {
	Type        string
	Description string
	ObjectRef   raw.ObjectRef
}

type ValidationReport struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

type ValidationError struct {
	Code      string
	Message   string
	Location  string
	ObjectRef raw.ObjectRef
}

type ValidationWarning struct {
	Code     string
	Message  string
	Location string
}

type Hub interface {
	Register(ext Extension) error
	Execute(ctx Context, doc *semantic.Document) error
	Extensions(phase Phase) []Extension
}

type HubImpl struct {
	exts map[Phase][]Extension
}

func NewHub() *HubImpl { return &HubImpl{exts: make(map[Phase][]Extension)} }

func (h *HubImpl) Register(ext Extension) error {
	ph := ext.Phase()
	h.exts[ph] = append(h.exts[ph], ext)
	sort.Slice(h.exts[ph], func(i, j int) bool { return h.exts[ph][i].Priority() < h.exts[ph][j].Priority() })
	return nil
}

func (h *HubImpl) Execute(ctx Context, doc *semantic.Document) error {
	phases := []Phase{PhaseInspect, PhaseSanitize, PhaseTransform, PhaseValidate}
	for _, ph := range phases {
		for _, e := range h.exts[ph] {
			if err := e.Execute(ctx, doc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *HubImpl) Extensions(phase Phase) []Extension {
	return append([]Extension(nil), h.exts[phase]...)
}

type Context interface{ Done() <-chan struct{} }
