package parser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/wudi/pdfkit/filters"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/recovery"
	"github.com/wudi/pdfkit/scanner"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
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
	limits    security.Limits
	cache     Cache
	recovery  recovery.Strategy
}

func (b *ObjectLoaderBuilder) WithXRef(table xref.Table) *ObjectLoaderBuilder {
	b.xrefTable = table
	return b
}
func (b *ObjectLoaderBuilder) WithReader(r io.ReaderAt) *ObjectLoaderBuilder {
	b.reader = r
	return b
}
func (b *ObjectLoaderBuilder) WithSecurity(h security.Handler) *ObjectLoaderBuilder {
	b.security = h
	return b
}
func (b *ObjectLoaderBuilder) WithLimits(l security.Limits) *ObjectLoaderBuilder {
	b.limits = l
	return b
}
func (b *ObjectLoaderBuilder) WithCache(c Cache) *ObjectLoaderBuilder { b.cache = c; return b }

func (b *ObjectLoaderBuilder) Build() (ObjectLoader, error) {
	if b.reader == nil || b.xrefTable == nil {
		return nil, errors.New("reader and xrefTable required")
	}
	sec := b.security
	if sec == nil {
		sec = security.NoopHandler()
	}
	maxDepth := b.maxDepth
	if maxDepth == 0 {
		maxDepth = b.limits.MaxIndirectDepth
		if maxDepth == 0 {
			maxDepth = security.DefaultLimits().MaxIndirectDepth
		}
	}
	return &objectLoader{
		reader:    b.reader,
		xrefTable: b.xrefTable,
		scanner:   b.scanner,
		security:  sec,
		maxDepth:  maxDepth,
		limits:    b.limits,
		cache:     b.cache,
		recovery:  b.recovery,
	}, nil
}

type objectLoader struct {
	reader    io.ReaderAt
	xrefTable xref.Table
	scanner   scanner.Scanner
	security  security.Handler
	maxDepth  int
	limits    security.Limits
	cache     Cache
	recovery  recovery.Strategy
	mu        sync.Mutex
	objstm    map[int]map[int]raw.Object
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
		if osNum, idx, ok := o.xrefTable.ObjStream(ref.Num); ok {
			return o.loadFromObjectStream(ctx, ref, osNum, idx)
		}
		return nil, errors.New("object not found in xref")
	}

	// Build a fresh scanner per load to avoid shared cursor complications.
	return o.loadAtOffset(ref.Num, offset, gen)
}

// loadAtOffset assumes caller holds the loader mutex.
func (o *objectLoader) loadAtOffset(objNum int, offset int64, gen int) (raw.Object, error) {
	if o.scanner == nil {
		cfg := scanner.Config{
			Recovery:        o.recovery,
			MaxStringLength: o.limits.MaxStringLength,
			MaxArrayDepth:   o.limits.MaxIndirectDepth, // Approximation
			MaxDictDepth:    o.limits.MaxIndirectDepth, // Approximation
			MaxBufferSize:   o.limits.MaxDecompressedSize,
			MaxStreamLength: o.limits.MaxStreamLength,
		}
		o.scanner = scanner.New(o.reader, cfg)
	}
	return o.scanObject(o.scanner, objNum, offset, gen)
}

func (o *objectLoader) scanObject(s scanner.Scanner, objNum int, offset int64, gen int) (raw.Object, error) {
	if err := s.SeekTo(offset); err != nil {
		return nil, err
	}
	tr := newTokenReader(s)

	// Expect "<objNum> <gen> obj"
	tokNum, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tokNum.Type != scanner.TokenNumber || !tokNum.IsInt || int(tokNum.Int) != objNum {
		return nil, errors.New("object header number mismatch")
	}
	tokGen, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tokGen.Type != scanner.TokenNumber || !tokGen.IsInt || int(tokGen.Int) != gen {
		return nil, errors.New("object header generation mismatch")
	}
	tokObj, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tokObj.Type != scanner.TokenKeyword || tokObj.Str != "obj" {
		return nil, errors.New("expected obj keyword")
	}

	obj, err := parseObject(tr, o.recovery, objNum, gen)
	if err != nil {
		return nil, err
	}
	if dict, ok := obj.(*raw.DictObj); ok {
		hint, err := o.resolveStreamLength(dict)
		if err != nil {
			return nil, err
		}
		if hint > 0 {
			tr.setStreamLengthHint(hint)
		} else {
			tr.clearStreamLengthHint()
		}
		if streamTok, err := tr.next(); err == nil && streamTok.Type == scanner.TokenStream {
			obj = raw.NewStream(dict, streamTok.Bytes)
		} else if err == nil {
			tr.unread(streamTok)
		}
	}
	return o.decryptObject(raw.ObjectRef{Num: objNum, Gen: gen}, obj)
}

