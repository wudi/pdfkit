package xref

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"pdflib/recovery"
)

// Table holds object offsets for a classic xref table.
type Table interface {
	Lookup(objNum int) (offset int64, gen int, found bool)
	Objects() []int
	Type() string
}

// Resolver locates and parses xref information in a PDF.
type Resolver interface {
	Resolve(ctx context.Context, r io.ReaderAt) (Table, error)
	Linearized() bool
	Incremental() []Table
}

type ResolverConfig struct {
	MaxXRefDepth int
	Recovery     recovery.Strategy
}

// NewResolver returns a basic classic-table resolver.
func NewResolver(cfg ResolverConfig) Resolver {
	return &tableResolver{}
}

// tableResolver implements classic (non-stream) xref parsing for simple PDFs.
type tableResolver struct{}

func (t *tableResolver) Resolve(ctx context.Context, r io.ReaderAt) (Table, error) {
	data := readAll(r)

	startxref := bytes.LastIndex(data, []byte("startxref"))
	if startxref < 0 {
		return nil, errors.New("startxref not found")
	}
	rest := data[startxref+len("startxref"):]
	lines := bufio.NewScanner(bytes.NewReader(rest))
	var offset int64
	for lines.Scan() {
		text := strings.TrimSpace(lines.Text())
		if text == "" {
			continue
		}
		val, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse startxref: %w", err)
		}
		offset = val
		break
	}

	if offset <= 0 || offset >= int64(len(data)) {
		return nil, fmt.Errorf("xref offset out of range: %d", offset)
	}

	tableData := data[offset:]
	sc := bufio.NewScanner(bytes.NewReader(tableData))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "xref" {
		return nil, errors.New("xref keyword not found at offset")
	}

	entries := make(map[int]entry)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "trailer") {
			break
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid xref subsection header: %q", line)
		}
		startObj, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse xref start: %w", err)
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse xref count: %w", err)
		}

		for i := 0; i < count; i++ {
			if !sc.Scan() {
				return nil, errors.New("unexpected end of xref section")
			}
			entryLine := strings.TrimSpace(sc.Text())
			fields := strings.Fields(entryLine)
			if len(fields) < 3 {
				return nil, fmt.Errorf("invalid xref entry: %q", entryLine)
			}
			off, err := strconv.ParseInt(fields[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse xref offset: %w", err)
			}
			gen, err := strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("parse xref gen: %w", err)
			}
			if len(fields[2]) == 0 || fields[2][0] != 'n' {
				continue // free entry
			}
			entries[startObj+i] = entry{offset: off, gen: gen}
		}
	}

	return &table{entries: entries}, nil
}

func (t *tableResolver) Linearized() bool     { return false }
func (t *tableResolver) Incremental() []Table { return nil }

type entry struct {
	offset int64
	gen    int
}

type table struct {
	entries map[int]entry
}

func (t *table) Lookup(objNum int) (int64, int, bool) {
	e, ok := t.entries[objNum]
	if !ok {
		return 0, 0, false
	}
	return e.offset, e.gen, true
}

func (t *table) Objects() []int {
	out := make([]int, 0, len(t.entries))
	for k := range t.entries {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func (t *table) Type() string { return "table" }

func readAll(r io.ReaderAt) []byte {
	var buf bytes.Buffer
	const chunk = int64(32 * 1024)
	for off := int64(0); ; off += chunk {
		tmp := make([]byte, chunk)
		n, err := r.ReadAt(tmp, off)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
		if int64(n) < chunk {
			break
		}
	}
	return buf.Bytes()
}
