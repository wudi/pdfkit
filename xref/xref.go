package xref

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"pdflib/filters"
	"pdflib/ir/raw"
	"pdflib/recovery"
	"pdflib/scanner"
)

// Table holds object offsets for a classic xref table.
type Table interface {
	Lookup(objNum int) (offset int64, gen int, found bool)
	ObjStream(objNum int) (streamObj int, index int, ok bool)
	Objects() []int
	Type() string
}

// Resolver locates and parses xref information in a PDF.
type Resolver interface {
	Resolve(ctx context.Context, r io.ReaderAt) (Table, error)
	Linearized() bool
	Incremental() []Table
}

type ResolverConfig struct {
	MaxXRefDepth int
	Recovery     recovery.Strategy
}

// NewResolver returns a basic classic-table resolver.
func NewResolver(cfg ResolverConfig) Resolver {
	return &tableResolver{}
}

// tableResolver implements classic (non-stream) xref parsing for simple PDFs.
type tableResolver struct{}

func (t *tableResolver) Resolve(ctx context.Context, r io.ReaderAt) (Table, error) {
	data := readAll(r)

	startxref := bytes.LastIndex(data, []byte("startxref"))
	if startxref < 0 {
		return nil, errors.New("startxref not found")
	}
	rest := data[startxref+len("startxref"):]
	lines := bufio.NewScanner(bytes.NewReader(rest))
	var offset int64
	for lines.Scan() {
		text := strings.TrimSpace(lines.Text())
		if text == "" {
			continue
		}
		val, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse startxref: %w", err)
		}
		offset = val
		break
	}

	if offset <= 0 || offset >= int64(len(data)) {
		return nil, fmt.Errorf("xref offset out of range: %d", offset)
	}

	tableData := data[offset:]
	sc := bufio.NewScanner(bytes.NewReader(tableData))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "xref" {
		// Try xref stream at this offset
		st, err := parseXRefStream(ctx, data, offset)
		if err != nil {
			return nil, fmt.Errorf("xref keyword not found at offset: %w", err)
		}
		return st, nil
	}

	entries := make(map[int]entry)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "trailer") {
			break
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid xref subsection header: %q", line)
		}
		startObj, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse xref start: %w", err)
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse xref count: %w", err)
		}

		for i := 0; i < count; i++ {
			if !sc.Scan() {
				return nil, errors.New("unexpected end of xref section")
			}
			entryLine := strings.TrimSpace(sc.Text())
			fields := strings.Fields(entryLine)
			if len(fields) < 3 {
				return nil, fmt.Errorf("invalid xref entry: %q", entryLine)
			}
			off, err := strconv.ParseInt(fields[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse xref offset: %w", err)
			}
			gen, err := strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("parse xref gen: %w", err)
			}
			if len(fields[2]) == 0 || fields[2][0] != 'n' {
				continue // free entry
			}
			entries[startObj+i] = entry{offset: off, gen: gen}
		}
	}

	return &table{entries: entries}, nil
}

func (t *tableResolver) Linearized() bool     { return false }
func (t *tableResolver) Incremental() []Table { return nil }

type entry struct {
	offset int64
	gen    int
}

type table struct {
	entries map[int]entry
}

func (t *table) Lookup(objNum int) (int64, int, bool) {
	e, ok := t.entries[objNum]
	if !ok {
		return 0, 0, false
	}
	return e.offset, e.gen, true
}

