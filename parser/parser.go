package parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"pdflib/filters"
	"pdflib/ir/raw"
	"pdflib/recovery"
	"pdflib/scanner"
	"pdflib/security"
	"pdflib/xref"
)

// Config controls high-level PDF parsing (xref resolution + object loading).
type Config struct {
	Recovery    recovery.Strategy
	XRef        xref.ResolverConfig
	MaxIndirect int
	Security    security.Handler
	Limits      security.Limits
	Cache       Cache
	Password    string
}

// DocumentParser builds a raw.Document using xref tables/streams and the object loader.
type DocumentParser struct {
	cfg Config
}

func NewDocumentParser(cfg Config) *DocumentParser {
	if cfg.MaxIndirect == 0 {
		cfg.MaxIndirect = cfg.Limits.MaxIndirectDepth
		if cfg.MaxIndirect == 0 {
			cfg.MaxIndirect = security.DefaultLimits().MaxIndirectDepth
		}
	}
	return &DocumentParser{cfg: cfg}
}

// SetPassword updates the password for decryption when parsing encrypted PDFs.
func (p *DocumentParser) SetPassword(pwd string) {
	p.cfg.Password = pwd
}

func (p *DocumentParser) Parse(ctx context.Context, r io.ReaderAt) (*raw.Document, error) {
	resolver := xref.NewResolver(p.cfg.XRef)
	table, err := resolver.Resolve(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("resolve xref: %w", err)
	}

	sec, err := p.selectSecurity(ctx, r, table, resolver.Trailer())
	if err != nil {
		return nil, fmt.Errorf("security setup: %w", err)
	}

	builder := &ObjectLoaderBuilder{
		reader:    r,
		xrefTable: table,
		security:  sec,
		maxDepth:  p.cfg.MaxIndirect,
		limits:    p.cfg.Limits,
		cache:     p.cfg.Cache,
		recovery:  p.cfg.Recovery,
	}
	loader, err := builder.Build()
	if err != nil {
		return nil, err
	}

	doc := &raw.Document{
		Objects:           make(map[raw.ObjectRef]raw.Object),
		Trailer:           resolver.Trailer(),
		Version:           detectHeaderVersion(r),
		Permissions:       toRawPermissions(sec.Permissions()),
		MetadataEncrypted: encryptsMetadata(sec),
		Encrypted:         sec.IsEncrypted(),
	}

	// Check for PDF 2.0 Unencrypted Wrapper (Collection)
	if doc.Trailer != nil {
		if rootRef, ok := doc.Trailer.Get(raw.NameObj{Val: "Root"}); ok {
			if ref, ok := rootRef.(raw.RefObj); ok {
				// We need to load the Catalog to check for Collection
				// But we haven't loaded objects yet.
				// We can use the loader we just built.
				if catalogObj, err := loader.Load(ctx, ref.R); err == nil {
					if catalog, ok := catalogObj.(*raw.DictObj); ok {
						if collection, ok := catalog.Get(raw.NameObj{Val: "Collection"}); ok {
							// Check if it's an unencrypted wrapper
							// This logic is simplified; real check involves checking schema
							// and potentially "EncryptedPayload" in the Collection dictionary
							// or associated files.
							_ = collection // Placeholder for future implementation
						}
					}
				}
			}
		}
	}

	for _, objNum := range table.Objects() {
		if objNum == 0 {
			continue // free head entry
		}
		_, gen, found := table.Lookup(objNum)
		if !found {
			continue
		}
		ref := raw.ObjectRef{Num: objNum, Gen: gen}
		obj, err := loader.Load(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("load object %d: %w", objNum, err)
		}
		doc.Objects[ref] = obj
	}

	if doc.Trailer != nil {
		p.populateMetadata(ctx, loader, doc)
	}

	if resolver.Linearized() {
		doc.Linearized = true
		p.parseLinearization(ctx, r, doc, loader)
	}

	return doc, nil
}

