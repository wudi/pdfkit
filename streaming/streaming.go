package streaming

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"pdflib/filters"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
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

// ContentStreamEvent carries decoded content operations and resources for a page.
type ContentStreamEvent struct {
	PageIndex  int
	Operations []semantic.Operation
	Resources  *semantic.Resources
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
		for _, content := range page.contents {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ops := parseOperations(content.data)
			res := resourcesFromDict(doc, page.resources)
			events <- ContentStreamEvent{PageIndex: i, Operations: ops, Resources: res}
		}
		events <- PageEndEvent{Index: i}
	}
}

type pageInfo struct {
	mediaBox  [4]float64
	resources raw.Object
	contents  []decodedStream
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
		var resources raw.Object = dict // keep resources reference for later resolution
		if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
			resources = res
		}
		contents := collectContents(doc, dict)
		*pages = append(*pages, pageInfo{mediaBox: mbox, contents: contents, resources: resources})
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

type decodedStream struct {
	data []byte
}

func collectContents(doc *raw.Document, pageDict raw.Dictionary) []decodedStream {
	contentsObj, ok := pageDict.Get(raw.NameLiteral("Contents"))
	if !ok {
		return nil
	}
	var data []decodedStream
	appendStream := func(obj raw.Object) {
		if ref, ok := obj.(raw.RefObj); ok {
			if s, ok := doc.Objects[ref.Ref()].(*raw.StreamObj); ok {
				data = append(data, decodedStream{data: decodeStream(s)})
			}
			return
		}
		if s, ok := obj.(*raw.StreamObj); ok {
			data = append(data, decodedStream{data: decodeStream(s)})
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
	return data
}

func decodeStream(stream *raw.StreamObj) []byte {
	if stream == nil {
		return nil
	}
	names, params := parseFilters(stream.Dict)
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
	if len(names) == 0 {
		return stream.RawData()
	}
	decoded, err := fp.Decode(context.Background(), stream.RawData(), names, params)
	if err != nil {
		return stream.RawData()
	}
	return decoded
}

func parseFilters(dict *raw.DictObj) ([]string, []raw.Dictionary) {
	if dict == nil {
		return nil, nil
	}
	filterObj, ok := dict.Get(raw.NameLiteral("Filter"))
	if !ok {
		return nil, nil
	}
	var names []string
	var params []raw.Dictionary
	add := func(obj raw.Object) {
		if n, ok := obj.(raw.NameObj); ok {
			names = append(names, n.Value())
		}
	}
	switch v := filterObj.(type) {
	case raw.NameObj:
		add(v)
	case *raw.ArrayObj:
		for _, it := range v.Items {
			add(it)
		}
	}
	if parmsObj, ok := dict.Get(raw.NameLiteral("DecodeParms")); ok {
		switch v := parmsObj.(type) {
		case *raw.DictObj:
			params = append(params, v)
		case *raw.ArrayObj:
			for _, it := range v.Items {
				if d, ok := it.(*raw.DictObj); ok {
					params = append(params, d)
				}
			}
		}
	}
	return names, params
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

func parseOperations(data []byte) []semantic.Operation {
	tokens := splitTokens(string(data))
	ops := []semantic.Operation{}
	stack := []semantic.Operand{}
	for _, tok := range tokens {
		if num, err := strconv.ParseFloat(tok, 64); err == nil {
			stack = append(stack, semantic.NumberOperand{Value: num})
			continue
		}
		if len(tok) > 0 && tok[0] == '/' {
			stack = append(stack, semantic.NameOperand{Value: tok[1:]})
			continue
		}
		if len(tok) >= 2 && tok[0] == '(' && tok[len(tok)-1] == ')' {
			stack = append(stack, semantic.StringOperand{Value: []byte(tok[1 : len(tok)-1])})
			continue
		}
		// treat as operator
		ops = append(ops, semantic.Operation{Operator: tok, Operands: stack})
		stack = stack[:0]
	}
	return ops
}

func resourcesFromDict(doc *raw.Document, obj raw.Object) *semantic.Resources {
	_, dict := resolveDict(doc, obj)
	if dict == nil {
		return nil
	}
	res := &semantic.Resources{}
	if fontsObj, ok := dict.Get(raw.NameLiteral("Font")); ok {
		if fdict, ok := fontsObj.(*raw.DictObj); ok {
			res.Fonts = make(map[string]*semantic.Font)
			for name, entry := range fdict.KV {
				_, fontDict := resolveDict(doc, entry)
				if fontDict == nil {
					continue
				}
				font := &semantic.Font{}
				if subtype, ok := fontDict.Get(raw.NameLiteral("Subtype")); ok {
					if n, ok := subtype.(raw.NameObj); ok {
						font.Subtype = n.Value()
					}
				}
				if base, ok := fontDict.Get(raw.NameLiteral("BaseFont")); ok {
					if n, ok := base.(raw.NameObj); ok {
						font.BaseFont = n.Value()
					}
				}
				if enc, ok := fontDict.Get(raw.NameLiteral("Encoding")); ok {
					if n, ok := enc.(raw.NameObj); ok {
						font.Encoding = n.Value()
					}
				}
				res.Fonts[name] = font
			}
		}
	}
	return res
}

// splitTokens is a small, naive tokenizer for content streams.
func splitTokens(src string) []string {
	fields := strings.Fields(src)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f)
	}
	return out
}

type readerAtAdapter struct{ ReaderAt }

func (r readerAtAdapter) ReadAt(p []byte, off int64) (int, error) {
	return r.ReaderAt.ReadAt(p, off)
}
