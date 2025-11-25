package writer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/filters"
	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/parser"
	"github.com/wudi/pdfkit/security"
)

func TestEncryptionRoundTrip(t *testing.T) {
	// 1. Create a document with encryption
	b := builder.NewBuilder()
	b.SetMetadata([]byte("<x:xmpmeta>Secret Metadata</x:xmpmeta>"))
	b.NewPage(595, 842).
		DrawText("Secret Content", 100, 700, builder.TextOptions{FontSize: 24}).
		Finish()

	userPwd := "user"
	ownerPwd := "owner"
	perms := raw.Permissions{Print: true}
	b.SetEncryption(ownerPwd, userPwd, perms, true)

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 2. Write to buffer
	var buf bytes.Buffer
	w := NewWriter()
	// Use deterministic generation to ensure consistent IDs for debugging if needed,
	// though for this test we just care that the ID matches what's used for encryption.
	if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfData := buf.Bytes()
	requireNoPlaintext(t, pdfData, "Secret Content", "Secret Metadata")

	// 3. Parse with User Password
	t.Run("AuthenticateWithUserPassword", func(t *testing.T) {
		cfg := parser.Config{
			Password: userPwd,
		}
		p := parser.NewDocumentParser(cfg)
		parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err != nil {
			t.Fatalf("Parse with user password failed: %v", err)
		}

		if !parsedDoc.Encrypted {
			t.Error("Parsed document should be marked as Encrypted")
		}
		if parsedDoc.MetadataEncrypted != true {
			t.Error("Metadata should be marked as encrypted")
		}
		if parsedDoc.Permissions != perms {
			t.Fatalf("Permissions mismatch: got %+v want %+v", parsedDoc.Permissions, perms)
		}
		assertEncryptDictionary(t, parsedDoc, perms, true)

		decodedDoc := decodeStreams(t, parsedDoc)
		requireContentDecrypted(t, decodedDoc, "Secret Content")
		requireMetadataDecrypted(t, decodedDoc, "Secret Metadata")
	})

	// 4. Parse with Owner Password
	t.Run("AuthenticateWithOwnerPassword", func(t *testing.T) {
		cfg := parser.Config{
			Password: ownerPwd,
		}
		p := parser.NewDocumentParser(cfg)
		parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err != nil {
			t.Fatalf("Parse with owner password failed: %v", err)
		}

		if parsedDoc.Permissions != perms {
			t.Fatalf("Permissions mismatch with owner password: got %+v want %+v", parsedDoc.Permissions, perms)
		}

		decodedDoc := decodeStreams(t, parsedDoc)
		requireContentDecrypted(t, decodedDoc, "Secret Content")
		requireMetadataDecrypted(t, decodedDoc, "Secret Metadata")
	})

	// 5. Parse with Wrong Password
	t.Run("FailWithWrongPassword", func(t *testing.T) {
		cfg := parser.Config{
			Password: "wrong",
		}
		p := parser.NewDocumentParser(cfg)
		_, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err == nil {
			t.Fatal("Parse with wrong password should have failed")
		}
		if !strings.Contains(err.Error(), "invalid password") {
			t.Fatalf("Expected authentication error, got: %v", err)
		}
	})

	t.Run("FailWithEmptyPassword", func(t *testing.T) {
		cfg := parser.Config{
			Password: "",
		}
		p := parser.NewDocumentParser(cfg)
		_, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err == nil {
			t.Fatal("Parse with empty password should have failed")
		}
		if !strings.Contains(err.Error(), "invalid password") {
			t.Fatalf("Expected authentication error for empty password, got: %v", err)
		}
	})
}

func requireNoPlaintext(t *testing.T, pdfData []byte, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if bytes.Contains(pdfData, []byte(secret)) {
			t.Fatalf("found plaintext %q in encrypted file", secret)
		}
	}
}

