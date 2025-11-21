package contentstream

import (
	"context"
	"testing"

	"github.com/wudi/pdfkit/ir/semantic"
)

type testHandler struct {
	calls int
	last  []string
}

func (h *testHandler) Handle(_ *ExecutionContext, operands []semantic.Operand) error {
	h.calls++
	h.last = make([]string, len(operands))
	for i, op := range operands {
		h.last[i] = op.Type()
	}
	return nil
}

func TestProcessorDispatchesOperators(t *testing.T) {
	p := NewProcessor()
	h := &testHandler{}
	p.RegisterHandler("Tj", h)

	state := &GraphicsState{}
	err := p.Process(context.Background(), []byte("(Hello) Tj"), state)
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("expected handler to be called once, got %d", h.calls)
	}
	if len(h.last) != 1 || h.last[0] != "string" {
		t.Fatalf("unexpected operand types: %v", h.last)
	}
}
