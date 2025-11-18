package scanner

import (
	"bytes"
	"errors"
	"io"
	"pdflib/recovery"
	"strconv"
	"unicode"
)

type TokenType int

const (
	TokenDict        TokenType = iota // '<<'
	TokenArray                        // '['
	TokenName                         // '/Name'
	TokenString                       // literal or hex string
	TokenNumber                       // numeric value
	TokenBoolean                      // true/false
	TokenNull                         // null
	TokenRef                          // indirect ref '5 0 R'
	TokenStream                       // 'stream' keyword
	TokenInlineImage                  // inline image data following ID ... EI (content stream only)
	TokenKeyword                      // other keywords (obj, endobj, endstream, >>, ], etc.)
)

type Token struct {
	Type  TokenType
	Value interface{}
	Pos   int64
}

type Scanner interface {
	Next() (Token, error)
	Position() int64
	Seek(offset int64) error
	SetNextStreamLength(n int64)
}

type Config struct {
	MaxStringLength int64
	MaxArrayDepth   int
	MaxDictDepth    int
	MaxStreamLength int64
	MaxStreamScan   int64
	MaxInlineImage  int64
	WindowSize      int64
	Recovery        recovery.Strategy
}

type ReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
}

// pdfScanner incrementally buffers PDF data from a ReaderAt in fixed-size windows.
type pdfScanner struct {
	reader        ReaderAt
	data          []byte
	pos           int64
	cfg           Config
	nextStreamLen int64
	chunkSize     int64
	eof           bool
	arrayDepth    int
	dictDepth     int
	recLoc        recovery.Location
	lastAction    recovery.Action
}

// New loads entire ReaderAt into memory and returns a scanner.
func New(r ReaderAt, cfg Config) Scanner {
	chunk := cfg.WindowSize
	if chunk <= 0 {
		chunk = 64 * 1024
	}
	return &pdfScanner{reader: r, cfg: cfg, nextStreamLen: -1, chunkSize: chunk}
}

func (s *pdfScanner) Position() int64 { return s.pos }
func (s *pdfScanner) Seek(offset int64) error {
	if offset < 0 {
		return errors.New("seek out of range")
	}
	if err := s.ensure(offset); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if offset > int64(len(s.data)) {
		return errors.New("seek out of range")
	}
	s.pos = offset
	return nil
}
func (s *pdfScanner) SetNextStreamLength(n int64)               { s.nextStreamLen = n }
func (s *pdfScanner) SetRecoveryLocation(loc recovery.Location) { s.recLoc = loc }

func (s *pdfScanner) Next() (Token, error) {
	if err := s.skipWSAndComments(); err != nil {
		if errors.Is(err, io.EOF) {
			return Token{}, io.EOF
		}
		return Token{}, err
	}
	if s.pos >= int64(len(s.data)) {
		return Token{}, io.EOF
	}
	if err := s.ensure(s.pos); err != nil {
		if errors.Is(err, io.EOF) {
			return Token{}, io.EOF
		}
		return Token{}, err
	}
	start := s.pos
	c := s.data[s.pos]
	// Structural tokens
	switch c {
	case '<':
		if s.peekAhead(1) == '<' { // dictionary start
			s.pos += 2
			return s.emit(Token{Type: TokenDict, Value: "<<", Pos: start})
		}
		// hex string
		return s.scanHexString()
	case '>':
		if s.peekAhead(1) == '>' {
			s.pos += 2
			return s.emit(Token{Type: TokenKeyword, Value: ">>", Pos: start})
		}
		s.pos++
		return s.emit(Token{Type: TokenKeyword, Value: string(c), Pos: start})
	case '[':
		s.pos++
		return s.emit(Token{Type: TokenArray, Value: "[", Pos: start})
	case ']':
		s.pos++
		return s.emit(Token{Type: TokenKeyword, Value: "]", Pos: start})
	case '(':
		return s.scanLiteralString()
	case '/':
		return s.scanName()
	}
	// Keywords / numbers / booleans / null / ref
	if isDigitStart(c) {
		return s.scanNumberOrRef()
	}
	if isAlpha(c) {
		return s.scanKeyword()
	}
	// Fallback single char keyword
	s.pos++
	return s.emit(Token{Type: TokenKeyword, Value: string(c), Pos: start})
}

