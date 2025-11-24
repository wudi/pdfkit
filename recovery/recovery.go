package recovery

import "context"

type Strategy interface {
	OnError(ctx context.Context, err error, location Location) Action
}

type Location struct {
	ByteOffset int64
	ObjectNum  int
	ObjectGen  int
	Component  string
}

type Action int

const (
	ActionFail Action = iota
	ActionSkip
	ActionFix
	ActionWarn
)
