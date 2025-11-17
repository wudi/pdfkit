package raw

import (
	"bytes"
	"context"
	"testing"
)

// readerAt returns a ReaderAt for in-memory PDF text.
func readerAt(s string) *bytes.Reader { return bytes.NewReader([]byte(s)) }

func TestParserParsesObjectsAndStream(t *testing.T) {
	src := "" +
		"1 0 obj\n" +
		"<< /Type /Catalog >>\n" +
		"endobj\n" +
		"2 0 obj\n" +
		"<< /Length 5 >>\n" +
		"stream\n" +
		"hello\n" +
		"endstream\n" +
		"endobj\n"

	parser := NewParser(ParserConfig{})
	doc, err := parser.Parse(context.Background(), readerAt(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(doc.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(doc.Objects))
	}

	obj1, ok := doc.Objects[ObjectRef{Num: 1, Gen: 0}]
	if !ok {
		t.Fatalf("missing catalog object")
	}
	if obj1.Type() != "dict" {
		t.Fatalf("expected dict for obj 1, got %s", obj1.Type())
	}

	obj2, ok := doc.Objects[ObjectRef{Num: 2, Gen: 0}]
	if !ok {
		t.Fatalf("missing stream object")
	}
	stream, ok := obj2.(*StreamObj)
	if !ok {
		t.Fatalf("expected stream object, got %T", obj2)
	}
	if got := string(stream.Data); got != "hello" {
		t.Fatalf("unexpected stream data: %q", got)
	}
}
