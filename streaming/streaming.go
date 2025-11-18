package streaming

import (
	"context"
	"sync"

	"pdflib/filters"
	"pdflib/ir/raw"
	"pdflib/parser"
)

type EventType int

const (
	EventDocumentStart EventType = iota
	EventDocumentEnd
	EventPageStart
	EventContentStream
	EventPageEnd
)

type Event interface{ Type() EventType }

type DocumentStartEvent struct {
	Version   string
	Encrypted bool
}

func (DocumentStartEvent) Type() EventType { return EventDocumentStart }

type DocumentEndEvent struct{}

func (DocumentEndEvent) Type() EventType { return EventDocumentEnd }

// PageStartEvent marks the beginning of a page and carries basic geometry.
type PageStartEvent struct {
	Index    int
	MediaBox [4]float64
}

func (PageStartEvent) Type() EventType { return EventPageStart }

// ContentStreamEvent carries raw page content stream bytes (decoded if available).
type ContentStreamEvent struct {
	PageIndex int
	Data      []byte
}

func (ContentStreamEvent) Type() EventType { return EventContentStream }

// PageEndEvent marks the end of a page.
type PageEndEvent struct{ Index int }

func (PageEndEvent) Type() EventType { return EventPageEnd }

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

		emitPages(ctx, rawDoc, events)

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

func emitPages(ctx Context, doc *raw.Document, events chan<- Event) {
	rootObj, ok := doc.Trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return
	}
	_, catalog := resolveDict(doc, rootObj)
	if catalog == nil {
		return
	}
	pagesObj, ok := catalog.Get(raw.NameLiteral("Pages"))
	if !ok {
		return
	}
	pagesRef, dict := resolveDict(doc, pagesObj)
	if dict == nil {
		return
	}
	pageList := []pageInfo{}
	walkPages(doc, pagesRef, dict, [4]float64{}, &pageList)
	for i, page := range pageList {
		select {
		case <-ctx.Done():
			return
		default:
		}
		events <- PageStartEvent{Index: i, MediaBox: page.mediaBox}
		for _, data := range page.contents {
			select {
			case <-ctx.Done():
				return
			default:
			}
			events <- ContentStreamEvent{PageIndex: i, Data: data}
		}
		events <- PageEndEvent{Index: i}
	}
}

type pageInfo struct {
	mediaBox [4]float64
	contents [][]byte
}

func walkPages(doc *raw.Document, ref raw.ObjectRef, dict raw.Dictionary, inheritedBox [4]float64, pages *[]pageInfo) {
	typ, _ := dict.Get(raw.NameLiteral("Type"))
	if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
		mbox := inheritedBox
		if mb, ok := dict.Get(raw.NameLiteral("MediaBox")); ok {
			if rect, ok := rectFromArray(mb); ok {
				mbox = rect
			}
		}
		contents := collectContents(doc, dict)
		*pages = append(*pages, pageInfo{mediaBox: mbox, contents: contents})
		return
	}
	if name, ok := typ.(raw.NameObj); !ok || name.Value() != "Pages" {
		return
	}
	var inheritBox = inheritedBox
	if mb, ok := dict.Get(raw.NameLiteral("MediaBox")); ok {
		if rect, ok := rectFromArray(mb); ok {
			inheritBox = rect
		}
	}
	if kidsObj, ok := dict.Get(raw.NameLiteral("Kids")); ok {
		if arr, ok := kidsObj.(*raw.ArrayObj); ok {
			for _, kid := range arr.Items {
				refKid, kidDict := resolveDict(doc, kid)
				if kidDict != nil {
					walkPages(doc, refKid, kidDict, inheritBox, pages)
				}
			}
		}
	}
}

func collectContents(doc *raw.Document, pageDict raw.Dictionary) [][]byte {
	contentsObj, ok := pageDict.Get(raw.NameLiteral("Contents"))
	if !ok {
		return nil
	}
	var data [][]byte
	appendStream := func(obj raw.Object) {
		if ref, ok := obj.(raw.RefObj); ok {
			if s, ok := doc.Objects[ref.Ref()].(*raw.StreamObj); ok {
				data = append(data, s.RawData())
			}
			return
		}
		if s, ok := obj.(*raw.StreamObj); ok {
			data = append(data, s.RawData())
		}
	}
	switch v := contentsObj.(type) {
	case *raw.ArrayObj:
		for _, it := range v.Items {
			appendStream(it)
		}
	default:
		appendStream(v)
	}
	return decodeStreams(data)
}

func decodeStreams(streams [][]byte) [][]byte {
	if len(streams) == 0 {
		return nil
	}
	fp := filters.NewPipeline(
		[]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewRunLengthDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
		},
		filters.Limits{},
	)
	out := make([][]byte, 0, len(streams))
	for _, s := range streams {
		decoded, err := fp.Decode(context.Background(), s, nil, nil)
		if err != nil {
			out = append(out, s) // fall back to raw
			continue
		}
		out = append(out, decoded)
	}
	return out
}

func resolveDict(doc *raw.Document, obj raw.Object) (raw.ObjectRef, raw.Dictionary) {
	switch v := obj.(type) {
	case raw.RefObj:
		if d, ok := doc.Objects[v.Ref()].(*raw.DictObj); ok {
			return v.Ref(), d
		}
	case *raw.DictObj:
		return raw.ObjectRef{}, v
	}
	return raw.ObjectRef{}, nil
}

func rectFromArray(obj raw.Object) ([4]float64, bool) {
	arr, ok := obj.(*raw.ArrayObj)
	if !ok || arr.Len() != 4 {
		return [4]float64{}, false
	}
	var rect [4]float64
	for i, it := range arr.Items {
		if num, ok := it.(raw.NumberObj); ok {
			rect[i] = num.Float()
		} else {
			return [4]float64{}, false
		}
	}
	return rect, true
}

type readerAtAdapter struct{ ReaderAt }

func (r readerAtAdapter) ReadAt(p []byte, off int64) (int, error) {
	return r.ReaderAt.ReadAt(p, off)
}