func (o *objectLoader) loadFromObjectStream(ctx context.Context, ref raw.ObjectRef, objStreamNum int, idx int) (raw.Object, error) {
	if o.objstm == nil {
		o.objstm = make(map[int]map[int]raw.Object)
	}
	if objs, ok := o.objstm[objStreamNum]; ok {
		if obj, ok2 := objs[idx]; ok2 {
			return obj, nil
		}
	}
	offset, gen, ok := o.xrefTable.Lookup(objStreamNum)
	if !ok {
		return nil, errors.New("object stream entry missing")
	}
	streamObj, err := o.loadAtOffset(objStreamNum, offset, gen)
	if err != nil {
		return nil, err
	}
	st, ok := streamObj.(*raw.StreamObj)
	if !ok {
		return nil, errors.New("object stream is not a stream")
	}
	nObj := int(getIntFromDict(st.Dict, "N"))
	first := int(getIntFromDict(st.Dict, "First"))
	data := st.RawData()
	if first > len(data) {
		return nil, errors.New("object stream First exceeds length")
	}
	filterNames, filterParams := filtersForStream(st.Dict)
	if len(filterNames) > 0 {
		p := filters.NewPipeline([]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewRunLengthDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
			filters.NewCryptDecoder(),
		}, filters.Limits{
			MaxDecompressedSize: o.limits.MaxDecompressedSize,
			MaxDecodeTime:       o.limits.MaxDecodeTime,
		})
		decoded, err := p.Decode(ctx, data, filterNames, filterParams)
		if err != nil {
			return nil, err
		}
		data = decoded
	}
	header := data[:first]
	body := data[first:]
	// parse header pairs
	cfg := scanner.Config{
		Recovery:        o.recovery,
		MaxStringLength: o.limits.MaxStringLength,
		MaxArrayDepth:   o.limits.MaxIndirectDepth,
		MaxDictDepth:    o.limits.MaxIndirectDepth,
		MaxBufferSize:   o.limits.MaxDecompressedSize,
		MaxStreamLength: o.limits.MaxStreamLength,
	}
	s := scanner.New(bytes.NewReader(header), cfg)
	var pairs []int
	for len(pairs)/2 < nObj {
		tok, err := s.Next()
		if err != nil {
			return nil, err
		}
		if tok.Type != scanner.TokenNumber {
			continue
		}
		if !tok.IsInt {
			continue
		}
		pairs = append(pairs, int(tok.Int))
	}
	// Build objects map
	objs := make(map[int]raw.Object)
	bodyScanner := func(start int) scanner.Scanner {
		cfg := scanner.Config{
			Recovery:        o.recovery,
			MaxStringLength: o.limits.MaxStringLength,
			MaxArrayDepth:   o.limits.MaxIndirectDepth,
			MaxDictDepth:    o.limits.MaxIndirectDepth,
			MaxBufferSize:   o.limits.MaxDecompressedSize,
			MaxStreamLength: o.limits.MaxStreamLength,
		}
		sc := scanner.New(bytes.NewReader(body[start:]), cfg)
		return sc
	}
	for i := 0; i < nObj; i++ {
		objNum := pairs[2*i]
		off := pairs[2*i+1]
		sc := bodyScanner(off)
		tr := &tokenReader{s: sc}
		obj, err := parseObject(tr, o.recovery, objNum, 0)
		if err != nil {
			return nil, err
		}
		objs[objNum] = obj
	}
	o.objstm[objStreamNum] = objs
	if obj, ok := objs[ref.Num]; ok {
		return obj, nil
	}
	return nil, errors.New("object not found in object stream")
}

func getIntFromDict(d *raw.DictObj, key string) int64 {
	if v, ok := d.Get(raw.NameObj{Val: key}); ok {
		if n, ok := v.(raw.NumberObj); ok {
			return n.Int()
		}
	}
	return 0
}

func filtersForStream(d *raw.DictObj) ([]string, []raw.Dictionary) {
	fObj, ok := d.Get(raw.NameObj{Val: "Filter"})
	if !ok {
		return nil, nil
	}
	var names []string
	switch v := fObj.(type) {
	case raw.NameObj:
		names = []string{v.Val}
	case *raw.ArrayObj:
		for _, it := range v.Items {
			if n, ok := it.(raw.NameObj); ok {
				names = append(names, n.Val)
			}
		}
	}
	var params []raw.Dictionary
	if dp, ok := d.Get(raw.NameObj{Val: "DecodeParms"}); ok {
		switch p := dp.(type) {
		case *raw.DictObj:
			params = append(params, p)
		case *raw.ArrayObj:
			for _, it := range p.Items {
				if dd, ok := it.(*raw.DictObj); ok {
					params = append(params, dd)
				}
			}
		}
	}
	return names, params
}