// Helpers
func (s *pdfScanner) skipWSAndComments() error {
	for {
		if s.pos >= int64(len(s.data)) {
			if err := s.ensure(s.pos); err != nil {
				return err
			}
		}
		if s.pos >= int64(len(s.data)) {
			return io.EOF
		}
		c := s.data[s.pos]
		// whitespace per PDF spec (space 0x20, tab, CR, LF, FF, null 0x00)
		if c == 0x00 || c == 0x09 || c == 0x0A || c == 0x0C || c == 0x0D || c == 0x20 {
			s.pos++
			continue
		}
		if c == '%' { // comment
			for {
				s.pos++
				if err := s.ensure(s.pos); err != nil && !errors.Is(err, io.EOF) {
					return err
				}
				if s.pos >= int64(len(s.data)) {
					return io.EOF
				}
				if s.data[s.pos] == '\n' || s.data[s.pos] == '\r' {
					break
				}
			}
			continue
		}
		return nil
	}
}

func (s *pdfScanner) ensure(n int64) error {
	for int64(len(s.data)) <= n {
		if s.eof {
			return io.EOF
		}
		if err := s.loadMore(); err != nil {
			return err
		}
	}
	return nil
}

func (s *pdfScanner) loadMore() error {
	buf := make([]byte, s.chunkSize)
	off := int64(len(s.data))
	n, err := s.reader.ReadAt(buf, off)
	if n > 0 {
		s.data = append(s.data, buf[:n]...)
	}
	if err == io.EOF {
		s.eof = true
		return nil
	}
	if err != nil {
		return err
	}
	if n == 0 {
		s.eof = true
	}
	return nil
}

func isDigitStart(c byte) bool { return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') }
func isAlpha(c byte) bool      { return unicode.IsLetter(rune(c)) }

func (s *pdfScanner) scanName() (Token, error) {
	start := s.pos
	s.pos++ // skip '/'
	var out bytes.Buffer
	for {
		if err := s.ensure(s.pos); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if s.pos >= int64(len(s.data)) {
			break
		}
		c := s.data[s.pos]
		if isDelimiter(c) {
			break
		}
		if c == '#' { // hex escape in name
			s.pos++
			a := s.hexNibble()
			b := s.hexNibble()
			out.WriteByte((a << 4) | b)
			continue
		}
		out.WriteByte(c)
		s.pos++
	}
	return s.emit(Token{Type: TokenName, Value: out.String(), Pos: start})
}

func (s *pdfScanner) hexNibble() byte {
	if s.pos >= int64(len(s.data)) {
		return 0
	}
	c := s.data[s.pos]
	s.pos++
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}

func (s *pdfScanner) scanLiteralString() (Token, error) { /* PDF 7.3.4.2 */
	start := s.pos
	s.pos++ // skip '('
	var buf bytes.Buffer
	depth := 1
	for {
		if err := s.ensure(s.pos); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if s.pos >= int64(len(s.data)) {
			break
		}
		c := s.data[s.pos]
		if c == '\\' { // escape
			s.pos++
			if err := s.ensure(s.pos); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return Token{}, err
			}
			if s.pos >= int64(len(s.data)) {
				break
			}
			esc := s.data[s.pos]
			// Line continuation: backslash followed by EOL is ignored
			if esc == '\r' {
				s.pos++
				if err := s.ensure(s.pos); err == nil && s.pos < int64(len(s.data)) && s.data[s.pos] == '\n' {
					s.pos++
				}
				continue
			}
			if esc == '\n' {
				s.pos++
				continue
			}
			// Octal escape up to 3 digits
			if esc >= '0' && esc <= '7' {
				val := int(esc - '0')
				s.pos++
				for k := 0; k < 2 && s.pos < int64(len(s.data)); k++ {
					if err := s.ensure(s.pos); err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						return Token{}, err
					}
					d := s.data[s.pos]
					if d < '0' || d > '7' {
						break
					}
					val = (val << 3) + int(d-'0')
					s.pos++
				}
				buf.WriteByte(byte(val))
				continue
			}
			buf.WriteByte(translateEscape(esc))
			s.pos++
			continue
		}
		if c == '(' {
			depth++
			buf.WriteByte(c)
			s.pos++
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				s.pos++
				break
			}
			buf.WriteByte(c)
			s.pos++
			continue
		}
		buf.WriteByte(c)
		s.pos++
		if s.cfg.MaxStringLength > 0 && int64(buf.Len()) > s.cfg.MaxStringLength {
			return Token{}, s.recover(errors.New("literal string too long"), "literal")
		}
	}
	if depth != 0 {
		if err := s.recover(errors.New("unterminated literal string"), "literal"); err != nil {
			if s.lastAction != recovery.ActionFix {
				return Token{}, err
			}
		}
	}
	return s.emit(Token{Type: TokenString, Value: buf.Bytes(), Pos: start})
}

