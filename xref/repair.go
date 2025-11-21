package xref

import (
	"context"
	"errors"
	"io"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/scanner"
)

// repair scans the entire file to reconstruct the xref table.
// It looks for "<num> <gen> obj" patterns and "trailer" dictionaries.
func repair(ctx context.Context, r io.ReaderAt, size int64) (Table, error) {
	// Use a lenient scanner config for repair
	s := scanner.New(r, scanner.Config{})
	entries := make(map[int]entry)
	var lastTrailer *raw.DictObj

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		tok, err := s.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Skip invalid tokens during repair scan
			continue
		}

		if tok.Type == scanner.TokenNumber && tok.IsInt {
			// Check for "<num> <gen> obj" pattern
			objNum := int(tok.Int)

			// We need to peek ahead. Since scanner doesn't support peek,
			// we consume and rely on the loop to continue if mismatch.
			// Note: This is a greedy scan.

			tokGen, err := s.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				continue
			}

			if tokGen.Type == scanner.TokenNumber && tokGen.IsInt {
				gen := int(tokGen.Int)

				tokObj, err := s.Next()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					continue
				}

				if tokObj.Type == scanner.TokenKeyword && tokObj.Str == "obj" {
					// Found object definition
					entries[objNum] = entry{offset: tok.Pos, gen: gen}
					continue
				}

				// Mismatch, but tokGen could be the start of an object.
				// Backtrack to tokGen to ensure we don't miss "1 2 0 obj" where we parsed "1 2 ..."
				if err := s.SeekTo(tokGen.Pos); err != nil {
					return nil, err
				}
				continue
			}
		} else if tok.Type == scanner.TokenKeyword && tok.Str == "trailer" {
			// Found trailer keyword, try to parse dictionary
			tr := &streamTokenReader{s: s}
			obj, err := parseObject(tr)
			if err == nil {
				if dict, ok := obj.(*raw.DictObj); ok {
					lastTrailer = dict
				}
			}
		}
	}

	if len(entries) == 0 {
		return nil, errors.New("repair failed: no objects found")
	}

	if lastTrailer == nil {
		// Construct minimal trailer if missing
		lastTrailer = raw.Dict()
		lastTrailer.Set(raw.NameObj{Val: "Size"}, raw.NumberObj{I: int64(len(entries)), IsInt: true})
	}

	return &table{entries: entries, trailer: lastTrailer}, nil
}
