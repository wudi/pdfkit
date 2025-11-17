package parser

import (
	"context"
	"errors"
	"io"
	"sync"

	"pdflib/ir/raw"
	"pdflib/recovery"
	"pdflib/scanner"
	"pdflib/security"
	"pdflib/xref"
)

type Cache interface {
	Get(ref raw.ObjectRef) (raw.Object, bool)
	Put(ref raw.ObjectRef, obj raw.Object)
}

type ObjectLoader interface {
	Load(ctx context.Context, ref raw.ObjectRef) (raw.Object, error)
	LoadIndirect(ctx context.Context, ref raw.ObjectRef, depth int) (raw.Object, error)
}

type ObjectLoaderBuilder struct {
	reader    io.ReaderAt
	xrefTable xref.Table
	scanner   scanner.Scanner
	security  security.Handler
	maxDepth  int
	cache     Cache
	recovery  recovery.Strategy
}

func (b *ObjectLoaderBuilder) WithXRef(table xref.Table) *ObjectLoaderBuilder {
	b.xrefTable = table
	return b
}
func (b *ObjectLoaderBuilder) WithSecurity(h security.Handler) *ObjectLoaderBuilder {
	b.security = h
	return b
}
func (b *ObjectLoaderBuilder) WithCache(c Cache) *ObjectLoaderBuilder { b.cache = c; return b }

func (b *ObjectLoaderBuilder) Build() (ObjectLoader, error) {
	if b.reader == nil || b.xrefTable == nil {
		return nil, errors.New("reader and xrefTable required")
	}
	return &objectLoader{reader: b.reader, xrefTable: b.xrefTable, scanner: b.scanner, security: b.security, maxDepth: b.maxDepth, cache: b.cache, recovery: b.recovery}, nil
}

type objectLoader struct {
	reader    io.ReaderAt
	xrefTable xref.Table
	scanner   scanner.Scanner
	security  security.Handler
	maxDepth  int
	cache     Cache
	recovery  recovery.Strategy
	mu        sync.Mutex
}

func (o *objectLoader) Load(ctx context.Context, ref raw.ObjectRef) (raw.Object, error) {
	if o.cache != nil {
		if obj, ok := o.cache.Get(ref); ok {
			return obj, nil
		}
	}

	obj, err := o.loadOnce(ctx, ref)
	if err != nil {
		return nil, err
	}

	if o.cache != nil {
		o.cache.Put(ref, obj)
	}
	return obj, nil
}
func (o *objectLoader) LoadIndirect(ctx context.Context, ref raw.ObjectRef, depth int) (raw.Object, error) {
	if depth > o.maxDepth {
		return nil, errors.New("max depth exceeded")
	}
	return o.Load(ctx, ref)
}

func (o *objectLoader) loadOnce(ctx context.Context, ref raw.ObjectRef) (raw.Object, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.xrefTable == nil {
		return nil, errors.New("xref table missing")
	}
	offset, gen, found := o.xrefTable.Lookup(ref.Num)
	if !found {
		return nil, errors.New("object not found in xref")
	}

	// Build a fresh scanner per load to avoid shared cursor complications.
	s := scanner.New(o.reader, scanner.Config{Recovery: o.recovery})
	if err := s.Seek(offset); err != nil {
		return nil, err
	}
	tr := &tokenReader{s: NewStreamAware(s)}

	// Expect "<objNum> <gen> obj"
	tokNum, err := tr.next()
	if err != nil {
		return nil, err
	}
	num, ok := toInt(tokNum.Value)
	if tokNum.Type != scanner.TokenNumber || !ok || int(num) != ref.Num {
		return nil, errors.New("object header number mismatch")
	}
	tokGen, err := tr.next()
	if err != nil {
		return nil, err
	}
	genVal, ok := toInt(tokGen.Value)
	if tokGen.Type != scanner.TokenNumber || !ok || int(genVal) != gen {
		return nil, errors.New("object header generation mismatch")
	}
	tokObj, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tokObj.Type != scanner.TokenKeyword || tokObj.Value != "obj" {
		return nil, errors.New("expected obj keyword")
	}

	obj, err := parseObject(tr)
	if err != nil {
		return nil, err
	}
	if dict, ok := obj.(*raw.DictObj); ok {
		if streamTok, err := tr.next(); err == nil && streamTok.Type == scanner.TokenStream {
			obj = raw.NewStream(dict, copyBytes(streamTok.Value))
		}
	}

	return obj, nil
}

// Parsing helpers (duplicated from raw parser for loader-focused parsing).

type tokenReader struct {
	s   interface{ Next() (scanner.Token, error) }
	buf []scanner.Token
}

func (r *tokenReader) next() (scanner.Token, error) {
	if l := len(r.buf); l > 0 {
		t := r.buf[l-1]
		r.buf = r.buf[:l-1]
		return t, nil
	}
	return r.s.Next()
}

func (r *tokenReader) unread(tok scanner.Token) { r.buf = append(r.buf, tok) }

func parseObject(tr *tokenReader) (raw.Object, error) {
	tok, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tok.Type == scanner.TokenKeyword && tok.Value == "endobj" {
		return nil, errors.New("unexpected endobj")
	}
	switch tok.Type {
	case scanner.TokenName:
		if v, ok := tok.Value.(string); ok {
			return raw.NameObj{Val: v}, nil
		}
	case scanner.TokenNumber:
		if i, ok := toInt(tok.Value); ok {
			return raw.NumberObj{I: i, IsInt: true}, nil
		}
		if f, ok := tok.Value.(float64); ok {
			return raw.NumberObj{F: f, IsInt: false}, nil
		}
	case scanner.TokenBoolean:
		if v, ok := tok.Value.(bool); ok {
			return raw.BoolObj{V: v}, nil
		}
	case scanner.TokenNull:
		return raw.NullObj{}, nil
	case scanner.TokenString:
		if b, ok := tok.Value.([]byte); ok {
			return raw.StringObj{Bytes: b}, nil
		}
	case scanner.TokenArray:
		return parseArray(tr)
	case scanner.TokenDict:
		return parseDict(tr)
	case scanner.TokenRef:
		if v, ok := tok.Value.(struct{ Num, Gen int }); ok {
			return raw.RefObj{R: raw.ObjectRef{Num: v.Num, Gen: v.Gen}}, nil
		}
	}
	return nil, errors.New("unexpected token")
}

func parseArray(tr *tokenReader) (raw.Object, error) {
	arr := &raw.ArrayObj{}
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Value == "]" {
			break
		}
		tr.unread(tok)
		item, err := parseObject(tr)
		if err != nil {
			return nil, err
		}
		arr.Append(item)
	}
	return arr, nil
}

func parseDict(tr *tokenReader) (raw.Object, error) {
	d := raw.Dict()
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Value == ">>" {
			break
		}
		if tok.Type != scanner.TokenName {
			return nil, errors.New("expected name in dict")
		}
		key, _ := tok.Value.(string)
		val, err := parseObject(tr)
		if err != nil {
			return nil, err
		}
		d.Set(raw.NameObj{Val: key}, val)
	}
	return d, nil
}

func toInt(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func copyBytes(v interface{}) []byte {
	b, ok := v.([]byte)
	if !ok {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