func (s *pdfScanner) scanHexString() (Token, error) {
	start := s.pos
	s.pos++ // skip '<'
	var hexbuf []byte
	closed := false
	for {
		if err := s.ensure(s.pos); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if s.pos >= int64(len(s.data)) {
			break
		}
		c := s.data[s.pos]
		if c == '>' {
			s.pos++
			closed = true
			break
		}
		if isWhitespace(c) {
			s.pos++
			continue
		}
		hexbuf = append(hexbuf, c)
		s.pos++
	}
	if !closed {
		if err := s.recover(errors.New("unterminated hex string"), "hex"); err != nil {
			if s.lastAction != recovery.ActionFix {
				return Token{}, err
			}
		}
	}
	// If odd number of nibbles, pad with 0
	if len(hexbuf)%2 == 1 {
		hexbuf = append(hexbuf, '0')
	}
	if s.cfg.MaxStringLength > 0 && int64(len(hexbuf)/2) > s.cfg.MaxStringLength {
		return Token{}, s.recover(errors.New("hex string too long"), "hex")
	}
	out := make([]byte, 0, len(hexbuf)/2)
	for i := 0; i < len(hexbuf); i += 2 {
		a := fromHex(hexbuf[i])
		b := fromHex(hexbuf[i+1])
		out = append(out, (a<<4)|b)
	}
	return s.emit(Token{Type: TokenString, Value: out, Pos: start})
}

func fromHex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}

