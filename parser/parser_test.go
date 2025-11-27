package parser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/security"
)

func TestDocumentParserParsesClassicXRef(t *testing.T) {
	data := buildClassicPDF()
	p := NewDocumentParser(Config{})

	doc, err := p.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if doc.Trailer == nil {
		t.Fatalf("trailer not captured")
	}
	if got := doc.Version; got != "1.7" {
		t.Fatalf("expected version 1.7, got %q", got)
	}
	if len(doc.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(doc.Objects))
	}
	if _, ok := doc.Objects[raw.ObjectRef{Num: 1, Gen: 0}]; !ok {
		t.Fatalf("catalog missing")
	}
}

func TestDocumentParserFollowsPrevChain(t *testing.T) {
	data := buildIncrementalPDF()
	p := NewDocumentParser(Config{})

	doc, err := p.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if _, ok := doc.Objects[raw.ObjectRef{Num: 3, Gen: 0}]; !ok {
		t.Fatalf("incremental object missing")
	}
	obj2, ok := doc.Objects[raw.ObjectRef{Num: 2, Gen: 0}].(*raw.DictObj)
	if !ok {
		t.Fatalf("expected dict for object 2, got %T", doc.Objects[raw.ObjectRef{Num: 2, Gen: 0}])
	}
	countObj, ok := obj2.Get(raw.NameObj{Val: "Count"})
	if !ok {
		t.Fatalf("Count missing on updated pages")
	}
	if num, ok := countObj.(raw.NumberObj); !ok || num.Int() != 2 {
		t.Fatalf("expected Count 2 after update, got %#v", countObj)
	}
	if doc.Trailer == nil {
		t.Fatalf("trailer missing")
	}
	if _, ok := doc.Trailer.Get(raw.NameObj{Val: "Prev"}); !ok {
		t.Fatalf("Prev not propagated on final trailer")
	}
}

func TestDocumentParserPDFA3bFixture(t *testing.T) {
	path := "../testdata/pdfa-3b-with-embedded-file.pdf"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	p := NewDocumentParser(Config{})
	doc, err := p.Parse(context.Background(), f)
	if err != nil {
		t.Fatalf("parse pdfa-3b fixture: %v", err)
	}
	if doc == nil {
		t.Fatalf("expected document")
	}
	foundEmbedded := false
	foundFilespec := false
	for _, obj := range doc.Objects {
		switch v := obj.(type) {
		case *raw.StreamObj:
			if typ, ok := v.Dict.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typ.(raw.NameObj); ok && name.Value() == "EmbeddedFile" {
					foundEmbedded = true
				}
			}
		case *raw.DictObj:
			if typ, ok := v.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typ.(raw.NameObj); ok && name.Value() == "Filespec" {
					if _, ok := v.Get(raw.NameLiteral("AFRelationship")); ok {
						foundFilespec = true
					}
				}
			}
		}
	}
	if !foundEmbedded {
		t.Fatalf("expected at least one embedded file stream in fixture")
	}
	if !foundFilespec {
		t.Fatalf("expected filespec dictionary with AFRelationship")
	}
}

func TestDocumentParserEnforcesLimits(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")
	off1 := buf.Len()
	buf.WriteString("1 0 obj\n(Hello World)\nendobj\n")
	xrefOffset := buf.Len()
	fmt.Fprintf(buf, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \n", off1)
	buf.WriteString("trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOffset)

	cfg := Config{
		Limits: security.Limits{
			MaxStringLength: 5, // "Hello World" is 11 chars
		},
	}
	p := NewDocumentParser(cfg)
	_, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("expected error due to string length limit, got nil")
	}
	if !strings.Contains(err.Error(), "string too long") {
		t.Fatalf("expected 'string too long' error, got: %v", err)
	}
}

func TestDocumentParserDetectsCollection(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-2.0\n")
	off1 := buf.Len()
	// Catalog with Collection
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Collection << /Type /Collection /View /D >> >>\nendobj\n")
	xrefOffset := buf.Len()
	fmt.Fprintf(buf, "xref\n0 2\n0000000000 65535 f \n%010d 00000 n \n", off1)
	buf.WriteString("trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOffset)

	p := NewDocumentParser(Config{})
	doc, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if doc.Collection == nil {
		t.Fatal("expected Collection dictionary, got nil")
	}
	if view, ok := doc.Collection.Get(raw.NameObj{Val: "View"}); !ok {
		t.Error("expected View entry in Collection")
	} else if name, ok := view.(raw.NameObj); !ok || name.Val != "D" {
		t.Errorf("expected View /D, got %v", view)
	}
}

func TestDocumentParserDetectsIndirectCollection(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-2.0\n")
	off1 := buf.Len()
	// Catalog with Indirect Collection
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Collection 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	// Collection Object
	buf.WriteString("2 0 obj\n<< /Type /Collection /View /T >>\nendobj\n")
	xrefOffset := buf.Len()
	fmt.Fprintf(buf, "xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", off1, off2)
	buf.WriteString("trailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOffset)

	p := NewDocumentParser(Config{})
	doc, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if doc.Collection == nil {
		t.Fatal("expected Collection dictionary, got nil")
	}
	if view, ok := doc.Collection.Get(raw.NameObj{Val: "View"}); !ok {
		t.Error("expected View entry in Collection")
	} else if name, ok := view.(raw.NameObj); !ok || name.Val != "T" {
		t.Errorf("expected View /T, got %v", view)
	}
}

func buildClassicPDF() []byte {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n")

	xrefOffset := buf.Len()
	fmt.Fprintf(buf, "xref\n0 3\n")
	fmt.Fprintf(buf, "0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", off1, off2)
	buf.WriteString("trailer\n<< /Size 3 /Root 1 0 R >>\n")
	buf.WriteString("startxref\n")
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOffset)
	return buf.Bytes()
}

func buildIncrementalPDF() []byte {
	buf := &bytes.Buffer{}
	buf.WriteString("%PDF-1.7\n")

	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 1 >>\nendobj\n")

	xref1 := buf.Len()
	fmt.Fprintf(buf, "xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", off1, off2)
	fmt.Fprintf(buf, "trailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref1)

	// Incremental update: replace object 2 and add object 3.
	off2b := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 2 >>\nendobj\n")

	off3 := buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n")

	xref2 := buf.Len()
	fmt.Fprintf(buf, "xref\n2 2\n%010d 00000 n \n%010d 00000 n \n", off2b, off3)
	fmt.Fprintf(buf, "trailer\n<< /Size 4 /Root 1 0 R /Prev %d >>\n", xref1)
	fmt.Fprintf(buf, "startxref\n%d\n%%%%EOF\n", xref2)
	return buf.Bytes()
}