func assertEncryptDictionary(t *testing.T, doc *raw.Document, expectedPerms raw.Permissions, encryptMetadata bool) {
	t.Helper()
	trailer, ok := doc.Trailer.(*raw.DictObj)
	if !ok {
		t.Fatal("trailer is not a dictionary")
	}

	encObj, ok := trailer.Get(raw.NameLiteral("Encrypt"))
	if !ok {
		t.Fatal("/Encrypt missing from trailer")
	}

	var encDict *raw.DictObj
	switch v := encObj.(type) {
	case *raw.DictObj:
		encDict = v
	case raw.RefObj:
		obj, ok := doc.Objects[v.R]
		if !ok {
			t.Fatalf("encrypt ref %v not found in objects", v.R)
		}
		var dictOk bool
		encDict, dictOk = obj.(*raw.DictObj)
		if !dictOk {
			t.Fatalf("encrypt ref does not point to dictionary, got %T", obj)
		}
	default:
		t.Fatalf("unexpected /Encrypt type %T", encObj)
	}

	if filter, ok := encDict.Get(raw.NameLiteral("Filter")); !ok {
		t.Fatal("/Encrypt missing Filter")
	} else if name, ok := filter.(raw.NameObj); !ok || name.Value() != "Standard" {
		t.Fatalf("unexpected /Filter: %T %v", filter, filter)
	}

	expectNumberField(t, encDict, "V", 1)
	expectNumberField(t, encDict, "R", 2)
	expectNumberField(t, encDict, "Length", 40)

	expectedPermValue := int64(security.PermissionsValue(expectedPerms))
	expectNumberField(t, encDict, "P", expectedPermValue)

	if encryptMetadata {
		if meta, ok := encDict.Get(raw.NameLiteral("EncryptMetadata")); ok {
			b, ok := meta.(raw.BoolObj)
			if !ok || !b.Value() {
				t.Fatalf("EncryptMetadata present but not true: %T %v", meta, meta)
			}
		}
	}
}

func expectNumberField(t *testing.T, dict *raw.DictObj, key string, expected int64) {
	t.Helper()
	value, ok := dict.Get(raw.NameLiteral(key))
	if !ok {
		t.Fatalf("expected field %s missing from /Encrypt", key)
	}
	num, ok := value.(raw.NumberObj)
	if !ok {
		t.Fatalf("field %s is not a number: %T", key, value)
	}
	if num.Int() != expected {
		t.Fatalf("field %s = %d, want %d", key, num.Int(), expected)
	}
}

func decodeStreams(t *testing.T, doc *raw.Document) *decoded.DecodedDocument {
	t.Helper()
	pipeline := filters.NewPipeline(
		[]filters.Decoder{
			filters.NewFlateDecoder(),
			filters.NewLZWDecoder(),
			filters.NewRunLengthDecoder(),
			filters.NewASCII85Decoder(),
			filters.NewASCIIHexDecoder(),
			filters.NewCryptDecoder(),
			filters.NewDCTDecoder(),
			filters.NewJPXDecoder(),
			filters.NewCCITTFaxDecoder(),
			filters.NewJBIG2Decoder(),
		},
		filters.Limits{},
	)
	dec := decoded.NewDecoder(pipeline)
	decodedDoc, err := dec.Decode(context.Background(), doc)
	if err != nil {
		t.Fatalf("decode streams failed: %v", err)
	}
	return decodedDoc
}

func requireContentDecrypted(t *testing.T, doc *decoded.DecodedDocument, expected string) {
	t.Helper()
	for _, stream := range doc.Streams {
		if dict, ok := stream.Dictionary().(*raw.DictObj); ok {
			if tVal, ok := dict.Get(raw.NameLiteral("Type")); ok {
				if name, ok := tVal.(raw.NameObj); ok && name.Value() == "Metadata" {
					continue
				}
			}
		}
		if bytes.Contains(stream.Data(), []byte(expected)) {
			return
		}
	}
	t.Fatalf("content %q not found in any decoded stream", expected)
}

func requireMetadataDecrypted(t *testing.T, doc *decoded.DecodedDocument, expected string) {
	t.Helper()
	for _, stream := range doc.Streams {
		dict, ok := stream.Dictionary().(*raw.DictObj)
		if !ok {
			continue
		}
		tVal, ok := dict.Get(raw.NameLiteral("Type"))
		if !ok {
			continue
		}
		name, ok := tVal.(raw.NameObj)
		if !ok || name.Value() != "Metadata" {
			continue
		}
		if bytes.Contains(stream.Data(), []byte(expected)) {
			return
		}
	}
	t.Fatalf("metadata %q not found in decoded streams", expected)
}
