package xref

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
)

func buildSimplePDF() ([]byte, map[int]int64) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	offsets := make(map[int]int64)

	offsets[1] = int64(buf.Len())
	buf.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")

	offsets[2] = int64(buf.Len())
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n")

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 3\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 2; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	buf.WriteString("trailer\n<< /Size 3 /Root 1 0 R >>\n")
	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes(), offsets
}

type readerAt struct {
	data []byte
}

func (r *readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if off+int64(n) >= int64(len(r.data)) {
		return n, io.EOF
	}
	return n, nil
}

func TestResolverParsesXRefTable(t *testing.T) {
	pdf, offsets := buildSimplePDF()
	r := &readerAt{data: pdf}

	resolver := NewResolver(ResolverConfig{})
	table, err := resolver.Resolve(context.Background(), r)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	for obj, off := range offsets {
		gotOff, gen, ok := table.Lookup(obj)
		if !ok {
			t.Fatalf("missing object %d", obj)
		}
		if gotOff != off || gen != 0 {
			t.Fatalf("object %d: expected (%d,0), got (%d,%d)", obj, off, gotOff, gen)
		}
	}
}
