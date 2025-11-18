package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"pdflib/ir"
	"pdflib/ir/decoded"
	"pdflib/ir/raw"
)

// Parses a PDF and performs a best-effort extraction of text show operators per page.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./examples/extract_text <pdf-path>\n")
		os.Exit(2)
	}

	if err := run(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "extract text: %v\n", err)
		os.Exit(1)
	}
}

func run(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	pipe := ir.NewDefault()
	doc, err := pipe.Parse(context.Background(), file)
	if err != nil {
		return fmt.Errorf("parse pdf: %w", err)
	}

	dec := doc.Decoded()
	if dec == nil || dec.Raw == nil {
		return errors.New("semantic document missing decoded/raw references")
	}

	pages := collectPages(dec.Raw)
	if len(pages) == 0 {
		return errors.New("no page dictionaries found")
	}

	fmt.Printf("Found %d page(s) in %s\n\n", len(pages), path)

	ctx := context.Background()
	for idx, page := range pages {
		text, err := extractPageText(ctx, page, dec)
		if err != nil {
			return fmt.Errorf("page %d: %w", idx+1, err)
		}
		if text == "" {
			fmt.Printf("Page %d: <no text operators>\n\n", idx+1)
			continue
		}
		fmt.Printf("Page %d:\n%s\n\n", idx+1, text)
	}

	return nil
}

func collectPages(doc *raw.Document) []*raw.DictObj {
	if doc == nil || doc.Trailer == nil {
		return nil
	}
	root, ok := doc.Trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return nil
	}
	pages := []*raw.DictObj{}
	walkPages(doc, root, func(page *raw.DictObj) {
		pages = append(pages, page)
	})
	return pages
}

func walkPages(doc *raw.Document, obj raw.Object, visit func(*raw.DictObj)) {
	dict := derefDict(doc, obj)
	if dict == nil {
		return
	}
	typObj, _ := dict.Get(raw.NameLiteral("Type"))
	if name, ok := typObj.(raw.Name); ok {
		switch name.Value() {
		case "Catalog":
			if pagesObj, ok := dict.Get(raw.NameLiteral("Pages")); ok {
				walkPages(doc, pagesObj, visit)
			}
		case "Pages":
			kidsObj, ok := dict.Get(raw.NameLiteral("Kids"))
			if !ok {
				return
			}
			arr := derefArray(doc, kidsObj)
			if arr == nil {
				return
			}
			for _, kid := range arr.Items {
				walkPages(doc, kid, visit)
			}
		case "Page":
			visit(dict)
		default:
			// ignore other node types
		}
		return
	}
	// Some PDFs omit Type entries; if a dictionary has Contents treat it as a page.
	if _, ok := dict.Get(raw.NameLiteral("Contents")); ok {
		visit(dict)
	}
}

func derefDict(doc *raw.Document, obj raw.Object) *raw.DictObj {
	resolved := deref(doc, obj)
	if dict, ok := resolved.(*raw.DictObj); ok {
		return dict
	}
	return nil
}

func derefArray(doc *raw.Document, obj raw.Object) *raw.ArrayObj {
	resolved := deref(doc, obj)
	if arr, ok := resolved.(*raw.ArrayObj); ok {
		return arr
	}
	return nil
}

func deref(doc *raw.Document, obj raw.Object) raw.Object {
	if ref, ok := obj.(raw.RefObj); ok {
		if doc != nil {
			if resolved, ok := doc.Objects[ref.Ref()]; ok {
				return resolved
			}
		}
	}
	return obj
}

func extractPageText(ctx context.Context, page *raw.DictObj, dec *decoded.DecodedDocument) (string, error) {
	if page == nil {
		return "", nil
	}
	contents, ok := page.Get(raw.NameLiteral("Contents"))
	if !ok {
		return "", nil
	}

	blobs := collectStreamData(dec, contents)
	if len(blobs) == 0 {
		return "", nil
	}

	var builder strings.Builder
	for _, data := range blobs {
		builder.WriteString(extractTextFromStream(data))
	}
	return strings.TrimSpace(builder.String()), nil
}

func collectStreamData(dec *decoded.DecodedDocument, obj raw.Object) [][]byte {
	switch v := obj.(type) {
	case raw.RefObj:
		if dec != nil {
			if stream, ok := dec.Streams[v.Ref()]; ok {
				return [][]byte{stream.Data()}
			}
		}
	case *raw.ArrayObj:
		var combined [][]byte
		for _, item := range v.Items {
			combined = append(combined, collectStreamData(dec, item)...)
		}
		return combined
	case raw.Stream:
		return [][]byte{v.RawData()}
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
