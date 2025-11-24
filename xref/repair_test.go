package xref_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/recovery"
	"github.com/wudi/pdfkit/xref"
)

func TestResolverRepairsCorruptXRef(t *testing.T) {
	// Build a PDF with NO xref table or startxref
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n")

	// No xref, no startxref, just EOF
	buf.WriteString("trailer\n<< /Size 3 /Root 1 0 R >>\n")
	buf.WriteString("%%EOF\n")

	r := &readerAt{data: buf.Bytes()}

	// 1. Default config should fail
	resolver := xref.NewResolver(xref.ResolverConfig{})
	_, err := resolver.Resolve(context.Background(), r)
	if err == nil {
		t.Fatal("expected error on missing startxref, got nil")
	}

	// 2. Recovery config should succeed
	rec := &testRecovery{action: recovery.ActionFix}
	resolver = xref.NewResolver(xref.ResolverConfig{Recovery: rec})
	table, err := resolver.Resolve(context.Background(), r)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	// Verify objects found
	if off, _, ok := table.Lookup(1); !ok || off != int64(off1) {
		t.Errorf("object 1 lookup failed or wrong offset: got %d, want %d, ok=%v", off, off1, ok)
	}
	if off, _, ok := table.Lookup(2); !ok || off != int64(off2) {
		t.Errorf("object 2 lookup failed or wrong offset: got %d, want %d, ok=%v", off, off2, ok)
	}
}

func TestResolverRepairsGarbagePrefix(t *testing.T) {
	// Test case for "1 2 0 obj" where "1" is garbage
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	// Garbage number followed by valid object
	buf.WriteString("999 ")
	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< >>\nendobj\n")

	buf.WriteString("trailer\n<< /Size 2 /Root 1 0 R >>\n%%EOF\n")

	r := &readerAt{data: buf.Bytes()}
	rec := &testRecovery{action: recovery.ActionFix}
	resolver := xref.NewResolver(xref.ResolverConfig{Recovery: rec})

	table, err := resolver.Resolve(context.Background(), r)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	if off, _, ok := table.Lookup(1); !ok || off != int64(off1) {
		t.Errorf("object 1 lookup failed: got %d, want %d", off, off1)
	}
}

type testRecovery struct {
	action recovery.Action
}

func (r *testRecovery) OnError(ctx context.Context, err error, loc recovery.Location) recovery.Action {
	return r.action
}