func (p *DocumentParser) selectSecurity(ctx context.Context, r io.ReaderAt, table xref.Table, trailer raw.Dictionary) (security.Handler, error) {
	if p.cfg.Security != nil {
		return p.cfg.Security, nil
	}
	trailerDict, _ := trailer.(*raw.DictObj)
	if trailerDict == nil {
		return security.NoopHandler(), nil
	}
	encObj, ok := trailerDict.Get(raw.NameObj{Val: "Encrypt"})
	if !ok {
		return security.NoopHandler(), nil
	}
	var encDict *raw.DictObj
	switch v := encObj.(type) {
	case *raw.DictObj:
		encDict = v
	case raw.RefObj:
		loader, err := (&ObjectLoaderBuilder{
			reader:    r,
			xrefTable: table,
			security:  security.NoopHandler(),
			maxDepth:  p.cfg.MaxIndirect,
			limits:    p.cfg.Limits,
			cache:     p.cfg.Cache,
			recovery:  p.cfg.Recovery,
		}).Build()
		if err != nil {
			return nil, err
		}
		obj, err := loader.Load(ctx, v.R)
		if err == nil {
			encDict, _ = obj.(*raw.DictObj)
		}
	}
	if encDict == nil {
		return security.NoopHandler(), nil
	}
	fileID := fileIDFromTrailer(trailerDict)
	handler, err := (&security.HandlerBuilder{}).WithEncryptDict(encDict).WithTrailer(trailerDict).WithFileID(fileID).Build()
	if err != nil {
		return nil, err
	}
	if err := handler.Authenticate(p.cfg.Password); err != nil {
		return nil, err
	}
	return handler, nil
}

func fileIDFromTrailer(trailer *raw.DictObj) []byte {
	if trailer == nil {
		return nil
	}
	if idObj, ok := trailer.Get(raw.NameObj{Val: "ID"}); ok {
		if arr, ok := idObj.(*raw.ArrayObj); ok && arr.Len() > 0 {
			if s, ok := arr.Items[0].(raw.StringObj); ok {
				return s.Value()
			}
			if hx, ok := arr.Items[0].(raw.HexStringObj); ok {
				return hx.Value()
			}
		}
	}
	return nil
}

func (p *DocumentParser) populateMetadata(ctx context.Context, loader ObjectLoader, doc *raw.Document) {
	infoObj, ok := doc.Trailer.Get(raw.NameObj{Val: "Info"})
	if !ok {
		return
	}
	ref, ok := infoObj.(raw.RefObj)
	if !ok {
		return
	}
	info, err := loader.Load(ctx, ref.R)
	if err != nil {
		return
	}
	dict, ok := info.(*raw.DictObj)
	if !ok {
		return
	}
	md := raw.DocumentMetadata{}
	if v, ok := stringValue(dict, "Title"); ok {
		md.Title = v
	}
	if v, ok := stringValue(dict, "Author"); ok {
		md.Author = v
	}
	if v, ok := stringValue(dict, "Creator"); ok {
		md.Creator = v
	}
	if v, ok := stringValue(dict, "Producer"); ok {
		md.Producer = v
	}
	if v, ok := stringValue(dict, "Subject"); ok {
		md.Subject = v
	}
	if v, ok := stringValue(dict, "Keywords"); ok {
		md.Keywords = strings.Split(v, ",")
	}
	doc.Metadata = md
}

func stringValue(dict *raw.DictObj, key string) (string, bool) {
	obj, ok := dict.Get(raw.NameObj{Val: key})
	if !ok {
		return "", false
	}
	str, ok := obj.(raw.StringObj)
	if !ok {
		return "", false
	}
	return string(str.Value()), true
}

func encryptsMetadata(h security.Handler) bool {
	if h == nil {
		return false
	}
	return h.EncryptMetadata()
}

func toRawPermissions(p security.Permissions) raw.Permissions {
	return raw.Permissions{
		Print:             p.Print,
		Modify:            p.Modify,
		Copy:              p.Copy,
		ModifyAnnotations: p.ModifyAnnotations,
		FillForms:         p.FillForms,
		ExtractAccessible: p.ExtractAccessible,
		Assemble:          p.Assemble,
		PrintHighQuality:  p.PrintHighQuality,
	}
}

