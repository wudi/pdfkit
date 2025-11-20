package parser

import (
	"pdflib/scanner"
)

// streamAware wraps a scanner and sets stream length hints from preceding dictionaries.
type streamAware struct {
	s   scanner.Scanner
	buf []scanner.Token
}

func NewStreamAware(s scanner.Scanner) *streamAware { return &streamAware{s: s} }

// SetNextStreamLength forwards stream length hints to the underlying scanner.
func (w *streamAware) SetNextStreamLength(n int64) { w.s.SetNextStreamLength(n) }

func (w *streamAware) Next() (scanner.Token, error) {
	if len(w.buf) > 0 {
		t := w.buf[0]
		w.buf = w.buf[1:]
		return t, nil
	}
	tok, err := w.s.Next()
	if err != nil {
		return scanner.Token{}, err
	}
	if tok.Type == scanner.TokenDict {
		// Collect full dictionary and detect Length
		w.buf = append(w.buf, tok)
		var length int64 = -1
		for {
			kt, err := w.s.Next()
			if err != nil {
				return scanner.Token{}, err
			}
			w.buf = append(w.buf, kt)
			// End of dict
			if kt.Type == scanner.TokenKeyword {
				if kt.Str == ">>" {
					break
				}
			}
			// Expect name key
			if kt.Type != scanner.TokenName {
				continue
			}
			key := kt.Str
			vt, err := w.s.Next()
			if err != nil {
				return scanner.Token{}, err
			}
			w.buf = append(w.buf, vt)
			if key == "Length" {
				if vt.Type == scanner.TokenNumber && vt.IsInt {
					length = vt.Int
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
func (w *streamAware) Position() int64         { return w.s.Position() }
func (w *streamAware) SeekTo(offset int64) error { return w.s.SeekTo(offset) }
