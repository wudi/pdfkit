package streaming

import (
	"bytes"
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
	PageIndex    int
	Operations   []semantic.Operation
	Resources    *semantic.Resources
	UsedFonts    []string
	UsedXObjects []string
	UsedPatterns []string
	UsedShadings []string
	InlineImages []InlineImage
}

// InlineImage captures inline image dictionary and data from a content stream.
type InlineImage struct {
	Dict semantic.DictOperand
	Data []byte
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
			inline := extractInlineImages(content.data)
			usage := collectUsage(ops)
			events <- ContentStreamEvent{
				PageIndex:    i,
				Operations:   ops,
				Resources:    res,
				InlineImages: inline,
				UsedFonts:    usage.fonts,
				UsedXObjects: usage.xobjects,
				UsedPatterns: usage.patterns,
				UsedShadings: usage.shadings,
			}
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
	tokens := lexTokens(data)
	ops := []semantic.Operation{}
	stack := []semantic.Operand{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok.kind {
		case "number":
			stack = append(stack, semantic.NumberOperand{Value: tok.num})
		case "name":
			stack = append(stack, semantic.NameOperand{Value: tok.text})
		case "string", "hex":
			stack = append(stack, semantic.StringOperand{Value: tok.bytes})
		case "arrayStart":
			op, next := parseArrayOperand(tokens, i+1)
			stack = append(stack, op)
			i = next
		case "dictStart":
			op, next := parseDictOperand(tokens, i+1)
			stack = append(stack, op)
			i = next
		case "op":
			ops = append(ops, semantic.Operation{Operator: tok.text, Operands: stack})
			stack = stack[:0]
		}
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
	if csObj, ok := dict.Get(raw.NameLiteral("ColorSpace")); ok {
		if csDict, ok := csObj.(*raw.DictObj); ok {
			res.ColorSpaces = make(map[string]semantic.ColorSpace)
			for name, entry := range csDict.KV {
				if n, ok := entry.(raw.NameObj); ok {
					res.ColorSpaces[name] = semantic.ColorSpace{Name: n.Value()}
				}
			}
		}
	}
	if xoObj, ok := dict.Get(raw.NameLiteral("XObject")); ok {
		if xoDict, ok := xoObj.(*raw.DictObj); ok {
			res.XObjects = make(map[string]semantic.XObject)
			for name, entry := range xoDict.KV {
				ref, xoDict := resolveDict(doc, entry)
				if xoDict == nil {
					continue
				}
				if subtype, ok := xoDict.Get(raw.NameLiteral("Subtype")); ok {
					if sn, ok := subtype.(raw.NameObj); ok {
						switch sn.Value() {
						case "Image":
							if stream, ok := doc.Objects[ref].(*raw.StreamObj); ok {
								xo := semantic.XObject{Subtype: "Image"}
								if w, ok := xoDict.Get(raw.NameLiteral("Width")); ok {
									if n, ok := w.(raw.NumberObj); ok {
										xo.Width = int(n.Int())
									}
								}
								if h, ok := xoDict.Get(raw.NameLiteral("Height")); ok {
									if n, ok := h.(raw.NumberObj); ok {
										xo.Height = int(n.Int())
									}
								}
								if bpc, ok := xoDict.Get(raw.NameLiteral("BitsPerComponent")); ok {
									if n, ok := bpc.(raw.NumberObj); ok {
										xo.BitsPerComponent = int(n.Int())
									}
								}
								if cs, ok := xoDict.Get(raw.NameLiteral("ColorSpace")); ok {
									if n, ok := cs.(raw.NameObj); ok {
										xo.ColorSpace = semantic.ColorSpace{Name: n.Value()}
									}
								}
								xo.Data = decodeStream(stream)
								res.XObjects[name] = xo
							}
						case "Form":
							if stream, ok := doc.Objects[ref].(*raw.StreamObj); ok {
								xo := semantic.XObject{Subtype: "Form"}
								if bboxObj, ok := xoDict.Get(raw.NameLiteral("BBox")); ok {
									if rect, ok := rectFromArray(bboxObj); ok {
										xo.BBox = semantic.Rectangle{LLX: rect[0], LLY: rect[1], URX: rect[2], URY: rect[3]}
									}
								}
								xo.Data = decodeStream(stream)
								res.XObjects[name] = xo
							}
						}
					}
				}
			}
		}
	}
	if gsObj, ok := dict.Get(raw.NameLiteral("ExtGState")); ok {
		if gsDict, ok := gsObj.(*raw.DictObj); ok {
			res.ExtGStates = make(map[string]semantic.ExtGState)
			for name, entry := range gsDict.KV {
				_, gs := resolveDict(doc, entry)
				if gs == nil {
					continue
				}
				state := semantic.ExtGState{}
				if lw, ok := gs.Get(raw.NameLiteral("LW")); ok {
					if n, ok := lw.(raw.NumberObj); ok {
						val := n.Float()
						state.LineWidth = &val
					}
				}
				if ca, ok := gs.Get(raw.NameLiteral("CA")); ok {
					if n, ok := ca.(raw.NumberObj); ok {
						val := n.Float()
						state.StrokeAlpha = &val
					}
				}
				if ca, ok := gs.Get(raw.NameLiteral("ca")); ok {
					if n, ok := ca.(raw.NumberObj); ok {
						val := n.Float()
						state.FillAlpha = &val
					}
				}
				res.ExtGStates[name] = state
			}
		}
	}
	if patObj, ok := dict.Get(raw.NameLiteral("Pattern")); ok {
		if patDict, ok := patObj.(*raw.DictObj); ok {
			res.Patterns = make(map[string]semantic.Pattern)
			for name, entry := range patDict.KV {
				ref, pd := resolveDict(doc, entry)
				if pd == nil {
					continue
				}
				p := semantic.Pattern{}
				if pt, ok := pd.Get(raw.NameLiteral("PatternType")); ok {
					if n, ok := pt.(raw.NumberObj); ok {
						p.PatternType = int(n.Int())
					}
				}
				if paint, ok := pd.Get(raw.NameLiteral("PaintType")); ok {
					if n, ok := paint.(raw.NumberObj); ok {
						p.PaintType = int(n.Int())
					}
				}
				if tt, ok := pd.Get(raw.NameLiteral("TilingType")); ok {
					if n, ok := tt.(raw.NumberObj); ok {
						p.TilingType = int(n.Int())
					}
				}
				if bbox, ok := pd.Get(raw.NameLiteral("BBox")); ok {
					if rect, ok := rectFromArray(bbox); ok {
						p.BBox = semantic.Rectangle{LLX: rect[0], LLY: rect[1], URX: rect[2], URY: rect[3]}
					}
				}
				if xs, ok := pd.Get(raw.NameLiteral("XStep")); ok {
					if n, ok := xs.(raw.NumberObj); ok {
						p.XStep = n.Float()
					}
				}
				if ys, ok := pd.Get(raw.NameLiteral("YStep")); ok {
					if n, ok := ys.(raw.NumberObj); ok {
						p.YStep = n.Float()
					}
				}
				if stream, ok := doc.Objects[ref].(*raw.StreamObj); ok {
					p.Content = decodeStream(stream)
				}
				res.Patterns[name] = p
			}
		}
	}
	if shadingObj, ok := dict.Get(raw.NameLiteral("Shading")); ok {
		if sd, ok := shadingObj.(*raw.DictObj); ok {
			res.Shadings = make(map[string]semantic.Shading)
			for name, entry := range sd.KV {
				_, shDict := resolveDict(doc, entry)
				if shDict == nil {
					continue
				}
				sh := semantic.Shading{}
				if st, ok := shDict.Get(raw.NameLiteral("ShadingType")); ok {
					if n, ok := st.(raw.NumberObj); ok {
						sh.ShadingType = int(n.Int())
					}
				}
				if cs, ok := shDict.Get(raw.NameLiteral("ColorSpace")); ok {
					if n, ok := cs.(raw.NameObj); ok {
						sh.ColorSpace = semantic.ColorSpace{Name: n.Value()}
					}
				}
				if coords, ok := shDict.Get(raw.NameLiteral("Coords")); ok {
					if arr, ok := coords.(*raw.ArrayObj); ok {
						for _, it := range arr.Items {
							if n, ok := it.(raw.NumberObj); ok {
								sh.Coords = append(sh.Coords, n.Float())
							}
						}
					}
				}
				if dom, ok := shDict.Get(raw.NameLiteral("Domain")); ok {
					if arr, ok := dom.(*raw.ArrayObj); ok {
						for _, it := range arr.Items {
							if n, ok := it.(raw.NumberObj); ok {
								sh.Domain = append(sh.Domain, n.Float())
							}
						}
					}
				}
				if ref, ok := entry.(raw.RefObj); ok {
					if stream, ok := doc.Objects[ref.Ref()].(*raw.StreamObj); ok {
						sh.Function = decodeStream(stream)
					}
				}
				res.Shadings[name] = sh
			}
		}
	}
	return res
}

// splitTokens is a small, naive tokenizer for content streams.
type token struct {
	kind  string
	text  string
	bytes []byte
	num   float64
	pos   int
}

// lexTokens parses PDF content tokens including strings, hex strings, arrays, and dictionaries.
func lexTokens(data []byte) []token {
	var tokens []token
	for i := 0; i < len(data); {
		c := data[i]
		// Skip whitespace and comments.
		if isWhite(c) {
			i++
			continue
		}
		if c == '%' {
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			continue
		}
		switch c {
		case '[':
			tokens = append(tokens, token{kind: "arrayStart"})
			i++
			continue
		case ']':
			tokens = append(tokens, token{kind: "arrayEnd"})
			i++
			continue
		case '<':
			if i+1 < len(data) && data[i+1] == '<' {
				tokens = append(tokens, token{kind: "dictStart"})
				i += 2
				continue
			}
			j := i + 1
			for j < len(data) && data[j] != '>' {
				j++
			}
			if j < len(data) {
				hexBytes := parseHexString(data[i+1 : j])
				tokens = append(tokens, token{kind: "hex", bytes: hexBytes})
				i = j + 1
				continue
			}
		case '>':
			if i+1 < len(data) && data[i+1] == '>' {
				tokens = append(tokens, token{kind: "dictEnd"})
				i += 2
				continue
			}
		case '/':
			j := i + 1
			for j < len(data) && !isDelimiter(data[j]) {
				j++
			}
			tokens = append(tokens, token{kind: "name", text: string(data[i+1 : j]), pos: i})
			i = j
			continue
		case '(':
			str, next := readLiteralString(data, i+1)
			tokens = append(tokens, token{kind: "string", bytes: []byte(str), pos: i})
			i = next
			continue
		}

		// Number or operator
		j := i
		for j < len(data) && !isDelimiter(data[j]) {
			j++
		}
		if j == i {
			i++
			continue
		}
		text := string(data[i:j])
		if num, err := strconv.ParseFloat(text, 64); err == nil {
			tokens = append(tokens, token{kind: "number", num: num, pos: i})
		} else {
			tokens = append(tokens, token{kind: "op", text: text, pos: i})
		}
		i = j
	}
	return tokens
}

func parseArrayOperand(tokens []token, idx int) (semantic.ArrayOperand, int) {
	values := []semantic.Operand{}
	for idx < len(tokens) {
		tok := tokens[idx]
		switch tok.kind {
		case "arrayEnd":
			return semantic.ArrayOperand{Values: values}, idx
		case "arrayStart":
			op, next := parseArrayOperand(tokens, idx+1)
			values = append(values, op)
			idx = next
		case "dictStart":
			op, next := parseDictOperand(tokens, idx+1)
			values = append(values, op)
			idx = next
		case "number":
			values = append(values, semantic.NumberOperand{Value: tok.num})
		case "name":
			values = append(values, semantic.NameOperand{Value: tok.text})
		case "string", "hex":
			values = append(values, semantic.StringOperand{Value: tok.bytes})
		default:
			// treat unexpected as operator token wrapped as name
			if tok.text != "" {
				values = append(values, semantic.NameOperand{Value: tok.text})
			}
		}
		idx++
	}
	return semantic.ArrayOperand{Values: values}, idx
}

func parseDictOperand(tokens []token, idx int) (semantic.DictOperand, int) {
	values := make(map[string]semantic.Operand)
	for idx < len(tokens) {
		tok := tokens[idx]
		if tok.kind == "dictEnd" {
			return semantic.DictOperand{Values: values}, idx
		}
		if tok.kind != "name" || idx+1 >= len(tokens) {
			idx++
			continue
		}
		key := tok.text
		idx++
		switch tokens[idx].kind {
		case "number":
			values[key] = semantic.NumberOperand{Value: tokens[idx].num}
		case "name":
			values[key] = semantic.NameOperand{Value: tokens[idx].text}
		case "string", "hex":
			values[key] = semantic.StringOperand{Value: tokens[idx].bytes}
		case "arrayStart":
			op, next := parseArrayOperand(tokens, idx+1)
			values[key] = op
			idx = next
		case "dictStart":
			op, next := parseDictOperand(tokens, idx+1)
			values[key] = op
			idx = next
		}
		idx++
	}
	return semantic.DictOperand{Values: values}, idx
}

func isWhite(b byte) bool {
	return b == 0 || b == 9 || b == 10 || b == 12 || b == 13 || b == 32
}

func isDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return isWhite(b)
}

