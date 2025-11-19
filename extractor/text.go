package extractor

import (
	"strings"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
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
		var builder strings.Builder
		for _, data := range blobs {
			builder.WriteString(extractTextFromStream(data))
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
		if data, _, ok := streamData(dec, v); ok {
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

func extractTextFromStream(data []byte) string {
	var out strings.Builder
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '(':
			text, next := readStringLiteral(data, i)
			i = next
			j := skipWhitespace(data, i)
			if hasOperator(data[j:], "Tj") {
				out.WriteString(text)
				out.WriteByte('\n')
				i = j + 2
			}
		case '[':
			end := findArrayEnd(data, i)
			if end == -1 {
				continue
			}
			j := skipWhitespace(data, end+1)
			if hasOperator(data[j:], "TJ") {
				text := extractArrayStrings(data[i+1 : end])
				if text != "" {
					out.WriteString(text)
					out.WriteByte('\n')
				}
				i = j + 2
			} else {
				i = end
			}
		}
	}
	return out.String()
}

func readStringLiteral(data []byte, start int) (string, int) {
	var out strings.Builder
	escaped := false
	depth := 0
	for i := start + 1; i < len(data); i++ {
		b := data[i]
		if escaped {
			out.WriteByte(unescapeByte(b))
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '(':
			depth++
			out.WriteByte(b)
		case ')':
			if depth == 0 {
				return out.String(), i + 1
			}
			depth--
			out.WriteByte(b)
		default:
			out.WriteByte(b)
		}
	}
	return out.String(), len(data)
}

func unescapeByte(b byte) byte {
	switch b {
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
	default:
		return b
	}
}

func skipWhitespace(data []byte, i int) int {
	for i < len(data) {
		if isDelimiter(data[i]) {
			if isWhitespace(data[i]) {
				i++
				continue
			}
			return i
		}
		if data[i] == 0 {
			i++
			continue
		}
		if data[i] <= 32 {
			i++
			continue
		}
		return i
	}
	return i
}

func isWhitespace(b byte) bool {
	switch b {
	case 0, 9, 10, 12, 13, 32:
		return true
	default:
		return false
	}
}

func isDelimiter(b byte) bool {
	switch b {
	case 0, '(', ')', '<', '>', '[', ']', '{', '}', '/', '%', 9, 10, 12, 13, 32:
		return true
	default:
		return false
	}
}

func hasOperator(data []byte, op string) bool {
	if len(data) < len(op) {
		return false
	}
	if !strings.HasPrefix(string(data), op) {
		return false
	}
	if len(data) == len(op) {
		return true
	}
	return isDelimiter(data[len(op)])
}

func findArrayEnd(data []byte, start int) int {
	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func extractArrayStrings(data []byte) string {
	var out strings.Builder
	for i := 0; i < len(data); i++ {
		if data[i] == '(' {
			text, next := readStringLiteral(data, i)
			out.WriteString(text)
			i = next - 1
		}
	}
	return out.String()
}
