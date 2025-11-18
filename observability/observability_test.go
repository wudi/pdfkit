package observability

import (
	"context"
	"testing"
)

func TestNopTracer(t *testing.T) {
	tracer := NopTracer()
	ctx := context.Background()
	ctx2, span := tracer.StartSpan(ctx, "test")
	if ctx2 != ctx {
		t.Fatalf("nop tracer should return same context")
	}
	span.SetTag("key", "value")
	span.SetError(nil)
	span.Finish()
}