// scanStream consumes bytes until the next 'endstream' keyword.
func (s *pdfScanner) scanStream(start int64) (Token, error) {
	// Optional EOL after 'stream'
	if err := s.ensure(s.pos); err != nil && !errors.Is(err, io.EOF) {
		return Token{}, err
	}
	// PDF 7.3.8: stream keyword must be followed by EOL before data
	if s.pos >= int64(len(s.data)) {
		return Token{}, s.recover(errors.New("stream missing EOL before data"), "stream")
	}
	if s.data[s.pos] == '\r' {
		s.pos++
		if err := s.ensure(s.pos); err == nil && s.pos < int64(len(s.data)) && s.data[s.pos] == '\n' {
			s.pos++
		}
	} else if s.data[s.pos] == '\n' {
		s.pos++
	} else {
		return Token{}, s.recover(errors.New("stream missing EOL before data"), "stream")
	}
	dataStart := s.pos
	// If caller provided expected length, use it
	if s.nextStreamLen >= 0 {
		l := s.nextStreamLen
		if s.cfg.MaxStreamLength > 0 && l > s.cfg.MaxStreamLength {
			return Token{}, errors.New("stream too long")
		}
		if err := s.ensure(dataStart + l - 1); err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		} else if errors.Is(err, io.EOF) {
			if recErr := s.recover(errors.New("stream ended before declared length"), "stream"); recErr != nil {
				if s.lastAction != recovery.ActionFix {
					return Token{}, recErr
				}
			}
		}
		if dataStart+l > int64(len(s.data)) {
			l = int64(len(s.data)) - dataStart
		}
		end := dataStart + l
		payload := append([]byte(nil), s.data[dataStart:end]...)
		// consume optional EOL after data
		s.pos = end
		if err := s.ensure(s.pos); err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		}
		if s.pos < int64(len(s.data)) {
			if s.data[s.pos] == '\r' {
				s.pos++
				if err := s.ensure(s.pos); err == nil && s.pos < int64(len(s.data)) && s.data[s.pos] == '\n' {
					s.pos++
				}
			} else if s.data[s.pos] == '\n' {
				s.pos++
			}
		}
		// expect 'endstream'
		needle := []byte("endstream")
		if s.pos+int64(len(needle)) <= int64(len(s.data)) && bytes.Equal(s.data[s.pos:s.pos+int64(len(needle))], needle) {
			s.pos += int64(len(needle))
		} else {
			// fallback: search forward
			idx := bytes.Index(s.data[s.pos:], needle)
			if idx >= 0 {
				s.pos += int64(idx + len(needle))
			}
		}
		s.nextStreamLen = -1
		return s.emit(Token{Type: TokenStream, Value: payload, Pos: start})
	}
	needle := []byte("endstream")
	idx := -1
	for i := dataStart; ; i++ {
		if err := s.ensure(i + int64(len(needle)) - 1); err != nil {
			if errors.Is(err, io.EOF) {
				if i+int64(len(needle)) > int64(len(s.data)) {
					break
				}
			} else {
				return Token{}, err
			}
		}
		if s.cfg.MaxStreamScan > 0 && int64(i)-dataStart > s.cfg.MaxStreamScan {
			if recErr := s.recover(errors.New("endstream not found within scan limit"), "stream"); recErr != nil && s.lastAction != recovery.ActionFix {
				return Token{}, recErr
			}
			break
		}
		if i+int64(len(needle)) > int64(len(s.data)) {
			break
		}
		if s.data[i] != 'e' {
			continue
		}
		// require a line break or whitespace before and delimiter/whitespace after
		prevOK := hasStreamBreakBefore(s.data, i, int(dataStart))
		match := true
		for j := int64(0); j < int64(len(needle)); j++ {
			if s.data[i+j] != needle[j] {
				match = false
				break
			}
		}
		followOK := i+int64(len(needle)) >= int64(len(s.data)) || isDelimiter(s.data[i+int64(len(needle))])
		if match && prevOK && followOK {
			idx = int(i)
			break
		}
		if s.cfg.MaxStreamLength > 0 && int64(i)-dataStart > s.cfg.MaxStreamLength {
			return Token{}, s.recover(errors.New("stream too long"), "stream")
		}
	}
	if idx == -1 {
		payload := append([]byte(nil), s.data[dataStart:]...)
		if s.cfg.MaxStreamLength > 0 && int64(len(payload)) > s.cfg.MaxStreamLength {
			return Token{}, s.recover(errors.New("stream too long"), "stream")
		}
		if s.cfg.MaxStreamScan > 0 && int64(len(payload)) > s.cfg.MaxStreamScan {
			if recErr := s.recover(errors.New("endstream not found within scan limit"), "stream"); recErr != nil && s.lastAction != recovery.ActionFix {
				return Token{}, recErr
			}
		}
		s.pos = int64(len(s.data))
		return s.emit(Token{Type: TokenStream, Value: payload, Pos: start})
	}
	// Trim EOL before marker
	end := idx
	if end > int(dataStart) {
		if s.data[end-1] == '\n' {
			end--
			if end > int(dataStart) && s.data[end-1] == '\r' {
				end--
			}
		}
		if s.data[end-1] == '\r' {
			end--
		}
	}
	payload := append([]byte(nil), s.data[dataStart:end]...)
	if s.cfg.MaxStreamLength > 0 && int64(len(payload)) > s.cfg.MaxStreamLength {
		return Token{}, s.recover(errors.New("stream too long"), "stream")
	}
	// Advance position past 'endstream'
	s.pos = int64(idx + len(needle))
	return s.emit(Token{Type: TokenStream, Value: payload, Pos: start})
}

