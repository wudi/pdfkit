package scanner

import (
	"bytes"
	"strings"
	"testing"
)

func newScanner(t *testing.T, data string, cfg Config) Scanner {
	t.Helper()
	return New(bytes.NewReader([]byte(data)), cfg)
}

func nextToken(t *testing.T, s Scanner) Token {
	t.Helper()
	tok, err := s.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return tok
}

func TestScanner_BasicTokens(t *testing.T) {
	s := newScanner(t, "%PDF-1.7\n1 0 obj\n<< /Name /Value /Nums [1 2 3] /Flag true /Null null >>\nendobj", Config{})

	tok := nextToken(t, s)
	if tok.Type != TokenNumber || tok.Value != int64(1) {
		t.Fatalf("expected first token number 1, got %+v", tok)
	}
	tok = nextToken(t, s)
	if tok.Type != TokenNumber || tok.Value != int64(0) {
		t.Fatalf("expected generation number 0, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenKeyword || tok.Value != "obj" {
		t.Fatalf("expected obj keyword, got %+v", tok)
	}
	// Dictionary start
	if tok = nextToken(t, s); tok.Type != TokenDict {
		t.Fatalf("expected dict start, got %+v", tok)
	}
	// First key/value pair
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Value != "Name" {
		t.Fatalf("expected Name key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Value != "Value" {
		t.Fatalf("expected Name value, got %+v", tok)
	}
	// Array contents 1 2 3
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Value != "Nums" {
		t.Fatalf("expected Nums key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenArray {
		t.Fatalf("expected array start, got %+v", tok)
	}
	for i := int64(1); i <= 3; i++ {
		tok = nextToken(t, s)
		if tok.Type != TokenNumber || tok.Value != i {
			t.Fatalf("expected array number %d, got %+v", i, tok)
		}
	}
	if tok = nextToken(t, s); tok.Type != TokenKeyword || tok.Value != "]" {
		t.Fatalf("expected array close, got %+v", tok)
	}
	// Boolean and null
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Value != "Flag" {
		t.Fatalf("expected Flag key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenBoolean || tok.Value != true {
		t.Fatalf("expected true boolean, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Value != "Null" {
		t.Fatalf("expected Null key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenNull {
		t.Fatalf("expected null value, got %+v", tok)
	}
}

func TestScanner_NameHexEscapes(t *testing.T) {
	s := newScanner(t, "/Name#20With#23Hash", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenName {
		t.Fatalf("expected name, got %+v", tok)
	}
	if tok.Value != "Name With#Hash" {
		t.Fatalf("unexpected name decode: %v", tok.Value)
	}
}

func TestScanner_LiteralStringEscapes(t *testing.T) {
	s := newScanner(t, "(Hi\\n\\050\\051\\t)", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenString {
		t.Fatalf("expected string, got %+v", tok)
	}
	if !bytes.Equal(tok.Value.([]byte), []byte("Hi\n()\t")) {
		t.Fatalf("unexpected literal string: %q", tok.Value)
	}
}

func TestScanner_LiteralStringLineContinuation(t *testing.T) {
	s := newScanner(t, "(Line\\\r\ncontinued)", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenString {
		t.Fatalf("expected string, got %+v", tok)
	}
	if got := string(tok.Value.([]byte)); got != "Linecontinued" {
		t.Fatalf("unexpected literal string with continuation: %q", got)
	}
}

func TestScanner_HexStringOddLength(t *testing.T) {
	s := newScanner(t, "<48656c6c6f3>", Config{})
	tok := nextToken(t, s)
	want := []byte("Hello0")
	if tok.Type != TokenString || !bytes.Equal(tok.Value.([]byte), want) {
		t.Fatalf("expected padded hex string %q, got %+v", want, tok)
	}
}

func TestScanner_ReferenceDetection(t *testing.T) {
	s := newScanner(t, "12 5 R %comment\n", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenRef {
		t.Fatalf("expected ref, got %+v", tok)
	}
	val, ok := tok.Value.(struct{ Num, Gen int })
	if !ok || val.Num != 12 || val.Gen != 5 {
		t.Fatalf("unexpected ref value: %+v", tok.Value)
	}
}

func TestScanner_StreamWithLength(t *testing.T) {
	data := "stream\r\nabcde\r\nendstream"
	s := newScanner(t, data, Config{})
	s.SetNextStreamLength(5)
	tok := nextToken(t, s)
	if tok.Type != TokenStream {
		t.Fatalf("expected stream token, got %+v", tok)
	}
	if string(tok.Value.([]byte)) != "abcde" {
		t.Fatalf("unexpected stream payload: %q", tok.Value)
	}
}

func TestScanner_StreamFallbackToEndstream(t *testing.T) {
	data := "stream\nabc\r\nendstream\n"
	s := newScanner(t, data, Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenStream {
		t.Fatalf("expected stream token, got %+v", tok)
	}
	if got := string(tok.Value.([]byte)); got != "abc" {
		t.Fatalf("unexpected stream payload: %q", got)
	}
}

func TestScanner_MaxStringLength(t *testing.T) {
	s := newScanner(t, "<000102>", Config{MaxStringLength: 2})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "hex string too long") {
		t.Fatalf("expected hex string too long error, got %v", err)
	}
}

func TestScanner_MaxLiteralStringLength(t *testing.T) {
	s := newScanner(t, "(abcdef)", Config{MaxStringLength: 3})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "literal string too long") {
		t.Fatalf("expected literal string too long error, got %v", err)
	}
}

func TestScanner_MaxStreamLength(t *testing.T) {
	s := newScanner(t, "stream\nabcdef\nendstream", Config{MaxStreamLength: 3})
	s.SetNextStreamLength(6)
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "stream too long") {
		t.Fatalf("expected stream too long error, got %v", err)
	}
}

func TestScanner_InlineImage(t *testing.T) {
	data := "ID abc EI BT"
	s := newScanner(t, data, Config{MaxInlineImage: 10})
	tok := nextToken(t, s)
	if tok.Type != TokenInlineImage {
		t.Fatalf("expected inline image token, got %+v", tok)
	}
	if got := string(tok.Value.([]byte)); got != "abc " {
		t.Fatalf("unexpected inline image payload: %q", got)
	}
	// Next token should be BT keyword
	tok = nextToken(t, s)
	if tok.Type != TokenKeyword || tok.Value != "BT" {
		t.Fatalf("expected BT after inline image, got %+v", tok)
	}
}

func TestScanner_InlineImageTooLong(t *testing.T) {
	data := "ID abcdefghijk EI"
	s := newScanner(t, data, Config{MaxInlineImage: 5})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "inline image too long") {
		t.Fatalf("expected inline image too long error, got %v", err)
	}
}

func TestScanner_StreamMissingEOL(t *testing.T) {
	// stream without required EOL before data
	data := "stream abc\nendstream"
	s := newScanner(t, data, Config{})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "missing EOL") {
		t.Fatalf("expected missing EOL error, got %v", err)
	}
}
