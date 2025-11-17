package recovery

type Strategy interface {
	OnError(ctx Context, err error, location Location) Action
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

type Context interface{ Done() <-chan struct{} }