func readLiteralString(data []byte, start int) (string, int) {
	var buf strings.Builder
	level := 1
	i := start
	for i < len(data) {
		c := data[i]
		if c == '\\' && i+1 < len(data) {
			i++
			buf.WriteByte(data[i])
		} else if c == '(' {
			level++
			buf.WriteByte(c)
		} else if c == ')' {
			level--
			if level == 0 {
				return buf.String(), i + 1
			}
			buf.WriteByte(c)
		} else {
			buf.WriteByte(c)
		}
		i++
	}
	return buf.String(), i
}

func parseHexString(data []byte) []byte {
	trim := strings.Map(func(r rune) rune {
		if isHexWhite(byte(r)) {
			return -1
		}
		return r
	}, string(data))
	if len(trim)%2 == 1 {
		trim += "0"
	}
	out := make([]byte, len(trim)/2)
	for i := 0; i < len(trim); i += 2 {
		val, err := strconv.ParseUint(trim[i:i+2], 16, 8)
		if err != nil {
			return []byte(trim)
		}
		out[i/2] = byte(val)
	}
	return out
}

func isHexWhite(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t' || b == '\f'
}

// extractInlineImages scans for inline images (BI ... ID ... EI) and returns them.
func extractInlineImages(data []byte) []InlineImage {
	var imgs []InlineImage
	i := 0
	for i < len(data)-2 {
		if !isWhite(data[i]) && data[i] != 'B' {
			i++
			continue
		}
		// Align to token boundary
		for i < len(data) && isWhite(data[i]) {
			i++
		}
		if i+2 > len(data) || data[i] != 'B' || data[i+1] != 'I' || !isDelimiter(data[i+2]) {
			i++
			continue
		}
		i += 2
		for i < len(data) && isWhite(data[i]) {
			i++
		}
		dictStart := i
		idPos := findToken(data, i, "ID")
		if idPos == -1 {
			break
		}
		dictBytes := data[dictStart:idPos]
		dictTok := lexTokens(dictBytes)
		var dictOp semantic.DictOperand
		for idx := 0; idx < len(dictTok); idx++ {
			if dictTok[idx].kind == "dictStart" {
				op, next := parseDictOperand(dictTok, idx+1)
				dictOp = op
				idx = next
				break
			}
		}
		dataStart := idPos + 2
		for dataStart < len(data) && isWhite(data[dataStart]) {
			dataStart++
		}
		eiPos := findToken(data, dataStart, "EI")
		if eiPos == -1 {
			break
		}
		imgData := bytes.TrimRight(data[dataStart:eiPos], "\r\n\t ")
		imgs = append(imgs, InlineImage{Dict: dictOp, Data: imgData})
		i = eiPos + 2
	}
	return imgs
}

