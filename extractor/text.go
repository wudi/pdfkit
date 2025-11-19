package extractor

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf16"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
	"pdflib/scanner"
)

// PageText captures extracted text per page along with optional labels.
type PageText struct {
	Page    int
	Label   string
	Content string
}

// ExtractText returns best-effort text content for each page by scanning show operators.
func (e *Extractor) ExtractText() ([]PageText, error) {
	var out []PageText
	for idx, page := range e.pages {
		blobs := collectContentStreams(e.dec, valueFromDict(page, "Contents"))
		if len(blobs) == 0 {
			continue
		}
		fonts := e.fontDecodersForPage(page)
		var builder strings.Builder
		for _, data := range blobs {
			builder.WriteString(extractTextFromStream(data, fonts))
		}
		txt := strings.TrimSpace(builder.String())
		if txt == "" {
			continue
		}
		out = append(out, PageText{
			Page:    idx,
			Label:   e.pageLabels[idx],
			Content: txt,
		})
	}
	return out, nil
}

func collectContentStreams(dec *decoded.DecodedDocument, obj raw.Object) [][]byte {
	switch v := obj.(type) {
	case raw.RefObj:
		if data, _, _, ok := streamData(dec, v); ok {
			return [][]byte{data}
		}
	case *raw.ArrayObj:
		var combined [][]byte
		for _, item := range v.Items {
			combined = append(combined, collectContentStreams(dec, item)...)
		}
		return combined
	case raw.Stream:
		data := v.RawData()
		copyData := make([]byte, len(data))
		copy(copyData, data)
		return [][]byte{copyData}
	}
	return nil
}

func extractTextFromStream(data []byte, fonts map[string]*fontDecoder) string {
	tr := newTokenReader(data)
	if tr == nil {
		return ""
	}
	var operands []raw.Object
	var out strings.Builder
	currentFont := ""

	for {
		tok, err := tr.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		if tok.Type == scanner.TokenKeyword {
			op, _ := tok.Value.(string)
			switch op {
			case "BT":
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
			case "Tf":
				if len(operands) >= 2 {
					if name, _ := nameFromObject(operands[len(operands)-2]); name != "" {
						currentFont = name
					}
				}
			case "Tj":
				appendTextOperand(&out, operands, currentFont, fonts, false)
			case "'", "\"":
				appendTextOperand(&out, operands, currentFont, fonts, true)
			case "TJ":
				appendArrayTextOperand(&out, operands, currentFont, fonts, false)
			case "T*":
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
			case "Td", "TD":
				if len(operands) >= 2 {
					if dy, ok := floatFromObject(operands[len(operands)-1]); ok && dy != 0 {
						if out.Len() > 0 {
							out.WriteByte('\n')
						}
					}
				}
			}
			operands = operands[:0]
			continue
		}
		tr.unread(tok)
		operand, err := parseObject(tr)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		operands = append(operands, operand)
	}

	return out.String()
}

func appendTextOperand(out *strings.Builder, operands []raw.Object, currentFont string, fonts map[string]*fontDecoder, newline bool) {
	if len(operands) == 0 {
		return
	}
	data := bytesFromStringObject(operands[len(operands)-1])
	if len(data) == 0 {
		return
	}
	text := decodeTextBytes(data, fonts[currentFont])
	if text == "" {
		return
	}
	if newline && out.Len() > 0 {
		out.WriteByte('\n')
	}
	out.WriteString(text)
}

func appendArrayTextOperand(out *strings.Builder, operands []raw.Object, currentFont string, fonts map[string]*fontDecoder, newline bool) {
	if len(operands) == 0 {
		return
	}
	arr, _ := operands[len(operands)-1].(*raw.ArrayObj)
	if arr == nil {
		return
	}
	var line strings.Builder
	for _, item := range arr.Items {
		data := bytesFromStringObject(item)
		if len(data) == 0 {
			continue
		}
		line.WriteString(decodeTextBytes(data, fonts[currentFont]))
	}
	text := line.String()
	if text == "" {
		return
	}
	if newline && out.Len() > 0 {
		out.WriteByte('\n')
	}
	out.WriteString(text)
}