// scanInlineImage consumes bytes after the ID keyword until the first EOL-terminated EI delimiter.
// This is a content-stream-only construct; scanner does not interpret params.
func (s *pdfScanner) scanInlineImage(start int64) (Token, error) {
	// After ID there should be a single whitespace; consume one char if present.
	if err := s.ensure(s.pos); err != nil {
		if errors.Is(err, io.EOF) {
			return Token{}, s.recover(errors.New("unterminated inline image"), "inline_image")
		}
		return Token{}, err
	}
	if s.pos >= int64(len(s.data)) || !isWhitespace(s.data[s.pos]) {
		return Token{}, s.recover(errors.New("inline image missing required whitespace after ID"), "inline_image")
	}
	s.pos++
	// Optional EOL immediately after ID whitespace does not belong to data.
	if err := s.ensure(s.pos); err == nil && s.pos < int64(len(s.data)) {
		if s.data[s.pos] == '\r' {
			s.pos++
			if err := s.ensure(s.pos); err == nil && s.pos < int64(len(s.data)) && s.data[s.pos] == '\n' {
				s.pos++
			}
		} else if s.data[s.pos] == '\n' {
			s.pos++
		}
	}
	dataStart := s.pos
	// Search for EI preceded by whitespace and followed by delimiter/whitespace.
	for {
		if err := s.ensure(s.pos + 1); err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		}
		if s.pos+1 >= int64(len(s.data)) {
			return Token{}, errors.New("unterminated inline image")
		}
		if s.data[s.pos] == 'E' && s.data[s.pos+1] == 'I' {
			// must have whitespace (typically EOL) before and delimiter/whitespace after
			prevOK := s.pos > dataStart && isWhitespace(s.data[s.pos-1])
			// prefer actual line break to avoid false positives inside binary
			lineBreakBefore := s.pos > dataStart && isEOL(s.data[s.pos-1])
			var nextOK bool
			if err := s.ensure(s.pos + 2); err != nil && !errors.Is(err, io.EOF) {
				return Token{}, err
			}
			if s.pos+2 >= int64(len(s.data)) {
				nextOK = true
			} else {
				nextOK = isDelimiter(s.data[s.pos+2]) || isWhitespace(s.data[s.pos+2])
			}
			if prevOK && lineBreakBefore && nextOK {
				payload := append([]byte(nil), s.data[dataStart:s.pos]...)
				if s.cfg.MaxInlineImage > 0 && int64(len(payload)) > s.cfg.MaxInlineImage {
					return Token{}, s.recover(errors.New("inline image too long"), "inline_image")
				}
				s.pos += 2
				return s.emit(Token{Type: TokenInlineImage, Value: payload, Pos: start})
			}
		}
		s.pos++
		if s.cfg.MaxInlineImage > 0 && s.pos-dataStart > s.cfg.MaxInlineImage {
			return Token{}, s.recover(errors.New("inline image too long"), "inline_image")
		}
		if s.pos >= int64(len(s.data)) && s.eof {
			return Token{}, s.recover(errors.New("unterminated inline image"), "inline_image")
		}
	}
}

func isWhitespace(c byte) bool {
	return c == 0x00 || c == 0x09 || c == 0x0A || c == 0x0C || c == 0x0D || c == 0x20
}
func isEOL(c byte) bool { return c == '\r' || c == '\n' }
func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return isWhitespace(c)
	}
}

func translateEscape(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case '(':
		return '('
	case ')':
		return ')'
	case '\\':
		return '\\'
	default:
		return c
	}
}

func (s *pdfScanner) peekAhead(n int64) byte {
	if err := s.ensure(s.pos + n); err != nil {
		return 0
	}
	if s.pos+n >= int64(len(s.data)) {
		return 0
	}
	return s.data[s.pos+n]
}

func (s *pdfScanner) scanKeyword() (Token, error) {
	start := s.pos
	var buf bytes.Buffer
	for {
		if err := s.ensure(s.pos); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if s.pos >= int64(len(s.data)) {
			break
		}
		c := s.data[s.pos]
		if isDelimiter(c) {
			break
		}
		buf.WriteByte(c)
		s.pos++
	}
	kw := buf.String()
	switch kw {
	case "true", "false":
		return Token{Type: TokenBoolean, Value: kw == "true", Pos: start}, nil
	case "null":
		return Token{Type: TokenNull, Value: nil, Pos: start}, nil
	case "obj", "endobj", "endstream":
		return Token{Type: TokenKeyword, Value: kw, Pos: start}, nil
	case "stream":
		return s.scanStream(start)
	case "ID": // inline image data; caller should have parsed image dict already
		return s.scanInlineImage(start)
	default:
		return Token{Type: TokenKeyword, Value: kw, Pos: start}, nil
	}
}

