package pdfa_test

import (
	"testing"

	"pdflib/ir/semantic"
	"pdflib/pdfa"
	"pdflib/writer"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestPDFALevelSharedType(t *testing.T) {
	levels := []pdfa.Level{pdfa.PDFA1B, pdfa.PDFA3B}
	for _, level := range levels {
		cfg := writer.Config{PDFALevel: level}
		e := pdfa.NewEnforcer()
		doc := &semantic.Document{}
		if err := e.Enforce(staticCtx{}, doc, cfg.PDFALevel); err != nil {
			t.Fatalf("enforce level %v: %v", level, err)
		}
		rep, err := e.Validate(staticCtx{}, doc, level)
		if err != nil {
			t.Fatalf("validate level %v: %v", level, err)
		}
		if rep.Level != level {
			t.Fatalf("expected level %v got %v", level, rep.Level)
		}
	}
}
