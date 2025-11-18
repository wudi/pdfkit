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
	MaxNameLength   int64
	MaxNumberLength int
	WindowSize      int64
	MaxBufferSize   int64
	Recovery        recovery.Strategy
}

type ReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
}

// pdfScanner incrementally buffers PDF data from a ReaderAt in fixed-size windows.
type pdfScanner struct {
	reader        ReaderAt
	data          []byte
	base          int64
	pos           int64
	cfg           Config
	nextStreamLen int64
	chunkSize     int64
	eof           bool
	arrayDepth    int
	dictDepth     int
	recLoc        recovery.Location
	lastAction    recovery.Action
	fixArrayClose int
	fixDictClose  int
	pin           int64
}

// New loads entire ReaderAt into memory and returns a scanner.
func New(r ReaderAt, cfg Config) Scanner {
	chunk := cfg.WindowSize
	if chunk <= 0 {
		chunk = 64 * 1024
	}
	return &pdfScanner{reader: r, cfg: cfg, nextStreamLen: -1, chunkSize: chunk, pin: -1}
}

func (s *pdfScanner) Position() int64 { return s.pos }
func (s *pdfScanner) Seek(offset int64) error {
	if offset < 0 {
		return errors.New("seek out of range")
	}
	if err := s.ensure(offset); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if offset < s.base || offset > s.base+int64(len(s.data)) {
		return errors.New("seek out of range")
	}
	s.pos = offset
	return nil
}
func (s *pdfScanner) SetNextStreamLength(n int64)               { s.nextStreamLen = n }
func (s *pdfScanner) SetRecoveryLocation(loc recovery.Location) { s.recLoc = loc }

func (s *pdfScanner) Next() (Token, error) {
	s.lastAction = recovery.ActionFail
	if s.fixArrayClose > 0 {
		s.fixArrayClose--
		return s.emit(Token{Type: TokenKeyword, Value: "]", Pos: s.pos})
	}
	if s.fixDictClose > 0 {
		s.fixDictClose--
		return s.emit(Token{Type: TokenKeyword, Value: ">>", Pos: s.pos})
	}
	if err := s.skipWSAndComments(); err != nil {
		if errors.Is(err, io.EOF) {
			if s.arrayDepth > 0 || s.dictDepth > 0 {
				if recErr := s.recover(errors.New("unexpected EOF inside container"), "eof"); recErr != nil && s.lastAction != recovery.ActionFix {
					return Token{}, recErr
				}
				if s.lastAction == recovery.ActionFix {
					s.fixArrayClose = s.arrayDepth
					s.fixDictClose = s.dictDepth
					return s.Next()
				}
			}
			return Token{}, io.EOF
		}
		return Token{}, err
	}
	c, err := s.byteAt(s.pos)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Token{}, io.EOF
		}
		return Token{}, err
	}
	start := s.pos
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
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			return err
		}
		// whitespace per PDF spec (space 0x20, tab, CR, LF, FF, null 0x00)
		if c == 0x00 || c == 0x09 || c == 0x0A || c == 0x0C || c == 0x0D || c == 0x20 {
			s.pos++
			continue
		}
		if c == '%' { // comment
			for {
				s.pos++
				ch, err := s.byteAt(s.pos)
				if err != nil {
					if errors.Is(err, io.EOF) {
						return io.EOF
					}
					return err
				}
				if ch == '\n' || ch == '\r' {
					break
				}
			}
			continue
		}
		return nil
	}
}