func (s *pdfScanner) scanNumberOrRef() (Token, error) {
	start := s.pos
	// first number
	num1Str := s.scanNumberString()
	if num1Str == "" {
		return Token{}, errors.New("invalid number")
	}

	s.skipWSAndComments()
	secondStart := s.pos
	num2Str := s.scanNumberString()
	if num2Str != "" { // possible ref
		s.skipWSAndComments()
		if s.pos < int64(len(s.data)) && s.data[s.pos] == 'R' { // it's a ref
			s.pos++
			n1, _ := strconv.Atoi(num1Str)
			n2, _ := strconv.Atoi(num2Str)
			return Token{Type: TokenRef, Value: struct{ Num, Gen int }{Num: n1, Gen: n2}, Pos: start}, nil
		}
	}
	// not a ref; revert if we consumed second number
	if num2Str != "" {
		s.pos = secondStart
	} // parser will read second number later
	// produce number token from num1Str
	if i, err := strconv.ParseInt(num1Str, 10, 64); err == nil {
		return s.emit(Token{Type: TokenNumber, Value: i, Pos: start})
	}
	f, _ := strconv.ParseFloat(num1Str, 64)
	return s.emit(Token{Type: TokenNumber, Value: f, Pos: start})
}

func (s *pdfScanner) scanNumberString() string {
	start := s.pos
	var buf bytes.Buffer
	seenDigit := false
	for {
		if err := s.ensure(s.pos); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return ""
		}
		if s.pos >= int64(len(s.data)) {
			break
		}
		c := s.data[s.pos]
		if c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
			buf.WriteByte(c)
			if c >= '0' && c <= '9' {
				seenDigit = true
			}
			s.pos++
			continue
		}
		break
	}
	if !seenDigit {
		s.pos = start
		return ""
	}
	return buf.String()
}
func (s *pdfScanner) recover(err error, loc string) error {
	if s.cfg.Recovery == nil {
		return err
	}
	location := s.recLoc
	location.ByteOffset = s.pos
	if location.Component != "" {
		location.Component += "->"
	}
	location.Component += "scanner:" + loc
	action := s.cfg.Recovery.OnError(nil, err, recovery.Location{
		ByteOffset: location.ByteOffset,
		ObjectNum:  location.ObjectNum,
		ObjectGen:  location.ObjectGen,
		Component:  location.Component,
	})
	switch action {
	case recovery.ActionSkip:
		return nil
	case recovery.ActionFix:
		return nil
	case recovery.ActionWarn:
		return err
	default:
		return err
	}
}

func (s *pdfScanner) emit(tok Token) (Token, error) {
	switch tok.Type {
	case TokenArray:
		s.arrayDepth++
		if s.cfg.MaxArrayDepth > 0 && s.arrayDepth > s.cfg.MaxArrayDepth {
			return Token{}, s.recover(errors.New("array depth exceeded"), "array")
		}
	case TokenDict:
		s.dictDepth++
		if s.cfg.MaxDictDepth > 0 && s.dictDepth > s.cfg.MaxDictDepth {
			return Token{}, s.recover(errors.New("dict depth exceeded"), "dict")
		}
	case TokenKeyword:
		if tok.Value == "]" {
			if s.arrayDepth == 0 {
				return Token{}, s.recover(errors.New("array depth underflow"), "array")
			}
			s.arrayDepth--
		}
		if tok.Value == ">>" {
			if s.dictDepth == 0 {
				return Token{}, s.recover(errors.New("dict depth underflow"), "dict")
			}
			s.dictDepth--
		}
	}
	return tok, nil
}

// hasStreamBreakBefore returns true if the position i in data is preceded by a line break or whitespace boundary,
// making it a safe candidate for an endstream marker.
func hasStreamBreakBefore(data []byte, i int64, dataStart int) bool {
	if i == int64(dataStart) {
		return true
	}
	// Allow CR, LF, or CRLF right before marker
	if isEOL(data[i-1]) {
		return true
	}
	if i >= 2 && data[i-2] == '\r' && data[i-1] == '\n' {
		return true
	}
	// Fallback: whitespace boundary
	return isWhitespace(data[i-1])
}
