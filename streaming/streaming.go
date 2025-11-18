package streaming

import (
	"context"
	"sync"

	"pdflib/parser"
)

type EventType int

const (
	EventDocumentStart EventType = iota
	EventDocumentEnd
)

type Event interface{ Type() EventType }

type DocumentStartEvent struct {
	Version   string
	Encrypted bool
}

func (DocumentStartEvent) Type() EventType { return EventDocumentStart }

type DocumentEndEvent struct{}

func (DocumentEndEvent) Type() EventType { return EventDocumentEnd }

type DocumentStream interface {
	Events() <-chan Event
	Errors() <-chan error
	Close() error
}

type StreamConfig struct {
	BufferSize  int
	ReadAhead   int
	Concurrency int
}

type Parser interface {
	Stream(ctx Context, r ReaderAt, cfg StreamConfig) (DocumentStream, error)
}

type Context interface{ Done() <-chan struct{} }

type ReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
}

// NewParser constructs a streaming parser that emits document boundary events.
func NewParser() Parser {
	return &parserImpl{
		rawParser: parser.NewDocumentParser(parser.Config{}),
	}
}

type parserImpl struct {
	rawParser *parser.DocumentParser
}

// Stream parses the document once and emits DocumentStart and DocumentEnd
// events on the returned stream. Call Close to cancel early.
func (p *parserImpl) Stream(ctx Context, r ReaderAt, cfg StreamConfig) (DocumentStream, error) {
	events := make(chan Event, cfg.BufferSize)
	errs := make(chan error, 1)
	cctx, cancel := context.WithCancel(context.Background())
	ds := &documentStream{events: events, errors: errs, cancel: cancel}
	ds.wg.Add(1)
	go func() {
		defer ds.wg.Done()
		defer close(events)
		defer close(errs)

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Emit start/end; reuse existing parser to obtain header info.
		rp := p.rawParser
		if rp == nil {
			rp = parser.NewDocumentParser(parser.Config{})
		}
		rawDoc, err := rp.Parse(cctx, readerAtAdapter{r})
		if err != nil {
			select {
			case errs <- err:
			default:
			}
			return
		}

		start := DocumentStartEvent{Version: rawDoc.Version, Encrypted: rawDoc.Encrypted}
		select {
		case events <- start:
		case <-ctx.Done():
			return
		case <-cctx.Done():
			return
		}

		select {
		case events <- DocumentEndEvent{}:
		case <-ctx.Done():
		case <-cctx.Done():
		}
	}()

	return ds, nil
}

type documentStream struct {
	events chan Event
	errors chan error
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (ds *documentStream) Events() <-chan Event { return ds.events }
func (ds *documentStream) Errors() <-chan error { return ds.errors }
func (ds *documentStream) Close() error {
	ds.cancel()
	ds.wg.Wait()
	return nil
}

type readerAtAdapter struct{ ReaderAt }

func (r readerAtAdapter) ReadAt(p []byte, off int64) (int, error) {
	return r.ReaderAt.ReadAt(p, off)
}
