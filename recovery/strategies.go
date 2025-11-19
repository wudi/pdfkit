package recovery

import "fmt"

// StrictStrategy implements a fail-fast recovery strategy.
type StrictStrategy struct{}

func NewStrictStrategy() *StrictStrategy {
	return &StrictStrategy{}
}

func (s *StrictStrategy) OnError(ctx Context, err error, location Location) Action {
	return ActionFail
}

// LenientStrategy implements a best-effort recovery strategy.
// It logs errors (if a logger were available) and attempts to continue.
type LenientStrategy struct {
	// In a real implementation, we might have a logger here.
	// For now, we just accumulate errors if needed, or just return ActionWarn/ActionFix.
	Errors []error
}

func NewLenientStrategy() *LenientStrategy {
	return &LenientStrategy{}
}

func (s *LenientStrategy) OnError(ctx Context, err error, location Location) Action {
	s.Errors = append(s.Errors, fmt.Errorf("[%s] offset %d: %w", location.Component, location.ByteOffset, err))
	// Default to warning/skipping for now.
	// In a more advanced implementation, we could inspect the error type to decide between Fix, Skip, or Warn.
	return ActionWarn
}