func (s *pdfScanner) ensure(n int64) error {
	if n < s.base {
		return s.reloadWindow(n)
	}
	for n >= s.base+int64(len(s.data)) {
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
	off := s.base + int64(len(s.data))
	n, err := s.reader.ReadAt(buf, off)
	if n > 0 {
		s.data = append(s.data, buf[:n]...)
		s.trimBuffer()
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

func (s *pdfScanner) reloadWindow(off int64) error {
	if off < 0 {
		return errors.New("seek out of range")
	}
	size := s.cfg.MaxBufferSize
	if size == 0 {
		size = s.chunkSize
	}
	buf := make([]byte, size)
	n, err := s.reader.ReadAt(buf, off)
	s.base = off
	s.data = buf[:n]
	s.eof = err == io.EOF
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if n == 0 && err == nil {
		s.eof = true
	}
	return nil
}

func (s *pdfScanner) trimBuffer() {
	if s.cfg.MaxBufferSize <= 0 {
		return
	}
	if int64(len(s.data)) <= s.cfg.MaxBufferSize {
		return
	}
	excess := int64(len(s.data)) - s.cfg.MaxBufferSize
	// Preserve the pinned region (e.g., start of a stream payload) and current position.
	keep := s.pos
	if s.pin >= 0 && s.pin < keep {
		keep = s.pin
	}
	maxDrop := keep - s.base
	if maxDrop < 0 {
		maxDrop = 0
	}
	if excess > maxDrop {
		excess = maxDrop
	}
	if excess <= 0 {
		return
	}
	s.data = s.data[excess:]
	s.base += excess
}

func (s *pdfScanner) byteAt(off int64) (byte, error) {
	if err := s.ensure(off); err != nil {
		return 0, err
	}
	idx := off - s.base
	if idx < 0 || idx >= int64(len(s.data)) {
		return 0, io.EOF
	}
	return s.data[idx], nil
}

func (s *pdfScanner) tailFrom(off int64) ([]byte, error) {
	if err := s.ensure(off); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	idx := off - s.base
	if idx < 0 || idx > int64(len(s.data)) {
		return nil, io.EOF
	}
	return s.data[idx:], nil
}

func (s *pdfScanner) slice(start, end int64) ([]byte, error) {
	if end < start {
		return nil, errors.New("invalid slice")
	}
	if err := s.ensure(end - 1); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sIdx := start - s.base
	eIdx := end - s.base
	if sIdx < 0 || eIdx > int64(len(s.data)) {
		return nil, io.EOF
	}
	return s.data[sIdx:eIdx], nil
}

func isDigitStart(c byte) bool { return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') }
func isAlpha(c byte) bool      { return unicode.IsLetter(rune(c)) }

func (s *pdfScanner) scanName() (Token, error) {
	start := s.pos
	s.pos++ // skip '/'
	var out bytes.Buffer
	for {
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
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
		if s.cfg.MaxNameLength > 0 && int64(out.Len()) > s.cfg.MaxNameLength {
			return Token{}, s.recover(errors.New("name too long"), "name")
		}
	}
	return s.emit(Token{Type: TokenName, Value: out.String(), Pos: start})
}

func (s *pdfScanner) hexNibble() byte {
	c, err := s.byteAt(s.pos)
	if err != nil {
		return 0
	}
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
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if s.pos < start {
			break
		}
		if c == '\\' { // escape
			s.pos++
			esc, err := s.byteAt(s.pos)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return Token{}, err
			}
			if s.pos < start {
				break
			}
			// Line continuation: backslash followed by EOL is ignored
			if esc == '\r' {
				s.pos++
				if next, err := s.byteAt(s.pos); err == nil && next == '\n' {
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
				for k := 0; k < 2; k++ {
					d, err := s.byteAt(s.pos)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						return Token{}, err
					}
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
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
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
	c, err := s.byteAt(s.pos)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Token{}, s.recover(errors.New("stream missing EOL before data"), "stream")
		}
		return Token{}, err
	}
	// PDF 7.3.8: stream keyword must be followed by EOL before data
	if c == '\r' {
		s.pos++
		if next, err := s.byteAt(s.pos); err == nil && next == '\n' {
			s.pos++
		}
	} else if c == '\n' {
		s.pos++
	} else {
		return Token{}, s.recover(errors.New("stream missing EOL before data"), "stream")
	}
	dataStart := s.pos
	defer func() { s.pin = -1 }()
	s.pin = dataStart
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
		end := dataStart + l
		availableEnd := s.base + int64(len(s.data))
		if end > availableEnd {
			end = availableEnd
		}
		payloadSlice, err := s.slice(dataStart, end)
		if err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		}
		payload := append([]byte(nil), payloadSlice...)
		// consume optional EOL after data
		s.pos = end
		if b, err := s.byteAt(s.pos); err == nil {
			if b == '\r' {
				s.pos++
				if next, err := s.byteAt(s.pos); err == nil && next == '\n' {
					s.pos++
				}
			} else if b == '\n' {
				s.pos++
			}
		}
		// expect 'endstream'
		needle := []byte("endstream")
		tail, _ := s.tailFrom(s.pos)
		if len(tail) >= len(needle) && bytes.Equal(tail[:len(needle)], needle) {
			s.pos += int64(len(needle))
		} else if idx := bytes.Index(tail, needle); idx >= 0 {
			s.pos += int64(idx + len(needle))
		}
		s.nextStreamLen = -1
		return s.emit(Token{Type: TokenStream, Value: payload, Pos: start})
	}
	needle := []byte("endstream")
	idx := -1
	dataTail, err := s.tailFrom(dataStart)
	if err != nil && !errors.Is(err, io.EOF) {
		return Token{}, err
	}
	for i := int64(0); i < int64(len(dataTail)); i++ {
		if s.cfg.MaxStreamScan > 0 && i > s.cfg.MaxStreamScan {
			if recErr := s.recover(errors.New("endstream not found within scan limit"), "stream"); recErr != nil && s.lastAction != recovery.ActionFix {
				return Token{}, recErr
			}
			break
		}
		if i+int64(len(needle)) > int64(len(dataTail)) {
			break
		}
		if dataTail[i] != 'e' {
			continue
		}
		if !hasStreamBreakBefore(dataTail, i, 0) {
			continue
		}
		if bytes.Equal(dataTail[i:int64(len(needle))+i], needle) {
			followIdx := i + int64(len(needle))
			if followIdx >= int64(len(dataTail)) || isDelimiter(dataTail[followIdx]) {
				idx = int(dataStart + i)
				break
			}
		}
		if s.cfg.MaxStreamLength > 0 && i > s.cfg.MaxStreamLength {
			return Token{}, s.recover(errors.New("stream too long"), "stream")
		}
	}
	if idx == -1 {
		payload := append([]byte(nil), dataTail...)
		if s.cfg.MaxStreamLength > 0 && int64(len(payload)) > s.cfg.MaxStreamLength {
			return Token{}, s.recover(errors.New("stream too long"), "stream")
		}
		if s.cfg.MaxStreamScan > 0 && int64(len(payload)) > s.cfg.MaxStreamScan {
			if recErr := s.recover(errors.New("endstream not found within scan limit"), "stream"); recErr != nil && s.lastAction != recovery.ActionFix {
				return Token{}, recErr
			}
		}
		s.pos = dataStart + int64(len(payload))
		return s.emit(Token{Type: TokenStream, Value: payload, Pos: start})
	}
	// Trim EOL before marker
	end := int64(idx)
	relStart := dataStart
	if end > relStart {
		if b := dataTail[end-relStart-1]; b == '\n' {
			end--
			if end > relStart && dataTail[end-relStart-1] == '\r' {
				end--
			}
		}
		if end > relStart && dataTail[end-relStart-1] == '\r' {
			end--
		}
	}
	payloadSlice, err := s.slice(dataStart, end)
	if err != nil && !errors.Is(err, io.EOF) {
		return Token{}, err
	}
	payload := append([]byte(nil), payloadSlice...)
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
	c, err := s.byteAt(s.pos)
	if err != nil {
		return Token{}, err
	}
	if !isWhitespace(c) {
		return Token{}, s.recover(errors.New("inline image missing required whitespace after ID"), "inline_image")
	}
	s.pos++
	// Optional EOL immediately after ID whitespace does not belong to data.
	if next, err := s.byteAt(s.pos); err == nil {
		if next == '\r' {
			s.pos++
			if val, err := s.byteAt(s.pos); err == nil && val == '\n' {
				s.pos++
			}
		} else if next == '\n' {
			s.pos++
		}
	}
	dataStart := s.pos
	defer func() { s.pin = -1 }()
	s.pin = dataStart
	var best int64 = -1
	for {
		if err := s.ensure(s.pos + 1); err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		}
		if s.pos+1 >= s.base+int64(len(s.data)) && s.eof {
			break
		}
		cur, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		next, err := s.byteAt(s.pos + 1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
		if cur == 'E' && next == 'I' {
			// must have whitespace (typically EOL) before and delimiter/whitespace after
			prevByte, _ := s.byteAt(s.pos - 1)
			prevOK := s.pos > dataStart && isWhitespace(prevByte)
			// prefer actual line break to avoid false positives inside binary
			lineBreakBefore := s.pos > dataStart && isEOL(prevByte)
			var nextOK bool
			if err := s.ensure(s.pos + 2); err != nil && !errors.Is(err, io.EOF) {
				return Token{}, err
			}
			if s.pos+2 >= s.base+int64(len(s.data)) {
				nextOK = true
			} else {
				after, _ := s.byteAt(s.pos + 2)
				nextOK = isDelimiter(after) || isWhitespace(after)
			}
			if prevOK && lineBreakBefore && nextOK {
				best = s.pos
			}
		}
		s.pos++
		if s.cfg.MaxInlineImage > 0 && s.pos-dataStart > s.cfg.MaxInlineImage {
			break
		}
		if s.pos >= s.base+int64(len(s.data)) && s.eof {
			break
		}
	}
	if best >= 0 {
		payloadSlice, err := s.slice(dataStart, best)
		if err != nil && !errors.Is(err, io.EOF) {
			return Token{}, err
		}
		payload := append([]byte(nil), payloadSlice...)
		if s.cfg.MaxInlineImage > 0 && int64(len(payload)) > s.cfg.MaxInlineImage {
			return Token{}, s.recover(errors.New("inline image too long"), "inline_image")
		}
		s.pos = best + 2
		return s.emit(Token{Type: TokenInlineImage, Value: payload, Pos: start})
	}
	if s.cfg.MaxInlineImage > 0 && s.pos-dataStart > s.cfg.MaxInlineImage {
		return Token{}, s.recover(errors.New("inline image too long"), "inline_image")
	}
	return Token{}, s.recover(errors.New("unterminated inline image"), "inline_image")
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
	idx := s.pos + n - s.base
	if idx < 0 || idx >= int64(len(s.data)) {
		return 0
	}
	return s.data[idx]
}

func (s *pdfScanner) scanKeyword() (Token, error) {
	start := s.pos
	var buf bytes.Buffer
	for {
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Token{}, err
		}
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
		if c, err := s.byteAt(s.pos); err == nil && c == 'R' { // it's a ref
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
	dotSeen := false
	for {
		c, err := s.byteAt(s.pos)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return ""
		}
		if s.pos < start {
			break
		}
		switch {
		case c == '+' || c == '-':
			if buf.Len() > 0 {
				return s.finishNumber(start, &buf, seenDigit)
			}
			buf.WriteByte(c)
			s.pos++
		case c == '.':
			if dotSeen {
				return s.finishNumber(start, &buf, seenDigit)
			}
			dotSeen = true
			buf.WriteByte(c)
			s.pos++
		case c >= '0' && c <= '9':
			buf.WriteByte(c)
			seenDigit = true
			s.pos++
		default:
			return s.finishNumber(start, &buf, seenDigit)
		}
	}
	return s.finishNumber(start, &buf, seenDigit)
}

func (s *pdfScanner) finishNumber(start int64, buf *bytes.Buffer, seenDigit bool) string {
	if !seenDigit {
		s.pos = start
		return ""
	}
	if s.cfg.MaxNumberLength > 0 && buf.Len() > s.cfg.MaxNumberLength {
		_ = s.recover(errors.New("number too long"), "number")
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
	s.lastAction = action
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
			if err := s.recover(errors.New("array depth exceeded"), "array"); err != nil && s.lastAction != recovery.ActionFix {
				return Token{}, err
			}
			if s.lastAction == recovery.ActionFix {
				s.arrayDepth = s.cfg.MaxArrayDepth
			}
		}
	case TokenDict:
		s.dictDepth++
		if s.cfg.MaxDictDepth > 0 && s.dictDepth > s.cfg.MaxDictDepth {
			if err := s.recover(errors.New("dict depth exceeded"), "dict"); err != nil && s.lastAction != recovery.ActionFix {
				return Token{}, err
			}
			if s.lastAction == recovery.ActionFix {
				s.dictDepth = s.cfg.MaxDictDepth
			}
		}
	case TokenKeyword:
		if tok.Value == "]" {
			if s.arrayDepth == 0 {
				if err := s.recover(errors.New("array depth underflow"), "array"); err != nil && s.lastAction != recovery.ActionFix {
					return Token{}, err
				}
				if s.lastAction == recovery.ActionFix {
					// drop the unmatched closing and move to next token
					return s.Next()
				}
				return Token{}, nil
			}
			s.arrayDepth--
		}
		if tok.Value == ">>" {
			if s.dictDepth == 0 {
				if err := s.recover(errors.New("dict depth underflow"), "dict"); err != nil && s.lastAction != recovery.ActionFix {
					return Token{}, err
				}
				if s.lastAction == recovery.ActionFix {
					return s.Next()
				}
				return Token{}, nil
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