func bytesFromStringObject(obj raw.Object) []byte {
	switch v := obj.(type) {
	case raw.StringObj:
		return append([]byte(nil), v.Value()...)
	case raw.HexStringObj:
		return append([]byte(nil), v.Value()...)
	}
	if s, ok := obj.(raw.String); ok {
		return append([]byte(nil), s.Value()...)
	}
	return nil
}

func decodeTextBytes(data []byte, decoder *fontDecoder) string {
	if len(data) == 0 {
		return ""
	}
	if decoder != nil && decoder.cmap != nil {
		return decoder.cmap.decode(data)
	}
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return decodeUTF16BE(data[2:])
	}
	return string(data)
}

func decodeUTF16BE(data []byte) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return ""
	}
	buf := make([]uint16, len(data)/2)
	for i := 0; i < len(buf); i++ {
		buf[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
	}
	runes := utf16.Decode(buf)
	return string(runes)
}

func (e *Extractor) fontDecodersForPage(page *raw.DictObj) map[string]*fontDecoder {
	resources := derefDict(e.raw, valueFromDict(page, "Resources"))
	if resources == nil {
		return nil
	}
	fontsDict := derefDict(e.raw, valueFromDict(resources, "Font"))
	if fontsDict == nil {
		return nil
	}
	decoders := make(map[string]*fontDecoder)
	for name, fontObj := range fontsDict.KV {
		decoder := e.fontDecoder(fontObj)
		if decoder != nil {
			decoders[name] = decoder
		}
	}
	return decoders
}

func (e *Extractor) fontDecoder(obj raw.Object) *fontDecoder {
	if ref, ok := obj.(raw.RefObj); ok {
		if e.fontCache == nil {
			e.fontCache = make(map[raw.ObjectRef]*fontDecoder)
		}
		if cached, ok := e.fontCache[ref.Ref()]; ok {
			return cached
		}
		decoder := e.parseFontDecoder(ref)
		e.fontCache[ref.Ref()] = decoder
		return decoder
	}
	return e.parseFontDecoder(obj)
}

func (e *Extractor) parseFontDecoder(obj raw.Object) *fontDecoder {
	dict := derefDict(e.raw, obj)
	if dict == nil {
		return nil
	}
	decoder := &fontDecoder{}
	if cmapObj := valueFromDict(dict, "ToUnicode"); cmapObj != nil {
		if data, _ := e.streamBytes(cmapObj); len(data) > 0 {
			decoder.cmap = parseToUnicodeCMap(data)
		}
	}
	return decoder
}

type fontDecoder struct {
	cmap *toUnicodeMap
}

type toUnicodeMap struct {
	entries map[string]string
	lengths []int
}