func findToken(data []byte, start int, token string) int {
	tlen := len(token)
	for i := start; i <= len(data)-tlen; i++ {
		if !isDelimiterBefore(data, i) {
			continue
		}
		if string(data[i:i+tlen]) == token {
			if i+tlen == len(data) || isDelimiter(data[i+tlen]) {
				return i
			}
		}
	}
	return -1
}

func isDelimiterBefore(data []byte, idx int) bool {
	if idx == 0 {
		return true
	}
	return isDelimiter(data[idx-1])
}

type usage struct {
	fonts    []string
	xobjects []string
	patterns []string
	shadings []string
}

func collectUsage(ops []semantic.Operation) usage {
	fset := map[string]struct{}{}
	xset := map[string]struct{}{}
	pat := map[string]struct{}{}
	sh := map[string]struct{}{}
	for _, op := range ops {
		switch op.Operator {
		case "Tf":
			if len(op.Operands) >= 1 {
				if n, ok := op.Operands[0].(semantic.NameOperand); ok {
					fset[n.Value] = struct{}{}
				}
			}
		case "Do":
			if len(op.Operands) >= 1 {
				if n, ok := op.Operands[0].(semantic.NameOperand); ok {
					xset[n.Value] = struct{}{}
				}
			}
		case "sh":
			if len(op.Operands) >= 1 {
				if n, ok := op.Operands[0].(semantic.NameOperand); ok {
					sh[n.Value] = struct{}{}
				}
			}
		case "scn", "SCN", "cs", "CS":
			for _, operand := range op.Operands {
				if n, ok := operand.(semantic.NameOperand); ok && strings.HasPrefix(n.Value, "Pattern") {
					pat[n.Value] = struct{}{}
				}
			}
		}
	}
	return usage{fonts: keys(fset), xobjects: keys(xset), patterns: keys(pat), shadings: keys(sh)}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type readerAtAdapter struct{ ReaderAt }

func (r readerAtAdapter) ReadAt(p []byte, off int64) (int, error) {
	return r.ReaderAt.ReadAt(p, off)
}