func (t *table) Objects() []int {
	out := make([]int, 0, len(t.entries))
	for k := range t.entries {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func (t *table) Type() string                          { return "table" }
func (t *table) ObjStream(objNum int) (int, int, bool) { return 0, 0, false }

// streamTable supports xref streams with object stream references.
type streamTable struct {
	offsets   map[int]entry
	objStream map[int]struct {
		objstm int
		idx    int
	}
}

func (t *streamTable) Lookup(objNum int) (int64, int, bool) {
	if e, ok := t.offsets[objNum]; ok {
		return e.offset, e.gen, true
	}
	return 0, 0, false
}

func (t *streamTable) ObjStream(objNum int) (int, int, bool) {
	if e, ok := t.objStream[objNum]; ok {
		return e.objstm, e.idx, true
	}
	return 0, 0, false
}

func (t *streamTable) Objects() []int {
	seen := make(map[int]struct{})
	for k := range t.offsets {
		seen[k] = struct{}{}
	}
	for k := range t.objStream {
		seen[k] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func (t *streamTable) Type() string { return "xref-stream" }

// parseXRefStream decodes a cross-reference stream at the given offset.
func parseXRefStream(ctx context.Context, data []byte, offset int64) (Table, error) {
	s := scanner.New(bytes.NewReader(data), scanner.Config{})
	if err := s.Seek(offset); err != nil {
		return nil, err
	}
	// Expect "<obj> <gen> obj"
	tokObjNum, err := s.Next()
	if err != nil {
		return nil, err
	}
	if tokObjNum.Type != scanner.TokenNumber {
		return nil, errors.New("xref stream missing object number")
	}
	on, _ := strconv.Atoi(fmt.Sprint(tokObjNum.Value))
	tokGen, err := s.Next()
	if err != nil {
		return nil, err
	}
	gen, _ := strconv.Atoi(fmt.Sprint(tokGen.Value))
	tokKW, err := s.Next()
	if err != nil || tokKW.Type != scanner.TokenKeyword || tokKW.Value != "obj" {
		return nil, errors.New("xref stream missing obj keyword")
	}

	tr := &streamTokenReader{s: s}
	obj, err := parseObject(tr)
	if err != nil {
		return nil, err
	}
	dict, ok := obj.(*raw.DictObj)
	if !ok {
		return nil, errors.New("xref stream must start with dictionary")
	}
	streamTok, err := tr.next()
	if err != nil || streamTok.Type != scanner.TokenStream {
		return nil, errors.New("xref stream payload missing")
	}
	streamData := streamTok.Value.([]byte)
	if fTok, ok := dict.Get(raw.NameObj{Val: "Filter"}); ok {
		filterNames, filterParams := toFilters(fTok, dict)
		p := filters.NewPipeline([]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
		}, filters.Limits{})
		decoded, err := p.Decode(ctx, streamData, filterNames, filterParams)
		if err != nil {
			return nil, fmt.Errorf("decode xref stream: %w", err)
		}
		streamData = decoded
	}
	wArrObj, ok := dict.Get(raw.NameObj{Val: "W"})
	if !ok {
		return nil, errors.New("xref stream missing W")
	}
	w := toIntArray(wArrObj)
	if len(w) != 3 {
		return nil, errors.New("xref stream W must have 3 integers")
	}
	sizeObj, ok := dict.Get(raw.NameObj{Val: "Size"})
	if !ok {
		return nil, errors.New("xref stream missing Size")
	}
	size := toInt64(sizeObj)
	indexes := []int{0, int(size)}
	if idxObj, ok := dict.Get(raw.NameObj{Val: "Index"}); ok {
		idxArr := toIntArray(idxObj)
		if len(idxArr)%2 == 0 && len(idxArr) > 0 {
			indexes = idxArr
		}
	}

	st := &streamTable{offsets: make(map[int]entry), objStream: make(map[int]struct {
		objstm int
		idx    int
	})}
	cursor := 0
	entrySize := w[0] + w[1] + w[2]
	for i := 0; i < len(indexes); i += 2 {
		startObj := indexes[i]
		count := indexes[i+1]
		for j := 0; j < count; j++ {
			if cursor+entrySize > len(streamData) {
				return nil, errors.New("xref stream truncated")
			}
			fields := streamData[cursor : cursor+entrySize]
			cursor += entrySize
			tVal := parseField(fields[:w[0]])
			f1 := parseField(fields[w[0] : w[0]+w[1]])
			f2 := parseField(fields[w[0]+w[1]:])
			objNum := startObj + j
			switch tVal {
			case 0:
				continue // free
			case 1:
				st.offsets[objNum] = entry{offset: int64(f1), gen: int(f2)}
			case 2:
				st.objStream[objNum] = struct {
					objstm int
					idx    int
				}{objstm: f1, idx: f2}
			default:
				continue
			}
		}
	}
	// Include the stream object itself
	st.offsets[on] = entry{offset: offset, gen: gen}
	return st, nil
}

func parseField(b []byte) int {
	val := 0
	for _, c := range b {
		val = (val << 8) + int(c)
	}
	return val
}

// Minimal object parser for xref streams (subset of raw parser).
type streamTokenReader struct {
	s   scanner.Scanner
	buf []scanner.Token
}

func (r *streamTokenReader) next() (scanner.Token, error) {
	if l := len(r.buf); l > 0 {
		t := r.buf[l-1]
		r.buf = r.buf[:l-1]
		return t, nil
	}
	return r.s.Next()
}
func (r *streamTokenReader) unread(t scanner.Token) { r.buf = append(r.buf, t) }

func parseObject(tr *streamTokenReader) (raw.Object, error) {
	tok, err := tr.next()
	if err != nil {
		return nil, err
	}
	switch tok.Type {
	case scanner.TokenName:
		return raw.NameObj{Val: tok.Value.(string)}, nil
	case scanner.TokenNumber:
		switch v := tok.Value.(type) {
		case int64:
			return raw.NumberObj{I: v, IsInt: true}, nil
		case float64:
			return raw.NumberObj{F: v, IsInt: false}, nil
		}
	case scanner.TokenBoolean:
		return raw.BoolObj{V: tok.Value.(bool)}, nil
	case scanner.TokenNull:
		return raw.NullObj{}, nil
	case scanner.TokenString:
		return raw.StringObj{Bytes: tok.Value.([]byte)}, nil
	case scanner.TokenArray:
		arr := raw.NewArray()
		for {
			t, err := tr.next()
			if err != nil {
				return nil, err
			}
			if t.Type == scanner.TokenKeyword && t.Value == "]" {
				break
			}
			tr.unread(t)
			it, err := parseObject(tr)
			if err != nil {
				return nil, err
			}
			arr.Append(it)
		}
		return arr, nil
	case scanner.TokenDict:
		d := raw.Dict()
		for {
			t, err := tr.next()
			if err != nil {
				return nil, err
			}
			if t.Type == scanner.TokenKeyword && t.Value == ">>" {
				break
			}
			if t.Type != scanner.TokenName {
				return nil, errors.New("expected name in dict")
			}
			key := raw.NameObj{Val: t.Value.(string)}
			val, err := parseObject(tr)
			if err != nil {
				return nil, err
			}
			d.Set(key, val)
		}
		return d, nil
	case scanner.TokenRef:
		v := tok.Value.(struct{ Num, Gen int })
		return raw.RefObj{R: raw.ObjectRef{Num: v.Num, Gen: v.Gen}}, nil
	}
	return nil, fmt.Errorf("unexpected token %v", tok.Type)
}

func toIntArray(obj raw.Object) []int {
	arr, ok := obj.(*raw.ArrayObj)
	if !ok {
		return nil
	}
	out := make([]int, 0, arr.Len())
	for _, it := range arr.Items {
		switch v := it.(type) {
		case raw.NumberObj:
			out = append(out, int(v.Int()))
		case raw.RefObj:
			_ = v
		}
	}
	return out
}

func toInt64(obj raw.Object) int64 {
	if n, ok := obj.(raw.NumberObj); ok {
		return n.Int()
	}
	return 0
}

func toFilters(filterObj raw.Object, dict *raw.DictObj) ([]string, []raw.Dictionary) {
	var names []string
	var params []raw.Dictionary
	switch v := filterObj.(type) {
	case raw.NameObj:
		names = append(names, v.Val)
	case *raw.ArrayObj:
		for _, it := range v.Items {
			if n, ok := it.(raw.NameObj); ok {
				names = append(names, n.Val)
			}
		}
	}
	if dp, ok := dict.Get(raw.NameObj{Val: "DecodeParms"}); ok {
		switch p := dp.(type) {
		case *raw.DictObj:
			params = append(params, p)
		case *raw.ArrayObj:
			for _, it := range p.Items {
				if d, ok := it.(*raw.DictObj); ok {
					params = append(params, d)
				}
			}
		}
	}
	return names, params
}

func readAll(r io.ReaderAt) []byte {
	var buf bytes.Buffer
	const chunk = int64(32 * 1024)
	for off := int64(0); ; off += chunk {
		tmp := make([]byte, chunk)
		n, err := r.ReadAt(tmp, off)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
		if int64(n) < chunk {
			break
		}
	}
	return buf.Bytes()
}
