package parser

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/recovery"
	"github.com/wudi/pdfkit/scanner"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
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

type cryptStub struct {
	calls     int
	filter    string
	encrypted bool
}

func (c *cryptStub) IsEncrypted() bool                  { return c.encrypted }
func (c *cryptStub) Authenticate(password string) error { return nil }
func (c *cryptStub) DecryptWithFilter(objNum, gen int, data []byte, class security.DataClass, cryptFilter string) ([]byte, error) {
	c.calls++
	c.filter = cryptFilter
	return data, nil
}
func (c *cryptStub) Decrypt(objNum, gen int, data []byte, class security.DataClass) ([]byte, error) {
	c.calls++
	return data, nil
}
func (c *cryptStub) EncryptWithFilter(objNum, gen int, data []byte, class security.DataClass, cryptFilter string) ([]byte, error) {
	return data, nil
}
func (c *cryptStub) Encrypt(objNum, gen int, data []byte, class security.DataClass) ([]byte, error) {
	return data, nil
}
func (c *cryptStub) Permissions() security.Permissions { return security.Permissions{} }
func (c *cryptStub) EncryptMetadata() bool             { return true }

func TestDecryptStreamSkipsIdentityCryptFilter(t *testing.T) {
	stub := &cryptStub{encrypted: true}
	dict := raw.Dict()
	dict.Set(raw.NameLiteral("Filter"), raw.NameObj{Val: "Crypt"})
	dp := raw.Dict()
	dp.Set(raw.NameLiteral("Name"), raw.NameObj{Val: "Identity"})
	dict.Set(raw.NameLiteral("DecodeParms"), dp)
	stream := raw.NewStream(dict, []byte("plaintext"))

	loader := &objectLoader{security: stub}
	obj, err := loader.decryptObject(raw.ObjectRef{Num: 5, Gen: 0}, stream)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	out, ok := obj.(*raw.StreamObj)
	if !ok {
		t.Fatalf("expected stream, got %T", obj)
	}
	if string(out.RawData()) != "plaintext" {
		t.Fatalf("unexpected data: %q", out.RawData())
	}
	if stub.calls != 0 {
		t.Fatalf("expected decrypt not called for identity filter, got %d", stub.calls)
	}
}

func TestDecryptStreamUsesCryptFilterName(t *testing.T) {
	stub := &cryptStub{encrypted: true}
	dict := raw.Dict()
	dict.Set(raw.NameLiteral("Filter"), raw.NameObj{Val: "Crypt"})
	dp := raw.Dict()
	dp.Set(raw.NameLiteral("Name"), raw.NameObj{Val: "CF1"})
	dict.Set(raw.NameLiteral("DecodeParms"), dp)
	stream := raw.NewStream(dict, []byte("ciphertext"))

	loader := &objectLoader{security: stub}
	obj, err := loader.decryptObject(raw.ObjectRef{Num: 9, Gen: 2}, stream)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if _, ok := obj.(*raw.StreamObj); !ok {
		t.Fatalf("expected stream, got %T", obj)
	}
	if stub.calls != 1 {
		t.Fatalf("expected decrypt called once, got %d", stub.calls)
	}
	if stub.filter != "CF1" {
		t.Fatalf("crypt filter name propagated, got %q", stub.filter)
	}
}

func TestTokenReaderLengthHintWithNestedDecodeParms(t *testing.T) {
	data := []byte("<< /Type /XObject /DecodeParms [ << /Predictor 15 >> ] /Length 6 >>\nstream\nABCDEF\nendstream\n")
	sc := scanner.New(bytes.NewReader(data), scanner.Config{})
	tr := newTokenReader(NewStreamAware(sc))

	obj, err := parseObject(tr, recovery.NewStrictStrategy(), 0, 0)
	if err != nil {
		t.Fatalf("parse dict: %v", err)
	}
	if _, ok := obj.(*raw.DictObj); !ok {
		t.Fatalf("expected dict, got %T", obj)
	}
	streamTok, err := tr.next()
	if err != nil {
		t.Fatalf("read stream token: %v", err)
	}
	if streamTok.Type != scanner.TokenStream {
		t.Fatalf("expected stream token, got %v", streamTok.Type)
	}
	if len(streamTok.Bytes) != 6 {
		t.Fatalf("expected 6-byte stream, got %d", len(streamTok.Bytes))
	}
}