func cryptFilterForStream(d *raw.DictObj) (string, bool) {
	if d == nil {
		return "", false
	}
	names, params := filtersForStream(d)
	if len(names) == 0 {
		return "", false
	}
	for idx, name := range names {
		if name != "Crypt" {
			continue
		}
		var dp raw.Dictionary
		if len(params) == len(names) {
			dp = params[idx]
		} else if len(params) == 1 {
			dp = params[0]
		} else if idx < len(params) {
			dp = params[idx]
		}
		if dp != nil {
			if nObj, ok := dp.Get(raw.NameObj{Val: "Name"}); ok {
				if n, ok := nObj.(raw.NameObj); ok {
					return n.Val, true
				}
			}
		}
		return "", true // default Crypt filter
	}
	return "", false
}

func (o *objectLoader) decryptObject(ref raw.ObjectRef, obj raw.Object) (raw.Object, error) {
	if o.security == nil || !o.security.IsEncrypted() {
		return obj, nil
	}
	switch v := obj.(type) {
	case raw.StringObj:
		dec, err := o.security.Decrypt(ref.Num, ref.Gen, v.Value(), security.DataClassString)
		if err != nil {
			return nil, err
		}
		return raw.StringObj{Bytes: dec}, nil
	case *raw.ArrayObj:
		for i, item := range v.Items {
			dec, err := o.decryptObject(ref, item)
			if err != nil {
				return nil, err
			}
			v.Items[i] = dec
		}
		return v, nil
	case *raw.DictObj:
		for key, item := range v.KV {
			dec, err := o.decryptObject(ref, item)
			if err != nil {
				return nil, err
			}
			v.KV[key] = dec
		}
		return v, nil
	case *raw.StreamObj:
		if v.Dict != nil {
			if _, err := o.decryptObject(ref, v.Dict); err != nil {
				return nil, err
			}
		}
		if o.shouldDecryptStream(v.Dict) {
			class := security.DataClassStream
			if isMetadataStream(v.Dict) {
				class = security.DataClassMetadataStream
			}
			cryptFilter, hasCrypt := cryptFilterForStream(v.Dict)
			if hasCrypt && cryptFilter == "Identity" {
				return v, nil
			}
			dec, err := o.decryptStreamWithFilter(ref.Num, ref.Gen, v.Data, class, cryptFilter)
			if err != nil {
				return nil, err
			}
			v.Data = dec
			if v.Dict != nil {
				v.Dict.Set(raw.NameLiteral("Length"), raw.NumberObj{I: int64(len(dec)), IsInt: true})
			}
		}
		return v, nil
	default:
		return obj, nil
	}
}

func (o *objectLoader) shouldDecryptStream(dict *raw.DictObj) bool {
	if o.security == nil || !o.security.IsEncrypted() {
		return false
	}
	if dict == nil {
		return true
	}
	if isMetadataStream(dict) {
		if encMeta, ok := o.security.(interface{ EncryptMetadata() bool }); ok {
			return encMeta.EncryptMetadata()
		}
	}
	return true
}

func (o *objectLoader) decryptStreamWithFilter(objNum, gen int, data []byte, class security.DataClass, cryptFilter string) ([]byte, error) {
	if h, ok := o.security.(interface {
		DecryptWithFilter(objNum, gen int, data []byte, class security.DataClass, cryptFilter string) ([]byte, error)
	}); ok {
		return h.DecryptWithFilter(objNum, gen, data, class, cryptFilter)
	}
	return o.security.Decrypt(objNum, gen, data, class)
}

func isMetadataStream(d *raw.DictObj) bool {
	if d == nil {
		return false
	}
	if v, ok := d.Get(raw.NameObj{Val: "Type"}); ok {
		if n, ok := v.(raw.NameObj); ok && n.Val == "Metadata" {
			return true
		}
	}
	return false
}

// Parsing helpers (duplicated from raw parser for loader-focused parsing).

type streamLengthSetter interface{ SetNextStreamLength(int64) }

type tokenReader struct {
	s            interface{ Next() (scanner.Token, error) }
	buf          []scanner.Token
	lengthSetter streamLengthSetter
}

