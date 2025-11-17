package streaming

type EventType int
const ( EventDocumentStart EventType = iota; EventDocumentEnd )

type Event interface { Type() EventType }

type DocumentStartEvent struct{ Version string }
func (DocumentStartEvent) Type() EventType { return EventDocumentStart }

type DocumentStream interface { Events() <-chan Event; Errors() <-chan error; Close() error }

type StreamConfig struct { BufferSize int; ReadAhead int; Concurrency int }

type Parser interface { Stream(ctx Context, r ReaderAt, cfg StreamConfig) (DocumentStream, error) }

type Context interface{ Done() <-chan struct{} }

type ReaderAt interface{ ReadAt(p []byte, off int64) (n int, err error) }