func parseToUnicodeCMap(data []byte) *toUnicodeMap {
	lineScanner := bufio.NewScanner(bytes.NewReader(data))
	result := &toUnicodeMap{entries: make(map[string]string)}
	lengthSet := make(map[int]struct{})
	state := ""
	for lineScanner.Scan() {
		line := strings.TrimSpace(lineScanner.Text())
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}
		switch {
		case strings.HasSuffix(line, "begincodespacerange"):
			state = "codespace"
			continue
		case strings.HasSuffix(line, "endcodespacerange"):
			state = ""
			continue
		case strings.HasSuffix(line, "beginbfchar"):
			state = "bfchar"
			continue
		case strings.HasSuffix(line, "endbfchar"):
			state = ""
			continue
		case strings.HasSuffix(line, "beginbfrange"):
			state = "bfrange"
			continue
		case strings.HasSuffix(line, "endbfrange"):
			state = ""
			continue
		}
		switch state {
		case "codespace":
			hexes := extractHexTokens(line)
			if len(hexes) >= 1 {
				if b := hexToBytes(hexes[0]); len(b) > 0 {
					lengthSet[len(b)] = struct{}{}
				}
			}
		case "bfchar":
			hexes := extractHexTokens(line)
			if len(hexes) >= 2 {
				src := hexToBytes(hexes[0])
				dst := decodeUTF16BE(hexToBytes(hexes[1]))
				if len(src) > 0 {
					result.entries[string(src)] = dst
					lengthSet[len(src)] = struct{}{}
				}
			}
		case "bfrange":
			line = accumulateUntil(line, lineScanner)
			hexes := extractHexTokens(line)
			if len(hexes) < 3 {
				continue
			}
			srcStart := hexToBytes(hexes[0])
			srcEnd := hexToBytes(hexes[1])
			length := len(srcStart)
			lengthSet[length] = struct{}{}
			startVal := bytesToInt(srcStart)
			endVal := bytesToInt(srcEnd)
			if strings.Contains(line, "[") {
				for i := 0; i <= endVal-startVal && 2+i < len(hexes); i++ {
					src := intToBytes(startVal+i, length)
					dst := decodeUTF16BE(hexToBytes(hexes[2+i]))
					result.entries[string(src)] = dst
				}
			} else {
				dstStart := hexToBytes(hexes[2])
				dstVal := bytesToInt(dstStart)
				dstLen := len(dstStart)
				for i := 0; i <= endVal-startVal; i++ {
					src := intToBytes(startVal+i, length)
					dst := intToBytes(dstVal+i, dstLen)
					result.entries[string(src)] = decodeUTF16BE(dst)
				}
			}
		}
	}
	if len(lengthSet) == 0 {
		for k := range result.entries {
			lengthSet[len(k)] = struct{}{}
		}
	}
	for l := range lengthSet {
		result.lengths = append(result.lengths, l)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(result.lengths)))
	return result
}

func accumulateUntil(line string, lineScanner *bufio.Scanner) string {
	if strings.Contains(line, "]") {
		return line
	}
	for lineScanner.Scan() {
		next := strings.TrimSpace(lineScanner.Text())
		line += " " + next
		if strings.Contains(next, "]") {
			break
		}
	}
	return line
}

func extractHexTokens(line string) []string {
	var tokens []string
	for {
		start := strings.Index(line, "<")
		if start == -1 {
			break
		}
		end := strings.Index(line[start+1:], ">")
		if end == -1 {
			break
		}
		segment := line[start+1 : start+1+end]
		tokens = append(tokens, strings.ReplaceAll(segment, " ", ""))
		line = line[start+1+end+1:]
	}
	return tokens
}

func hexToBytes(hex string) []byte {
	if len(hex)%2 == 1 {
		hex += "0"
	}
	out := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		out[i/2] = (fromHexChar(hex[i]) << 4) | fromHexChar(hex[i+1])
	}
	return out
}

func fromHexChar(c byte) byte {
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

func bytesToInt(b []byte) int {
	val := 0
	for _, by := range b {
		val = (val << 8) | int(by)
	}
	return val
}

func intToBytes(value int, length int) []byte {
	buf := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		buf[i] = byte(value & 0xFF)
		value >>= 8
	}
	return buf
}

func (m *toUnicodeMap) decode(data []byte) string {
	if len(m.lengths) == 0 {
		return string(data)
	}
	var out strings.Builder
	for len(data) > 0 {
		matched := false
		for _, l := range m.lengths {
			if len(data) < l {
				continue
			}
			key := string(data[:l])
			if val, ok := m.entries[key]; ok {
				out.WriteString(val)
				data = data[l:]
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(data[0])
			data = data[1:]
		}
	}
	return out.String()
}

func newTokenReader(data []byte) *tokenReader {
	reader := bytes.NewReader(data)
	sc := scanner.New(reader, scanner.Config{})
	return &tokenReader{s: sc}
}
