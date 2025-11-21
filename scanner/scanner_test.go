package scanner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/recovery"
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
	if tok.Type != TokenNumber || !tok.IsInt || tok.Int != 1 {
		t.Fatalf("expected first token number 1, got %+v", tok)
	}
	tok = nextToken(t, s)
	if tok.Type != TokenNumber || !tok.IsInt || tok.Int != 0 {
		t.Fatalf("expected generation number 0, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenKeyword || tok.Str != "obj" {
		t.Fatalf("expected obj keyword, got %+v", tok)
	}
	// Dictionary start
	if tok = nextToken(t, s); tok.Type != TokenDict {
		t.Fatalf("expected dict start, got %+v", tok)
	}
	// First key/value pair
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Str != "Name" {
		t.Fatalf("expected Name key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Str != "Value" {
		t.Fatalf("expected Name value, got %+v", tok)
	}
	// Array contents 1 2 3
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Str != "Nums" {
		t.Fatalf("expected Nums key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenArray {
		t.Fatalf("expected array start, got %+v", tok)
	}
	for i := int64(1); i <= 3; i++ {
		tok = nextToken(t, s)
		if tok.Type != TokenNumber || !tok.IsInt || tok.Int != i {
			t.Fatalf("expected array number %d, got %+v", i, tok)
		}
	}
	if tok = nextToken(t, s); tok.Type != TokenKeyword || tok.Str != "]" {
		t.Fatalf("expected array close, got %+v", tok)
	}
	// Boolean and null
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Str != "Flag" {
		t.Fatalf("expected Flag key, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenBoolean || tok.Bool != true {
		t.Fatalf("expected true boolean, got %+v", tok)
	}
	if tok = nextToken(t, s); tok.Type != TokenName || tok.Str != "Null" {
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
	if tok.Str != "Name With#Hash" {
		t.Fatalf("unexpected name decode: %v", tok.Str)
	}
}

func TestScanner_LiteralStringEscapes(t *testing.T) {
	s := newScanner(t, "(Hi\\n\\050\\051\\t)", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenString {
		t.Fatalf("expected string, got %+v", tok)
	}
	if !bytes.Equal(tok.Bytes, []byte("Hi\n()\t")) {
		t.Fatalf("unexpected literal string: %q", tok.Bytes)
	}
}

func TestScanner_LiteralStringLineContinuation(t *testing.T) {
	s := newScanner(t, "(Line\\\r\ncontinued)", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenString {
		t.Fatalf("expected string, got %+v", tok)
	}
	if got := string(tok.Bytes); got != "Linecontinued" {
		t.Fatalf("unexpected literal string with continuation: %q", got)
	}
}

func TestScanner_HexStringOddLength(t *testing.T) {
	s := newScanner(t, "<48656c6c6f3>", Config{})
	tok := nextToken(t, s)
	want := []byte("Hello0")
	if tok.Type != TokenString || !bytes.Equal(tok.Bytes, want) {
		t.Fatalf("expected padded hex string %q, got %+v", want, tok)
	}
}

func TestScanner_ReferenceDetection(t *testing.T) {
	s := newScanner(t, "12 5 R %comment\n", Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenRef {
		t.Fatalf("expected ref, got %+v", tok)
	}
	if tok.Int != 12 || tok.Gen != 5 {
		t.Fatalf("unexpected ref value: %+v", tok)
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
	if string(tok.Bytes) != "abcde" {
		t.Fatalf("unexpected stream payload: %q", tok.Bytes)
	}
}

func TestScanner_StreamFallbackToEndstream(t *testing.T) {
	data := "stream\nabc\r\nendstream\n"
	s := newScanner(t, data, Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenStream {
		t.Fatalf("expected stream token, got %+v", tok)
	}
	if got := string(tok.Bytes); got != "abc" {
		t.Fatalf("unexpected stream payload: %q", got)
	}
}

func TestScanner_NameTooLong(t *testing.T) {
	s := newScanner(t, "/abcdefgh", Config{MaxNameLength: 5})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "name too long") {
		t.Fatalf("expected name too long error, got %v", err)
	}
}

func TestScanner_MaxStringLength(t *testing.T) {
	s := newScanner(t, "<000102>", Config{MaxStringLength: 2})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "hex string too long") {
		t.Fatalf("expected hex string too long error, got %v", err)
	}
}

func TestScanner_StreamCRPrecedingEndstream(t *testing.T) {
	data := "stream\rdata\rendstream\r"
	s := newScanner(t, data, Config{})
	tok := nextToken(t, s)
	if tok.Type != TokenStream {
		t.Fatalf("expected stream token, got %+v", tok)
	}
	if got := string(tok.Bytes); got != "data" {
		t.Fatalf("unexpected stream payload: %q", got)
	}
}

func TestScanner_StreamScanLimit(t *testing.T) {
	data := "stream\nabc"
	s := newScanner(t, data, Config{MaxStreamScan: 2})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "endstream not found") {
		t.Fatalf("expected scan limit error, got %v", err)
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
	data := "ID \nabc\nEI\nBT"
	s := newScanner(t, data, Config{MaxInlineImage: 10})
	tok := nextToken(t, s)
	if tok.Type != TokenInlineImage {
		t.Fatalf("expected inline image token, got %+v", tok)
	}
	if got := string(tok.Bytes); got != "abc\n" {
		t.Fatalf("unexpected inline image payload: %q", got)
	}
	// Next token should be BT keyword
	tok = nextToken(t, s)
	if tok.Type != TokenKeyword || tok.Str != "BT" {
		t.Fatalf("expected BT after inline image, got %+v", tok)
	}
}

func TestScanner_InlineImageTooLong(t *testing.T) {
	data := "ID \nabcdefghijk\nEI"
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

func TestScanner_UnterminatedLiteralString(t *testing.T) {
	s := newScanner(t, "(abc", Config{})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "unterminated literal string") {
		t.Fatalf("expected unterminated literal string error, got %v", err)
	}
}

func TestScanner_UnterminatedHexString(t *testing.T) {
	s := newScanner(t, "<abc", Config{})
	if _, err := s.Next(); err == nil || !strings.Contains(err.Error(), "unterminated hex string") {
		t.Fatalf("expected unterminated hex string error, got %v", err)
	}
}

func TestScanner_DepthLimits(t *testing.T) {
	s := newScanner(t, "<< /A << /B << >> >> >>", Config{MaxDictDepth: 2})
	var err error
	for err == nil {
		_, err = s.Next()
	}
	if !strings.Contains(err.Error(), "dict depth exceeded") {
		t.Fatalf("expected dict depth exceeded, got %v", err)
	}
}

type fixRecovery struct{}

func (f *fixRecovery) OnError(ctx recovery.Context, err error, loc recovery.Location) recovery.Action {
	return recovery.ActionFix
}

func TestScanner_FixUnterminatedLiteralString(t *testing.T) {
	s := New(bytes.NewReader([]byte("(abc")), Config{Recovery: &fixRecovery{}})
	tok, err := s.Next()
	if err != nil {
		t.Fatalf("expected recovery to continue, got %v", err)
	}
	if tok.Type != TokenString || string(tok.Bytes) != "abc" {
		t.Fatalf("unexpected token after recovery: %+v", tok)
	}
}

func TestScanner_FixUnterminatedHexString(t *testing.T) {
	s := New(bytes.NewReader([]byte("<4142")), Config{Recovery: &fixRecovery{}})
	tok, err := s.Next()
	if err != nil {
		t.Fatalf("expected recovery to continue, got %v", err)
	}
	if tok.Type != TokenString || string(tok.Bytes) != "AB" {
		t.Fatalf("unexpected token after recovery: %+v", tok)
	}
}

func TestScanner_FixTruncatedStreamLength(t *testing.T) {
	s := New(bytes.NewReader([]byte("stream\nabc")), Config{Recovery: &fixRecovery{}})
	s.SetNextStreamLength(5)
	tok, err := s.Next()
	if err != nil {
		t.Fatalf("expected recovery to continue, got %v", err)
	}
	if tok.Type != TokenStream || string(tok.Bytes) != "abc" {
		t.Fatalf("unexpected stream payload after recovery: %+v", tok)
	}
}

func TestScanner_FixArrayUnderflow(t *testing.T) {
	s := New(bytes.NewReader([]byte("] 1")), Config{Recovery: &fixRecovery{}})
	tok := nextToken(t, s)
	if tok.Type != TokenNumber || !tok.IsInt || tok.Int != 1 {
		t.Fatalf("expected to skip underflowing ], got %+v", tok)
	}
}

func TestScanner_FixStreamScanLimit(t *testing.T) {
	s := New(bytes.NewReader([]byte("stream\nabc")), Config{MaxStreamScan: 1, Recovery: &fixRecovery{}})
	tok, err := s.Next()
	if err != nil {
		t.Fatalf("expected recovery to allow stream token, got %v", err)
	}
	if tok.Type != TokenStream || string(tok.Bytes) != "abc" {
		t.Fatalf("unexpected stream payload after recovery: %+v", tok)
	}
}

func TestScanner_FixUnclosedArrayAtEOF(t *testing.T) {
	s := New(bytes.NewReader([]byte("[1 2 ")), Config{Recovery: &fixRecovery{}})
	tok := nextToken(t, s)
	if tok.Type != TokenArray {
		t.Fatalf("expected array start, got %+v", tok)
	}
	// read 1, 2, then expect auto-closed array via recovery fix
	tok = nextToken(t, s)
	if tok.Type != TokenNumber || !tok.IsInt || tok.Int != 1 {
		t.Fatalf("expected 1, got %+v", tok)
	}
	tok = nextToken(t, s)
	if tok.Type != TokenNumber || !tok.IsInt || tok.Int != 2 {
		t.Fatalf("expected 2, got %+v", tok)
	}
	tok = nextToken(t, s)
	if tok.Type != TokenKeyword || tok.Str != "]" {
		t.Fatalf("expected auto-closed ], got %+v", tok)
	}
	if _, err := s.Next(); err == nil {
		t.Fatalf("expected EOF after auto-close")
	}
}

type recordRecovery struct {
	loc recovery.Location
	err error
}

func (r *recordRecovery) OnError(ctx recovery.Context, err error, loc recovery.Location) recovery.Action {
	r.loc = loc
	r.err = err
	return recovery.ActionWarn
}

func TestScanner_RecoveryContextIncludesObject(t *testing.T) {
	rec := &recordRecovery{}
	s := New(bytes.NewReader([]byte("<abc")), Config{Recovery: rec})
	if rc, ok := s.(interface{ SetRecoveryLocation(recovery.Location) }); ok {
		rc.SetRecoveryLocation(recovery.Location{ObjectNum: 5, ObjectGen: 2, Component: "parser"})
	}
	if _, err := s.Next(); err == nil {
		t.Fatalf("expected unterminated hex string error")
	}
	if rec.loc.ObjectNum != 5 || rec.loc.ObjectGen != 2 {
		t.Fatalf("expected object context 5 2, got %+v", rec.loc)
	}
	if !strings.Contains(rec.loc.Component, "scanner:hex") {
		t.Fatalf("expected component to include scanner:hex, got %q", rec.loc.Component)
	}
}
