package raw

import (
	"context"
	"fmt"
	"io"

	"pdflib/recovery"
	"pdflib/scanner"
)

// ParserConfig controls raw parsing behavior.
type ParserConfig struct {
	Scanner scanner.Config
}

// NewParser constructs a simple raw.Parser implementation.
func NewParser(cfg ParserConfig) Parser {
	return &parserImpl{cfg: cfg}
}

type parserImpl struct {
	cfg ParserConfig
}

func (p *parserImpl) Parse(ctx context.Context, r io.ReaderAt) (*Document, error) {
	s := scanner.New(r, p.cfg.Scanner)
	tr := &tokenReader{s: s}
	if rc, ok := s.(interface{ SetRecoveryLocation(recovery.Location) }); ok {
		rc.SetRecoveryLocation(recovery.Location{})
	}

	doc := &Document{
		Objects: make(map[ObjectRef]Object),
	}

	for {
		tok, err := tr.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if tok.Type != scanner.TokenNumber {
			continue
		}
		if !tok.IsInt {
			continue
		}
		objNum := int(tok.Int)

		genTok, err := tr.next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if genTok.Type != scanner.TokenNumber {
			tr.unread(genTok)
			continue
		}
		if !genTok.IsInt {
			continue
		}
		gen := int(genTok.Int)

		kwTok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if kwTok.Type != scanner.TokenKeyword || kwTok.Str != "obj" {
			tr.unread(kwTok)
			tr.unread(genTok)
			continue
		}

		// Provide object context to recovery-aware scanners.
		if rc, ok := s.(interface{ SetRecoveryLocation(recovery.Location) }); ok {
			rc.SetRecoveryLocation(recovery.Location{ObjectNum: objNum, ObjectGen: gen})
		}

		obj, err := parseObject(tr)
		if err != nil {
			return nil, fmt.Errorf("parse object %d %d: %w", objNum, gen, err)
		}

		// Streams: if the next token is a stream payload, wrap the dictionary.
		if dict, ok := obj.(*DictObj); ok {
			// Hint the scanner with stream length if available directly.
			if lenObj, ok := dict.Get(NameObj{Val: "Length"}); ok {
				if num, ok := lenObj.(Number); ok && num.IsInteger() {
					s.SetNextStreamLength(num.Int())
				}
			}

			if streamTok, err := tr.next(); err == nil {
				if streamTok.Type == scanner.TokenStream {
					obj = NewStream(dict, streamTok.Bytes)
				} else {
					// Not a stream; put it back for outer loop.
					tr.unread(streamTok)
				}
			}
		}

		// Consume optional endobj
		if t, err := tr.next(); err == nil {
			if t.Type != scanner.TokenKeyword || t.Str != "endobj" {
				tr.unread(t)
			}
		}

		doc.Objects[ObjectRef{Num: objNum, Gen: gen}] = obj
	}

	return doc, nil
}

func parseObject(tr *tokenReader) (Object, error) {
	tok, err := tr.next()
	if err != nil {
		return nil, err
	}
	switch tok.Type {
	case scanner.TokenName:
		return NameObj{Val: tok.Str}, nil
	case scanner.TokenNumber:
		if tok.IsInt {
			return NumberObj{I: tok.Int, IsInt: true}, nil
		}
		return NumberObj{F: tok.Float, IsInt: false}, nil
	case scanner.TokenBoolean:
		return BoolObj{V: tok.Bool}, nil
	case scanner.TokenNull:
		return NullObj{}, nil
	case scanner.TokenString:
		return StringObj{Bytes: tok.Bytes}, nil
	case scanner.TokenArray:
		return parseArray(tr)
	case scanner.TokenDict:
		return parseDict(tr)
	case scanner.TokenRef:
		return RefObj{R: ObjectRef{Num: int(tok.Int), Gen: tok.Gen}}, nil
	}
	return nil, fmt.Errorf("unexpected token: %v", tok.Type)
}

func parseArray(tr *tokenReader) (Object, error) {
	arr := &ArrayObj{}
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Str == "]" {
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

func parseDict(tr *tokenReader) (Object, error) {
	d := Dict()
	for {
		tok, err := tr.next()
		if err != nil {
			return nil, err
		}
		if tok.Type == scanner.TokenKeyword && tok.Str == ">>" {
			break
		}
		if tok.Type != scanner.TokenName {
			return nil, fmt.Errorf("expected name in dict, got %v", tok.Type)
		}
		key := tok.Str
		val, err := parseObject(tr)
		if err != nil {
			return nil, err
		}
		d.Set(NameObj{Val: key}, val)
	}
	return d, nil
}

type tokenReader struct {
	s   scanner.Scanner
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

func (r *tokenReader) unread(tok scanner.Token) {
	r.buf = append(r.buf, tok)
}
