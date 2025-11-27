package scripting

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGojaEngine_ContextCancellation(t *testing.T) {
	engine := NewEngine()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	if _, err := engine.Execute(ctx, "while (true) {}"); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline error, got %v", err)
	}

	if _, err := engine.Execute(context.Background(), "1 + 1"); err != nil {
		t.Fatalf("engine should recover after cancellation, got %v", err)
	}
}

func TestGojaEngine_ImmediateCancel(t *testing.T) {
	engine := NewEngine()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := engine.Execute(ctx, "42"); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
