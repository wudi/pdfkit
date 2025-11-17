package parser

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"pdflib/ir/raw"
	"pdflib/xref"
)

type mapCache struct {
	m map[raw.ObjectRef]raw.Object
}

func (c *mapCache) Get(ref raw.ObjectRef) (raw.Object, bool) {
	if c.m == nil {
		return nil, false
	}
	v, ok := c.m[ref]
	return v, ok
}

func (c *mapCache) Put(ref raw.ObjectRef, obj raw.Object) {
	if c.m == nil {
		c.m = make(map[raw.ObjectRef]raw.Object)
	}
	c.m[ref] = obj
}

func TestObjectLoaderCachesObjects(t *testing.T) {
	src := buildPDF()

	reader := bytes.NewReader([]byte(src))
	cache := &mapCache{}

	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(context.Background(), reader)
	if err != nil {
		t.Fatalf("resolve xref: %v", err)
	}

	loader, err := (&ObjectLoaderBuilder{
		reader:    reader,
		xrefTable: table,
		cache:     cache,
		maxDepth:  5,
	}).Build()
	if err != nil {
		t.Fatalf("build loader: %v", err)
	}

	// First load should parse and cache.
	if _, err := loader.Load(context.Background(), raw.ObjectRef{Num: 1, Gen: 0}); err != nil {
		t.Fatalf("load object: %v", err)
	}

	if _, ok := cache.Get(raw.ObjectRef{Num: 1, Gen: 0}); !ok {
		t.Fatalf("expected object cached after load")
	}
}

func buildPDF() string {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	offsets := make(map[int]int64)

	offsets[1] = int64(buf.Len())
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

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

	return buf.String()
}
