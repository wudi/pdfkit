package parser

import (
	"pdflib/scanner"
)

// StreamAware wraps a scanner and sets stream length hints from preceding dictionaries.
type StreamAware struct {
	s   scanner.Scanner
	buf []scanner.Token
}

func NewStreamAware(s scanner.Scanner) *StreamAware { return &StreamAware{s: s} }

func (w *StreamAware) Next() (scanner.Token, error) {
	if len(w.buf) > 0 {
		t := w.buf[0]
		w.buf = w.buf[1:]
		return t, nil
	}
	tok, err := w.s.Next()
	if err != nil { return scanner.Token{}, err }
	if tok.Type == scanner.TokenDict {
		// Collect full dictionary and detect Length
		w.buf = append(w.buf, tok)
		var length int64 = -1
		for {
			kt, err := w.s.Next()
			if err != nil { return scanner.Token{}, err }
			w.buf = append(w.buf, kt)
			// End of dict
			if kt.Type == scanner.TokenKeyword {
				if s, ok := kt.Value.(string); ok && s == ">>" { break }
			}
			// Expect name key
			if kt.Type != scanner.TokenName { continue }
			key, _ := kt.Value.(string)
			vt, err := w.s.Next()
			if err != nil { return scanner.Token{}, err }
			w.buf = append(w.buf, vt)
			if key == "Length" {
				switch v := vt.Value.(type) {
				case int64:
					length = v
				}
			}
		}
		if length >= 0 {
			w.s.SetNextStreamLength(length)
		}
		// Return first buffered token
		t := w.buf[0]
		w.buf = w.buf[1:]
		return t, nil
	}
	return tok, nil
}

// Position proxies underlying scanner position.
func (w *StreamAware) Position() int64 { return w.s.Position() }
func (w *StreamAware) Seek(offset int64) error { return w.s.Seek(offset) }
