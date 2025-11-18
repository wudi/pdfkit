package extensions

import (
	"sort"

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
