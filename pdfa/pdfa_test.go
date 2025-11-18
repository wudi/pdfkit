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
	level := pdfa.PDFA1B
	cfg := writer.Config{PDFALevel: level}
	e := pdfa.NewEnforcer()
	doc := &semantic.Document{}
	if err := e.Enforce(staticCtx{}, doc, cfg.PDFALevel); err != nil {
		t.Fatalf("enforce: %v", err)
	}
	rep, err := e.Validate(staticCtx{}, doc, level)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if rep.Level != level {
		t.Fatalf("expected level %v got %v", level, rep.Level)
	}
}