func newTokenReader(src interface{ Next() (scanner.Token, error) }) *tokenReader {
	tr := &tokenReader{s: src}
	if setter, ok := src.(streamLengthSetter); ok {
		tr.lengthSetter = setter
	}
	return tr
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

func (r *tokenReader) setStreamLengthHint(n int64) {
	if r.lengthSetter != nil && n > 0 {
		r.lengthSetter.SetNextStreamLength(n)
	}
}

func (r *tokenReader) clearStreamLengthHint() {
	if r.lengthSetter != nil {
		r.lengthSetter.SetNextStreamLength(-1)
	}
}

func parseObject(tr *tokenReader, rec recovery.Strategy, objNum, gen int) (raw.Object, error) {
	tok, err := tr.next()
	if err != nil {
		return nil, err
	}
	if tok.Type == scanner.TokenKeyword && tok.Str == "endobj" {
		return nil, errors.New("unexpected endobj")
	}
	switch tok.Type {
	case scanner.TokenName:
		return raw.NameObj{Val: tok.Str}, nil
	case scanner.TokenNumber:
		if tok.IsInt {
			return raw.NumberObj{I: tok.Int, IsInt: true}, nil
		}
		return raw.NumberObj{F: tok.Float, IsInt: false}, nil
	case scanner.TokenBoolean:
		return raw.BoolObj{V: tok.Bool}, nil
	case scanner.TokenNull:
		return raw.NullObj{}, nil
	case scanner.TokenString:
		return raw.StringObj{Bytes: tok.Bytes}, nil
	case scanner.TokenArray:
		return parseArray(tr, rec, objNum, gen)
	case scanner.TokenDict:
		return parseDict(tr, rec, objNum, gen)
	case scanner.TokenRef:
		return raw.RefObj{R: raw.ObjectRef{Num: int(tok.Int), Gen: tok.Gen}}, nil
	}
	return nil, errors.New("unexpected token")
}

func parseArray(tr *tokenReader, rec recovery.Strategy, objNum, gen int) (raw.Object, error) {
	arr := &raw.ArrayObj{}
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Str == "]" {
			break
		}
		tr.unread(tok)
		item, err := parseObject(tr, rec, objNum, gen)
		if err != nil {
			return nil, err
		}
		arr.Append(item)
	}
	return arr, nil
}

func parseDict(tr *tokenReader, rec recovery.Strategy, objNum, gen int) (raw.Object, error) {
	d := raw.Dict()
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Str == ">>" {
			break
		}
		if tok.Type != scanner.TokenName {
			// Recovery logic for missing ">>"
			if tok.Type == scanner.TokenKeyword && tok.Str == "endobj" {
				err := errors.New("unexpected endobj in dict (missing >>?)")
				action := rec.OnError(nil, err, recovery.Location{ObjectNum: objNum, ObjectGen: gen, Component: "Parser"})
				if action == recovery.ActionWarn || action == recovery.ActionFix {
					tr.unread(tok)
					break
				}
				return nil, err
			}

			return nil, errors.New("expected name in dict")
		}
		key := tok.Str
		val, err := parseObject(tr, rec, objNum, gen)
		if err != nil {
			return nil, err
		}
		d.Set(raw.NameObj{Val: key}, val)
	}
	return d, nil
}

func (o *objectLoader) resolveStreamLength(dict *raw.DictObj) (int64, error) {
	if dict == nil {
		return 0, nil
	}
	val, ok := dict.Get(raw.NameLiteral("Length"))
	if !ok {
		return 0, nil
	}
	switch v := val.(type) {
	case raw.NumberObj:
		return v.Int(), nil
	case raw.RefObj:
		obj, err := o.loadReferencedObject(v.R)
		if err != nil {
			return 0, err
		}
		if num, ok := obj.(raw.NumberObj); ok {
			return num.Int(), nil
		}
		return 0, fmt.Errorf("length reference %v is not numeric", v.R)
	default:
		return 0, nil
	}
}

func (o *objectLoader) loadReferencedObject(ref raw.ObjectRef) (raw.Object, error) {
	offset, gen, ok := o.xrefTable.Lookup(ref.Num)
	if !ok {
		return nil, fmt.Errorf("object %d missing for length reference", ref.Num)
	}
	// Use a temporary scanner to avoid clobbering the shared scanner state
	cfg := scanner.Config{
		Recovery:        o.recovery,
		MaxStringLength: o.limits.MaxStringLength,
		MaxArrayDepth:   o.limits.MaxIndirectDepth,
		MaxDictDepth:    o.limits.MaxIndirectDepth,
		MaxBufferSize:   o.limits.MaxDecompressedSize,
		MaxStreamLength: o.limits.MaxStreamLength,
	}
	tmpScanner := scanner.New(o.reader, cfg)
	return o.scanObject(tmpScanner, ref.Num, offset, gen)
}