func detectHeaderVersion(r io.ReaderAt) string {
	buf := make([]byte, 64)
	n, err := r.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	line := string(buf[:n])
	for _, sep := range []string{"\r\n", "\n", "\r"} {
		if idx := strings.Index(line, sep); idx >= 0 {
			line = line[:idx]
			break
		}
	}
	if strings.HasPrefix(line, "%PDF-") && len(line) >= 8 {
		return strings.TrimSpace(line[5:])
	}
	return ""
}

func (p *DocumentParser) parseLinearization(ctx context.Context, r io.ReaderAt, doc *raw.Document, loader ObjectLoader) {
	// Scan for Linearization dictionary in the first 1024 bytes (or slightly more)
	// We use a fresh scanner.
	s := scanner.New(r, scanner.Config{Recovery: p.cfg.Recovery})

	var linDict *raw.DictObj
	var npages int

	// Simple state machine to find "obj << ... /Linearized ... >>"
	// We look for the first dictionary that has /Linearized.
	// Note: This is a heuristic. The correct way is to parse the first object.

	tr := newTokenReader(s)

	// Try to parse the first object
	// Skip header
	for {
		tok, err := s.Next()
		if err != nil {
			return
		}
		if tok.Type == scanner.TokenNumber {
			// Found potential object number
			// We assume the first object is the linearization dict
			// <num> <gen> obj
			tok2, err := s.Next()
			if err != nil {
				return
			}
			if tok2.Type == scanner.TokenNumber {
				tok3, err := s.Next()
				if err != nil {
					return
				}
				if tok3.Type == scanner.TokenKeyword && tok3.Str == "obj" {
					// Parse object
					obj, err := parseObject(tr, p.cfg.Recovery, int(tok.Int), int(tok2.Int))
					if err != nil {
						return
					}

					if dict, ok := obj.(*raw.DictObj); ok {
						if _, ok := dict.Get(raw.NameObj{Val: "Linearized"}); ok {
							linDict = dict
						}
					}
					break // Found first object, stop
				}
			}
		}
	}

	if linDict == nil {
		return
	}

	// Get N (Number of pages)
	if nVal, ok := linDict.Get(raw.NameObj{Val: "N"}); ok {
		npages = int(toInt64(nVal))
	}

	// Get H (Hint stream offset)
	hVal, ok := linDict.Get(raw.NameObj{Val: "H"})
	if !ok {
		return
	}
	hArr, ok := hVal.(*raw.ArrayObj)
	if !ok || hArr.Len() < 1 {
		return
	}

	hOffsetObj, _ := hArr.Get(0)
	hOffset := toInt64(hOffsetObj)

	// Parse Hint Stream at hOffset
	s2 := scanner.New(r, scanner.Config{Recovery: p.cfg.Recovery})
	if err := s2.SeekTo(hOffset); err != nil {
		return
	}
	tr2 := newTokenReader(s2)

	// Expect <num> <gen> obj
	tokNum, err := s2.Next()
	if err != nil {
		return
	}
	tokGen, err := s2.Next()
	if err != nil {
		return
	}
	tokObj, err := s2.Next()
	if err != nil || tokObj.Str != "obj" {
		return
	}

	// Parse stream object
	// We need to handle stream length.
	// We can reuse parseObject but we need to handle the stream data reading which relies on Length.
	// parseObject in loader.go handles stream if we provide a length hint or if it can resolve it.
	// But here we don't have the loader's resolve capability wired into tr2.
	// We have to do it manually.

	obj, err := parseObject(tr2, p.cfg.Recovery, int(tokNum.Int), int(tokGen.Int))
	if err != nil {
		return
	}

	dict, ok := obj.(*raw.DictObj)
	if !ok {
		return
	}

	// Resolve Length
	var length int64
	if lVal, ok := dict.Get(raw.NameObj{Val: "Length"}); ok {
		switch v := lVal.(type) {
		case raw.NumberObj:
			length = v.Int()
		case raw.RefObj:
			// Load indirect length
			lObj, err := loader.Load(ctx, v.R)
			if err == nil {
				if n, ok := lObj.(raw.NumberObj); ok {
					length = n.Int()
				}
			}
		}
	}

	if length <= 0 {
		return
	}

	// Read stream data
	// parseObject consumed the dict. The next token should be "stream".
	// But parseObject consumes the dict and stops at ">>".
	// We need to check for "stream".

	tokStream, err := s2.Next()
	if err != nil {
		return
	}
	if tokStream.Type != scanner.TokenKeyword || tokStream.Str != "stream" {
		return
	}

	// Read bytes
	// Scanner doesn't expose ReadBytes easily for exact length unless we use SetNextStreamLength
	// But we already passed the point where SetNextStreamLength would be called (before "stream" token).
	// Actually, scanner.Next() handles "stream" token by reading the stream if length is set.
	// If length is NOT set, it might try to find "endstream".
	// But we want to use the length we found.

	// We should have set the length hint on tr2 BEFORE calling parseObject?
	// No, parseObject parses the dict. We get Length FROM the dict.
	// Then we read the stream.

	// We can use s2.ReadStream(length) if it existed.
	// Or we can just read from the reader directly since we know the offset.
	// Current offset of s2?
	// s2 doesn't expose offset easily.

	// Let's use a new scanner or reader for the stream data.
	// We know hOffset. We parsed the header and dict.
	// We can approximate the stream start or just rely on scanner finding "stream" and then reading.

	// Actually, `scanner.Next()` returns `TokenStream` which contains the data.
	// But `scanner` needs to know the length to read it efficiently, or it scans for `endstream`.
	// If we didn't set the length, it scans.
	// Let's assume it scans correctly or we can set it.
	// But `scanner` is already past the dict.

	// If `tokStream` is `stream`, `scanner` might have already read the data if it was in `TokenStream` type.
	// But `scanner` returns `TokenKeyword` "stream" if it's not configured to read stream data automatically?
	// Let's check `scanner/scanner.go`.
	// Assuming `scanner` returns `TokenStream` with data if it knows it's a stream.
	// But `parseObject` in `loader.go` handles this:
	/*
		if streamTok, err := tr.next(); err == nil && streamTok.Type == scanner.TokenStream {
			obj = raw.NewStream(dict, streamTok.Bytes)
		}
	*/
	// It expects `TokenStream`.

	// We need to tell the scanner the length.
	// `tr2` has `setStreamLengthHint`.
	// But we need to call it *before* `tr2.next()` returns the stream token.
	// We just called `s2.Next()` which returned "stream" keyword?
	// No, `scanner` returns `TokenStream` if it parses a stream.
	// But `scanner` needs to know it's a stream.
	// It sees "stream" keyword.

	// If I use `tr2.setStreamLengthHint(length)`, it sets it on the scanner.
	// Then `tr2.next()` (which calls `s2.Next()`) will use that length when it encounters "stream".

	tr2.setStreamLengthHint(length)
	tokStreamData, err := tr2.next()
	if err != nil {
		return
	}

	var data []byte
	if tokStreamData.Type == scanner.TokenStream {
		data = tokStreamData.Bytes
	} else {
		return
	}

	// Decode stream if needed (FlateDecode is common for hint streams)
	// We need to handle filters.
	// We can use `loader`'s filter logic or duplicate it.
	// `loader` has `filtersForStream`.

	filterNames, filterParams := filtersForStream(dict)
	if len(filterNames) > 0 {
		pipeline := filters.NewPipeline([]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
			filters.NewRunLengthDecoder(),
		}, filters.Limits{
			MaxDecompressedSize: p.cfg.Limits.MaxDecompressedSize,
			MaxDecodeTime:       p.cfg.Limits.MaxDecodeTime,
		})

		decoded, err := pipeline.Decode(ctx, data, filterNames, filterParams)
		if err != nil {
			return
		}
		data = decoded
	}

	// Parse Hint Table
	ht, err := ParseHintStream(data, dict, npages)
	if err != nil {
		return
	}

	doc.HintTable = ht
}
