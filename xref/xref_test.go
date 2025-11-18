package xref_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"pdflib/ir/raw"
	"pdflib/parser"
	"pdflib/xref"
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

	resolver := xref.NewResolver(xref.ResolverConfig{})
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

func buildXRefStreamPDF() []byte {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")

	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n")

	// Object stream with two objects (4 and 5)
	objStreamContent := "<< /Val 7 >> 5"
	header := "4 0 5 " + fmt.Sprintf("%d ", len("<< /Val 7 >>")+1)
	first := len(header)
	decoded := []byte(header + objStreamContent)
	off3 := buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /ObjStm /N 2 /First ")
	buf.WriteString(fmt.Sprintf("%d", first))
	buf.WriteString(" /Length ")
	buf.WriteString(fmt.Sprintf("%d", len(decoded)))
	buf.WriteString(" >>\nstream\n")
	buf.Write(decoded)
	buf.WriteString("\nendstream\nendobj\n")

	xrefOffset := buf.Len()
	entries := buildXRefStreamEntries(map[int]int{
		1: off1,
		2: off2,
		3: off3,
		6: xrefOffset,
	})
	buf.WriteString("6 0 obj\n<< /Type /XRef /Size 7 /W [1 4 1] /Index [0 7] /Length ")
	buf.WriteString(fmt.Sprintf("%d", len(entries)))
	buf.WriteString(" >>\nstream\n")
	buf.Write(entries)
	buf.WriteString("\nendstream\nendobj\n")

	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")
	return buf.Bytes()
}

func buildXRefStreamEntries(offsets map[int]int) []byte {
	entrySize := 6 // w: [1 4 1]
	total := make([]byte, entrySize*7)
	for obj, off := range offsets {
		idx := obj * entrySize
		total[idx] = 1 // type 1
		total[idx+1] = byte(off >> 24)
		total[idx+2] = byte(off >> 16)
		total[idx+3] = byte(off >> 8)
		total[idx+4] = byte(off)
		total[idx+5] = 0
	}
	for i, obj := range []int{4, 5} {
		idx := obj * entrySize
		total[idx] = 2 // type 2
		total[idx+1] = 0
		total[idx+2] = 0
		total[idx+3] = 0
		total[idx+4] = 3 // object stream number
		total[idx+5] = byte(i)
	}
	return total
}

func TestResolverParsesXRefStreamAndObjStm(t *testing.T) {
	data := buildXRefStreamPDF()
	r := &readerAt{data: data}
	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(context.Background(), r)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if table.Type() != "xref-stream" {
		t.Fatalf("expected xref-stream table, got %s", table.Type())
	}
	if os, idx, ok := table.ObjStream(4); !ok || os != 3 || idx != 0 {
		t.Fatalf("expected obj 4 in objstm 3 idx0, got %v %v %v", os, idx, ok)
	}
	off, _, ok := table.Lookup(1)
	if !ok || off == 0 {
		t.Fatalf("object 1 missing offset")
	}

	builder := &parser.ObjectLoaderBuilder{}
	loader, err := builder.WithReader(r).WithXRef(table).Build()
	if err != nil {
		t.Fatalf("build loader: %v", err)
	}
	obj, err := loader.Load(context.Background(), raw.ObjectRef{Num: 4, Gen: 0})
	if err != nil {
		t.Fatalf("load obj 4: %v", err)
	}
	if _, ok := obj.(*raw.DictObj); !ok {
		t.Fatalf("expected dict from objstm, got %T", obj)
	}
	obj5, err := loader.Load(context.Background(), raw.ObjectRef{Num: 5, Gen: 0})
	if err != nil {
		t.Fatalf("load obj 5: %v", err)
	}
	if num, ok := obj5.(raw.NumberObj); !ok || num.Int() != 5 {
		t.Fatalf("expected number 5, got %#v", obj5)
	}
}
