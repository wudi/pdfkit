package xref

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"pdflib/filters"
	"pdflib/ir/raw"
	"pdflib/recovery"
	"pdflib/scanner"
)

// Table holds object offsets for classic and stream xref data.
type Table interface {
	Lookup(objNum int) (offset int64, gen int, found bool)
	ObjStream(objNum int) (streamObj int, index int, ok bool)
	Objects() []int
	Type() string
}

// Resolver locates and parses xref information in a PDF.
type Resolver interface {
	Resolve(ctx context.Context, r io.ReaderAt) (Table, error)
	Trailer() *raw.DictObj
	Trailers() []*raw.DictObj
	Linearized() bool
	Incremental() []Table
}

type ResolverConfig struct {
	MaxXRefDepth int
	Recovery     recovery.Strategy
}

// NewResolver returns an xref resolver that follows Prev chains and understands xref streams.
func NewResolver(cfg ResolverConfig) Resolver { return &tableResolver{cfg: cfg} }

type tableResolver struct {
	cfg        ResolverConfig
	trailers   []*raw.DictObj
	sections   []Table
	linearized bool
}

func (t *tableResolver) Resolve(ctx context.Context, r io.ReaderAt) (Table, error) {
	data := readAll(r)
	startOffset, err := findStartXRef(data)
	if err != nil {
		return nil, err
	}
	t.linearized = detectLinearized(data)

	var sections []Table
	var trailers []*raw.DictObj
	seen := make(map[int64]struct{})
	maxDepth := t.cfg.MaxXRefDepth
	if maxDepth == 0 {
		maxDepth = 32
	}

	for off, depth := startOffset, 0; off > 0; depth++ {
		if _, ok := seen[off]; ok {
			return nil, errors.New("xref loop detected")
		}
		if depth >= maxDepth {
			return nil, errors.New("xref depth exceeded")
		}
		seen[off] = struct{}{}

		tbl, trailer, prev, err := parseSection(ctx, data, off, t.cfg)
		if err != nil {
			return nil, err
		}
		sections = append(sections, tbl)
		if trailer != nil {
			trailers = append(trailers, trailer)
			if xrefStmOff, ok := xrefStmOffset(trailer); ok {
				if _, exists := seen[xrefStmOff]; exists {
					return nil, errors.New("xref loop detected (xrefstm)")
				}
				seen[xrefStmOff] = struct{}{}
				stmTable, stmTrailer, stmPrev, err := parseXRefStream(ctx, data, xrefStmOff, t.cfg)
				if err != nil {
					return nil, err
				}
				sections = append(sections, stmTable)
				if stmTrailer != nil {
					trailers = append(trailers, stmTrailer)
				}
				if prev == xrefStmOff {
					prev = stmPrev
				} else if prev == 0 && stmPrev > 0 {
					prev = stmPrev
				}
			}
		}
		if prev <= 0 {
			break
		}
		off = prev
	}

	if len(sections) == 0 {
		return nil, errors.New("xref sections not found")
	}
	t.trailers = trailers
	if len(sections) > 1 {
		t.sections = sections[1:]
	}
	merged := mergeTables(sections)
	if len(trailers) > 0 {
		if err := validateTrailer(trailers[0], merged); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

func (t *tableResolver) Trailer() *raw.DictObj {
	if len(t.trailers) == 0 {
		return nil
	}
	return t.trailers[0]
}
func (t *tableResolver) Trailers() []*raw.DictObj { return t.trailers }
func (t *tableResolver) Linearized() bool         { return t.linearized }
func (t *tableResolver) Incremental() []Table     { return t.sections }

type entry struct {
	offset int64
	gen    int
}

type table struct {
	entries map[int]entry
	trailer *raw.DictObj
}

func (t *table) Lookup(objNum int) (int64, int, bool) {
	e, ok := t.entries[objNum]
	if !ok {
		return 0, 0, false
	}
	return e.offset, e.gen, true
}
func (t *table) ObjStream(objNum int) (int, int, bool) { return 0, 0, false }
func (t *table) Objects() []int {
	out := make([]int, 0, len(t.entries))
	for k := range t.entries {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
func (t *table) Type() string { return "table" }

// streamTable supports xref streams with object stream references.
type streamTable struct {
	offsets   map[int]entry
	objStream map[int]struct {
		objstm int
		idx    int
	}
	trailer *raw.DictObj
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

func mergeTables(sections []Table) Table { return &mergedTable{sections: sections} }

type mergedTable struct {
	sections []Table // newest -> oldest
}

func (m *mergedTable) Lookup(objNum int) (int64, int, bool) {
	for _, tbl := range m.sections {
		if off, gen, ok := tbl.Lookup(objNum); ok {
			return off, gen, true
		}
	}
	return 0, 0, false
}
func (m *mergedTable) ObjStream(objNum int) (int, int, bool) {
	for _, tbl := range m.sections {
		if os, idx, ok := tbl.ObjStream(objNum); ok {
			return os, idx, true
		}
	}
	return 0, 0, false
}
func (m *mergedTable) Objects() []int {
	seen := make(map[int]struct{})
	for _, tbl := range m.sections {
		for _, obj := range tbl.Objects() {
			if _, ok := seen[obj]; ok {
				continue
			}
			seen[obj] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
func (m *mergedTable) Type() string {
	if len(m.sections) == 0 {
		return "merged-xref"
	}
	return m.sections[0].Type()
}

func (m *mergedTable) maxObjectNumber() int {
	max := 0
	for _, tbl := range m.sections {
		for _, obj := range tbl.Objects() {
			if obj > max {
				max = obj
			}
		}
	}
	return max
}

func parseSection(ctx context.Context, data []byte, offset int64, cfg ResolverConfig) (Table, *raw.DictObj, int64, error) {
	s := scanner.New(bytes.NewReader(data), scanner.Config{Recovery: cfg.Recovery})
	if err := s.Seek(offset); err != nil {
		return nil, nil, 0, err
	}
	tok, err := s.Next()
	if err != nil {
		return nil, nil, 0, err
	}
	if tok.Type == scanner.TokenKeyword && tok.Value == "xref" {
		return parseClassic(s)
	}
	return parseXRefStream(ctx, data, offset, cfg)
}

// parseClassic parses a traditional xref table (already positioned at the "xref" keyword).
func parseClassic(s scanner.Scanner) (Table, *raw.DictObj, int64, error) {
	entries := make(map[int]entry)
	for {
		tok, err := s.Next()
		if err != nil {
			return nil, nil, 0, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Value == "trailer" {
			break
		}
		if tok.Type != scanner.TokenNumber {
			return nil, nil, 0, errors.New("invalid xref subsection header")
		}
		startObj := int(toIntValue(tok.Value))
		countTok, err := s.Next()
		if err != nil {
			return nil, nil, 0, err
		}
		count := int(toIntValue(countTok.Value))
		for i := 0; i < count; i++ {
			offTok, err := s.Next()
			if err != nil {
				return nil, nil, 0, err
			}
			genTok, err := s.Next()
			if err != nil {
				return nil, nil, 0, err
			}
			flagTok, err := s.Next()
			if err != nil {
				return nil, nil, 0, err
			}
			flag, _ := flagTok.Value.(string)
			if len(flag) > 0 && flag[0] == 'n' {
				entries[startObj+i] = entry{offset: toIntValue(offTok.Value), gen: int(toIntValue(genTok.Value))}
			}
		}
	}
	tr := &streamTokenReader{s: s}
	trailerObj, err := parseObject(tr)
	if err != nil {
		return nil, nil, 0, err
	}
	dict, ok := trailerObj.(*raw.DictObj)
	if !ok {
		return nil, nil, 0, errors.New("trailer must be dictionary")
	}
	prev := int64(0)
	if p, ok := dict.Get(raw.NameObj{Val: "Prev"}); ok {
		prev = toInt64(p)
	}
	return &table{entries: entries, trailer: dict}, dict, prev, nil
}

// parseXRefStream decodes a cross-reference stream at the given offset.
func parseXRefStream(ctx context.Context, data []byte, offset int64, cfg ResolverConfig) (Table, *raw.DictObj, int64, error) {
	if offset < 0 || offset >= int64(len(data)) {
		return nil, nil, 0, errors.New("xref stream offset out of range")
	}
	s := scanner.New(bytes.NewReader(data), scanner.Config{Recovery: cfg.Recovery})
	if err := s.Seek(offset); err != nil {
		return nil, nil, 0, err
	}
	// Expect "<obj> <gen> obj"
	tokObjNum, err := s.Next()
	if err != nil {
		return nil, nil, 0, err
	}
	if tokObjNum.Type != scanner.TokenNumber {
		return nil, nil, 0, errors.New("xref stream missing object number")
	}
	on := int(toIntValue(tokObjNum.Value))
	tokGen, err := s.Next()
	if err != nil {
		return nil, nil, 0, err
	}
	gen := int(toIntValue(tokGen.Value))
	tokKW, err := s.Next()
	if err != nil || tokKW.Type != scanner.TokenKeyword || tokKW.Value != "obj" {
		return nil, nil, 0, errors.New("xref stream missing obj keyword")
	}

	tr := &streamTokenReader{s: s}
	obj, err := parseObject(tr)
	if err != nil {
		return nil, nil, 0, err
	}
	dict, ok := obj.(*raw.DictObj)
	if !ok {
		return nil, nil, 0, errors.New("xref stream must start with dictionary")
	}
	streamTok, err := tr.next()
	if err != nil || streamTok.Type != scanner.TokenStream {
		return nil, nil, 0, errors.New("xref stream payload missing")
	}
	streamData := streamTok.Value.([]byte)
	if fTok, ok := dict.Get(raw.NameObj{Val: "Filter"}); ok {
		filterNames, filterParams := toFilters(fTok, dict)
		p := filters.NewPipeline([]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewRunLengthDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
			filters.NewCryptDecoder(),
		}, filters.Limits{})
		decoded, err := p.Decode(ctx, streamData, filterNames, filterParams)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("decode xref stream: %w", err)
		}
		streamData = decoded
	}
	wArrObj, ok := dict.Get(raw.NameObj{Val: "W"})
	if !ok {
		return nil, nil, 0, errors.New("xref stream missing W")
	}
	w := toIntArray(wArrObj)
	if len(w) != 3 {
		return nil, nil, 0, errors.New("xref stream W must have 3 integers")
	}
	sizeObj, ok := dict.Get(raw.NameObj{Val: "Size"})
	if !ok {
		return nil, nil, 0, errors.New("xref stream missing Size")
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
	}), trailer: dict}
	cursor := 0
	entrySize := w[0] + w[1] + w[2]
	for i := 0; i < len(indexes); i += 2 {
		startObj := indexes[i]
		count := indexes[i+1]
		for j := 0; j < count; j++ {
			if cursor+entrySize > len(streamData) {
				return nil, nil, 0, errors.New("xref stream truncated")
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

	prev := int64(0)
	if p, ok := dict.Get(raw.NameObj{Val: "Prev"}); ok {
		prev = toInt64(p)
	}

	return st, dict, prev, nil
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
		case int:
			return raw.NumberObj{I: int64(v), IsInt: true}, nil
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

func xrefStmOffset(trailer *raw.DictObj) (int64, bool) {
	if trailer == nil {
		return 0, false
	}
	v, ok := trailer.Get(raw.NameObj{Val: "XRefStm"})
	if !ok {
		return 0, false
	}
	return toInt64(v), true
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
	switch n := obj.(type) {
	case raw.NumberObj:
		return n.Int()
	case raw.RefObj:
		// Prev entry may not be indirect, ignore to avoid recursion in resolver.
		return 0
	default:
		return 0
	}
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

func findStartXRef(data []byte) (int64, error) {
	startxref := bytes.LastIndex(data, []byte("startxref"))
	if startxref < 0 {
		return 0, errors.New("startxref not found")
	}
	rest := bytes.TrimSpace(data[startxref+len("startxref"):])
	end := bytes.IndexAny(rest, "\r\n ")
	if end < 0 {
		end = len(rest)
	}
	val, err := strconv.ParseInt(string(rest[:end]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse startxref: %w", err)
	}
	if val < 0 {
		return 0, errors.New("negative startxref offset")
	}
	return val, nil
}

func toIntValue(v interface{}) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func detectLinearized(data []byte) bool {
	s := scanner.New(bytes.NewReader(data), scanner.Config{})
	// Skip header
	for {
		tok, err := s.Next()
		if err != nil {
			return false
		}
		if tok.Type != scanner.TokenNumber {
			continue
		}
		tokGen, err := s.Next()
		if err != nil || tokGen.Type != scanner.TokenNumber {
			return false
		}
		tokObj, err := s.Next()
		if err != nil || tokObj.Type != scanner.TokenKeyword || tokObj.Value != "obj" {
			return false
		}
		tr := &streamTokenReader{s: s}
		obj, err := parseObject(tr)
		if err != nil {
			return false
		}
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			return false
		}
		val, ok := dict.Get(raw.NameObj{Val: "Linearized"})
		if !ok {
			return false
		}
		if num, ok := val.(raw.NumberObj); !ok || num.Float() <= 0 {
			return false
		}
		// Check minimal required keys for linearization dictionary: L, O, N, H
		if _, ok := dict.Get(raw.NameObj{Val: "L"}); !ok {
			return false
		}
		if _, ok := dict.Get(raw.NameObj{Val: "O"}); !ok {
			return false
		}
		if _, ok := dict.Get(raw.NameObj{Val: "N"}); !ok {
			return false
		}
		if _, ok := dict.Get(raw.NameObj{Val: "H"}); !ok {
			return false
		}
		return true
	}
}

func validateTrailer(tr *raw.DictObj, tbl Table) error {
	if tr == nil {
		return errors.New("trailer missing")
	}
	if _, ok := tr.Get(raw.NameObj{Val: "Root"}); !ok {
		return errors.New("trailer missing Root")
	}
	sizeObj, ok := tr.Get(raw.NameObj{Val: "Size"})
	if !ok {
		return errors.New("trailer missing Size")
	}
	size := toInt64(sizeObj)
	if size <= 0 {
		return errors.New("trailer Size invalid")
	}
	maxObj := 0
	if mt, ok := tbl.(*mergedTable); ok {
		maxObj = mt.maxObjectNumber()
	}
	if mt, ok := tbl.(*table); ok {
		for _, obj := range mt.Objects() {
			if obj > maxObj {
				maxObj = obj
			}
		}
	}
	if st, ok := tbl.(*streamTable); ok {
		for _, obj := range st.Objects() {
			if obj > maxObj {
				maxObj = obj
			}
		}
	}
	if int64(maxObj)+1 > size {
		return fmt.Errorf("trailer Size %d smaller than max object %d", size, maxObj)
	}
	return nil
}
