package writer

import (
	"bytes"
	"context"
	"crypto/aes"
	"fmt"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/filters"
	"github.com/wudi/pdfkit/ir/decoded"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/parser"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
)

func TestEncryptionRoundTrip(t *testing.T) {
	cases := []struct {
		name            string
		perms           raw.Permissions
		encryptMetadata bool
	}{
		{
			name:            "DefaultPermissions",
			perms:           raw.Permissions{Print: true},
			encryptMetadata: true,
		},
		{
			name: "RestrictedPermissions",
			perms: raw.Permissions{
				Print:             false,
				Copy:              false,
				Modify:            false,
				FillForms:         true,
				ExtractAccessible: false,
			},
			encryptMetadata: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := builder.NewBuilder()
			b.SetMetadata([]byte("<x:xmpmeta>Secret Metadata</x:xmpmeta>"))
			b.NewPage(595, 842).
				DrawText("Secret Content", 100, 700, builder.TextOptions{FontSize: 24}).
				Finish()

			userPwd := "user"
			ownerPwd := "owner"
			b.SetEncryption(ownerPwd, userPwd, tc.perms, tc.encryptMetadata)

			doc, err := b.Build()
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}

			var buf bytes.Buffer
			w := NewWriter()
			if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			pdfData := buf.Bytes()
			requireNoPlaintext(t, pdfData, "Secret Content", "Secret Metadata")

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
				if parsedDoc.MetadataEncrypted != tc.encryptMetadata {
					t.Errorf("Metadata encryption mismatch: got %v want %v", parsedDoc.MetadataEncrypted, tc.encryptMetadata)
				}
				if parsedDoc.Permissions != tc.perms {
					t.Fatalf("Permissions mismatch: got %+v want %+v", parsedDoc.Permissions, tc.perms)
				}
				assertEncryptDictionary(t, parsedDoc, tc.perms, tc.encryptMetadata)

				decodedDoc := decodeStreams(t, parsedDoc)
				requireContentDecrypted(t, decodedDoc, "Secret Content")
				requireMetadataDecrypted(t, decodedDoc, "Secret Metadata")
			})

			t.Run("AuthenticateWithOwnerPassword", func(t *testing.T) {
				cfg := parser.Config{
					Password: ownerPwd,
				}
				p := parser.NewDocumentParser(cfg)
				parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
				if err != nil {
					t.Fatalf("Parse with owner password failed: %v", err)
				}

				if parsedDoc.Permissions != tc.perms {
					t.Fatalf("Permissions mismatch with owner password: got %+v want %+v", parsedDoc.Permissions, tc.perms)
				}

				decodedDoc := decodeStreams(t, parsedDoc)
				requireContentDecrypted(t, decodedDoc, "Secret Content")
				requireMetadataDecrypted(t, decodedDoc, "Secret Metadata")
			})

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
		})
	}
}

func TestEncryptionRoundTrip_Strengths(t *testing.T) {
	cases := []struct {
		name string
		opts security.EncryptionOptions
	}{
		{name: "RC4_40", opts: security.EncryptionOptions{Algorithm: security.EncryptionAlgorithmRC4, KeyLength: 40}},
		{name: "RC4_128", opts: security.EncryptionOptions{Algorithm: security.EncryptionAlgorithmRC4, KeyLength: 128}},
		{name: "AES_128", opts: security.EncryptionOptions{Algorithm: security.EncryptionAlgorithmAES, KeyLength: 128}},
		{name: "AES_256", opts: security.EncryptionOptions{Algorithm: security.EncryptionAlgorithmAES, KeyLength: 256}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := builder.NewBuilder()
			b.NewPage(300, 300).
				DrawText("Encrypted Payload", 50, 50, builder.TextOptions{}).
				Finish()
			perms := raw.Permissions{Print: true, Copy: true}
			userPwd := "user-" + tc.name
			ownerPwd := "owner-" + tc.name
			b.SetEncryptionWithOptions(ownerPwd, userPwd, perms, true, tc.opts)

			doc, err := b.Build()
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if doc.UserPassword == "" {
				t.Fatalf("user password not set")
			}
			normalizedOpts := security.NormalizeEncryptionOptions(doc.EncryptionOptions)
			if normalizedOpts.Algorithm != security.NormalizeEncryptionOptions(tc.opts).Algorithm {
				t.Fatalf("encryption algorithm mismatch on document: %v", normalizedOpts.Algorithm)
			}

			var buf bytes.Buffer
			w := NewWriter()
			if err := w.Write(context.Background(), doc, &buf, Config{Deterministic: true}); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			idPair := fileID(doc, Config{Deterministic: true})
			resolver := xref.NewResolver(parser.Config{}.XRef)
			_, err = resolver.Resolve(context.Background(), bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("resolve xref: %v", err)
			}
			if trailerDict := resolver.Trailer(); trailerDict != nil {
				if idObj, ok := trailerDict.Get(raw.NameLiteral("ID")); ok {
					if arr, ok := idObj.(*raw.ArrayObj); ok && arr.Len() >= 1 {
						if s, ok := arr.Items[0].(raw.StringObj); ok {
							if !bytes.Equal(s.Value(), idPair[0]) {
								t.Fatalf("fileID mismatch: trailer=%x expected=%x", s.Value(), idPair[0])
							}
						}
					}
				}
			}

			noopParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
			noopDoc, err := noopParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("noop parse failed: %v", err)
			}
			if dec := decodeStreams(t, noopDoc); dec != nil {
				for _, s := range dec.Streams {
					if bytes.Contains(s.Data(), []byte("Encrypted Payload")) {
						t.Fatalf("content stream not encrypted")
					}
				}
			}
			trailer := noopDoc.Trailer.(*raw.DictObj)
			encObj, ok := trailer.Get(raw.NameLiteral("Encrypt"))
			if !ok {
				t.Fatalf("encrypt dictionary missing")
			}
			var encDict *raw.DictObj
			switch v := encObj.(type) {
			case *raw.DictObj:
				encDict = v
			case raw.RefObj:
				if obj, ok := noopDoc.Objects[v.R]; ok {
					if d, ok := obj.(*raw.DictObj); ok {
						encDict = d
					}
				}
			}
			if encDict == nil {
				t.Fatalf("encrypt dictionary not resolved")
			}
			if tc.opts.Algorithm == security.EncryptionAlgorithmAES {
				if v, ok := encDict.Get(raw.NameLiteral("V")); ok {
					if num, ok := v.(raw.NumberObj); !ok || num.Int() < 4 {
						t.Fatalf("expected AES V>=4, got %v", v)
					}
				} else {
					t.Fatalf("encrypt dict missing V")
				}
			}
			if testing.Verbose() {
				v, _ := encDict.Get(raw.NameLiteral("V"))
				rVal, _ := encDict.Get(raw.NameLiteral("R"))
				stmF, _ := encDict.Get(raw.NameLiteral("StmF"))
				strF, _ := encDict.Get(raw.NameLiteral("StrF"))
				cf, _ := encDict.Get(raw.NameLiteral("CF"))
				t.Logf("enc dict V=%v R=%v StmF=%T %v StrF=%T %v CF=%T", v, rVal, stmF, stmF, strF, strF, cf)
			}
			if obj, ok := noopDoc.Objects[raw.ObjectRef{Num: 3, Gen: 0}]; ok && testing.Verbose() {
				if d, ok := obj.(*raw.DictObj); ok {
					t.Logf("object 3 keys: %v", d.Keys())
				} else {
					t.Logf("object 3 type: %T", obj)
				}
			}
			handler, err := (&security.HandlerBuilder{}).WithEncryptDict(encDict).WithTrailer(trailer).Build()
			if err != nil {
				t.Fatalf("build handler: %v", err)
			}
			if err := handler.Authenticate(userPwd); err != nil {
				t.Fatalf("manual authenticate failed: %v", err)
			}
			for ref, obj := range noopDoc.Objects {
				if s, ok := obj.(*raw.StreamObj); ok {
					if tc.opts.Algorithm == security.EncryptionAlgorithmAES {
						if len(s.Data) < aes.BlockSize || (len(s.Data)-aes.BlockSize)%aes.BlockSize != 0 {
							t.Fatalf("stream length not AES sized: %d", len(s.Data))
						}
					}
					if _, err := handler.Decrypt(ref.Num, ref.Gen, s.Data, security.DataClassStream); err != nil {
						t.Fatalf("manual decrypt failed for %v: %v", ref, err)
					}
				}
			}

			parserCfg := parser.Config{Password: userPwd}
			p := parser.NewDocumentParser(parserCfg)
			parsed, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			assertEncryptDictionaryWithOptions(t, parsed, perms, true, tc.opts)
			decodedDoc := decodeStreams(t, parsed)
			requireContentDecrypted(t, decodedDoc, "Encrypted Payload")
		})
	}
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
	assertEncryptDictionaryWithOptions(t, doc, expectedPerms, encryptMetadata, security.EncryptionOptions{})
}

func assertEncryptDictionaryWithOptions(t *testing.T, doc *raw.Document, expectedPerms raw.Permissions, encryptMetadata bool, opts security.EncryptionOptions) {
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

	encOpts := security.NormalizeEncryptionOptions(opts)
	switch encOpts.Algorithm {
	case security.EncryptionAlgorithmAES:
		if encOpts.KeyLength >= 256 {
			expectNumberField(t, encDict, "V", 5)
			expectNumberField(t, encDict, "R", 6)
			expectNumberField(t, encDict, "Length", 256)
			assertCryptFilter(t, encDict, "AESV3", 256)
		} else {
			expectNumberField(t, encDict, "V", 4)
			expectNumberField(t, encDict, "R", 4)
			expectNumberField(t, encDict, "Length", int64(encOpts.KeyLength))
			assertCryptFilter(t, encDict, "AESV2", encOpts.KeyLength)
		}
	default:
		expectNumberField(t, encDict, "V", 1)
		expectNumberField(t, encDict, "R", 2)
		expectNumberField(t, encDict, "Length", int64(encOpts.KeyLength))
	}

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

func assertCryptFilter(t *testing.T, enc *raw.DictObj, expectedCFM string, lengthBits int) {
	t.Helper()
	cfVal, ok := enc.Get(raw.NameLiteral("CF"))
	if !ok {
		t.Fatal("CF missing for AES encryption")
	}
	cf, ok := cfVal.(*raw.DictObj)
	if !ok {
		t.Fatalf("CF has unexpected type %T", cfVal)
	}
	stdVal, ok := cf.Get(raw.NameLiteral("StdCF"))
	if !ok {
		t.Fatal("StdCF missing from CF")
	}
	std, ok := stdVal.(*raw.DictObj)
	if !ok {
		t.Fatalf("StdCF has unexpected type %T", stdVal)
	}
	cfm, ok := std.Get(raw.NameLiteral("CFM"))
	if !ok {
		t.Fatal("CFM missing from StdCF")
	}
	if name, ok := cfm.(raw.NameObj); !ok || name.Value() != expectedCFM {
		t.Fatalf("unexpected CFM: %T %v", cfm, cfm)
	}
	expectNumberField(t, std, "Length", int64(lengthBits))
	if stm, ok := enc.Get(raw.NameLiteral("StmF")); ok {
		if name, ok := stm.(raw.NameObj); !ok || name.Value() != "StdCF" {
			t.Fatalf("unexpected StmF: %T %v", stm, stm)
		}
	}
	if str, ok := enc.Get(raw.NameLiteral("StrF")); ok {
		if name, ok := str.(raw.NameObj); !ok || name.Value() != "StdCF" {
			t.Fatalf("unexpected StrF: %T %v", str, str)
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

func TestEncryptionRoundTrip_ComplexStructures(t *testing.T) {
	perms := raw.Permissions{
		Print:     true,
		Modify:    true,
		FillForms: true,
		Copy:      true,
	}
	imageData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	annotationAppearance := []byte("BT (Encrypted annotation) Tj ET")
	formAppearance := []byte("BT (Encrypted form appearance) Tj ET")
	formValue := "Encrypted Field Value"
	meta := "<x:xmpmeta>Encrypted Metadata</x:xmpmeta>"
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 50, URY: 50},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": {
							Subtype:          "Image",
							Width:            1,
							Height:           1,
							BitsPerComponent: 8,
							ColorSpace:       &semantic.DeviceColorSpace{Name: "DeviceRGB"},
							Data:             imageData,
						},
					},
				},
				Contents: []semantic.ContentStream{{
					Operations: []semantic.Operation{
						{Operator: "BT"},
						{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte("Encrypted Page One")}}},
						{Operator: "ET"},
						{Operator: "q"},
						{Operator: "cm", Operands: []semantic.Operand{
							semantic.NumberOperand{Value: 1}, semantic.NumberOperand{Value: 0},
							semantic.NumberOperand{Value: 0}, semantic.NumberOperand{Value: 1},
							semantic.NumberOperand{Value: 0}, semantic.NumberOperand{Value: 0},
						}},
						{Operator: "Do", Operands: []semantic.Operand{semantic.NameOperand{Value: "Im1"}}},
						{Operator: "Q"},
					},
				}},
			},
			{
				MediaBox: semantic.Rectangle{URX: 50, URY: 50},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT (Encrypted Page Two) Tj ET")}},
				Annotations: []semantic.Annotation{
					&semantic.GenericAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype:         "Text",
							RectVal:         semantic.Rectangle{LLX: 5, LLY: 5, URX: 15, URY: 15},
							Contents:        "Encrypted Annotation",
							Appearance:      annotationAppearance,
							AppearanceState: "N",
						},
					},
				},
			},
		},
		AcroForm: &semantic.AcroForm{
			Fields: []semantic.FormField{
				&semantic.TextFormField{
					BaseFormField: semantic.BaseFormField{
						Name:              "Field1",
						PageIndex:         1,
						Rect:              semantic.Rectangle{LLX: 10, LLY: 10, URX: 30, URY: 20},
						Appearance:        formAppearance,
						AppearanceState:   "N",
						DefaultAppearance: "/F1 10 Tf 0 g",
					},
					Value: formValue,
				},
			},
			DefaultResources: &semantic.Resources{
				Fonts: map[string]*semantic.Font{
					"F1": {BaseFont: "Helvetica"},
				},
			},
			NeedAppearances: true,
		},
		Metadata:          &semantic.XMPMetadata{Raw: []byte(meta)},
		Encrypted:         true,
		UserPassword:      "user",
		OwnerPassword:     "owner",
		Permissions:       perms,
		MetadataEncrypted: true,
	}
	w := NewWriter()
	for _, cfg := range []Config{{Deterministic: true}, {Deterministic: true, XRefStreams: true}} {
		cfg := cfg
		t.Run(fmt.Sprintf("XRefStreams_%v", cfg.XRefStreams), func(t *testing.T) {
			var buf bytes.Buffer
			if err := w.Write(context.Background(), doc, &buf, cfg); err != nil {
				t.Fatalf("write encrypted complex doc (streams=%v): %v", cfg.XRefStreams, err)
			}
			parserCfg := parser.Config{Password: doc.UserPassword}
			p := parser.NewDocumentParser(parserCfg)
			parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("parse encrypted complex output: %v", err)
			}
			if parsedDoc.Permissions != perms {
				t.Fatalf("permissions mismatch: got %+v want %+v", parsedDoc.Permissions, perms)
			}
			if parsedDoc.MetadataEncrypted != doc.MetadataEncrypted {
				t.Fatalf("metadata encryption flag mismatch: got %v want %v", parsedDoc.MetadataEncrypted, doc.MetadataEncrypted)
			}
			requirePageCount(t, parsedDoc, 2)
			assertEncryptDictionary(t, parsedDoc, perms, doc.MetadataEncrypted)

			decodedDoc := decodeStreams(t, parsedDoc)
			requireStreamContains(t, decodedDoc, []byte("Encrypted Page One"))
			requireStreamContains(t, decodedDoc, []byte("Encrypted Page Two"))
			requireStreamContains(t, decodedDoc, annotationAppearance)
			requireStreamContains(t, decodedDoc, formAppearance)
			requireStreamContains(t, decodedDoc, imageData)
			requireMetadataDecrypted(t, decodedDoc, "Encrypted Metadata")
			assertFieldValue(t, parsedDoc, "Field1", formValue)
		})
	}
}

func TestEncryptionRoundTrip_EmbeddedFilesAndOutlines(t *testing.T) {
	perms := raw.Permissions{
		Print:  true,
		Modify: true,
	}
	embeddedData := []byte("Encrypted attachment payload")
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 40, URY: 40},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT (Encrypted attachments) Tj ET")}},
			},
		},
		EmbeddedFiles: []semantic.EmbeddedFile{
			{
				Name:        "secret.txt",
				Subtype:     "text/plain",
				Description: "Encrypted payload",
				Data:        embeddedData,
			},
		},
		Outlines: []semantic.OutlineItem{
			{Title: "Encrypted Outline 1", PageIndex: 0},
			{Title: "Encrypted Outline 2", PageIndex: 0},
		},
		Encrypted:         true,
		UserPassword:      "user",
		OwnerPassword:     "owner",
		Permissions:       perms,
		MetadataEncrypted: false,
	}
	w := NewWriter()
	for _, cfg := range []Config{{Deterministic: true}, {Deterministic: true, XRefStreams: true}} {
		cfg := cfg
		t.Run(fmt.Sprintf("XRefStreams_%v", cfg.XRefStreams), func(t *testing.T) {
			var buf bytes.Buffer
			if err := w.Write(context.Background(), doc, &buf, cfg); err != nil {
				t.Fatalf("write encrypted attachments (streams=%v): %v", cfg.XRefStreams, err)
			}
			p := parser.NewDocumentParser(parser.Config{Password: doc.UserPassword})
			parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("parse encrypted attachments: %v", err)
			}
			assertEncryptDictionary(t, parsedDoc, perms, doc.MetadataEncrypted)
			requirePageCount(t, parsedDoc, 1)
			requireEmbeddedFilesNameTree(t, parsedDoc)
			pageRef := firstPageRef(t, parsedDoc)
			assertOutlineDestinations(t, parsedDoc, pageRef, []string{"Encrypted Outline 1", "Encrypted Outline 2"})
			decodedDoc := decodeStreams(t, parsedDoc)
			requireStreamContains(t, decodedDoc, embeddedData)
		})
	}
}

func requireStreamContains(t *testing.T, doc *decoded.DecodedDocument, needle []byte) {
	t.Helper()
	for _, stream := range doc.Streams {
		if bytes.Contains(stream.Data(), needle) {
			return
		}
	}
	t.Fatalf("decoded streams missing %q", needle)
}

func requirePageCount(t *testing.T, doc *raw.Document, expected int) {
	t.Helper()
	count := 0
	for _, obj := range doc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := dict.Get(raw.NameLiteral("Type")); ok {
			if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
				count++
			}
		}
	}
	if count != expected {
		t.Fatalf("unexpected page count: got %d want %d", count, expected)
	}
}

func assertFieldValue(t *testing.T, doc *raw.Document, fieldName, expected string) {
	t.Helper()
	for _, obj := range doc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		nameObj, ok := dict.Get(raw.NameLiteral("T"))
		if !ok {
			continue
		}
		nameStr, ok := nameObj.(raw.StringObj)
		if !ok || string(nameStr.Value()) != fieldName {
			continue
		}
		valObj, ok := dict.Get(raw.NameLiteral("V"))
		if !ok {
			t.Fatalf("field %s missing value", fieldName)
		}
		valStr, ok := valObj.(raw.StringObj)
		if !ok {
			t.Fatalf("field %s value not string: %T", fieldName, valObj)
		}
		if string(valStr.Value()) != expected {
			t.Fatalf("field %s value mismatch: got %q want %q", fieldName, valStr.Value(), expected)
		}
		return
	}
	t.Fatalf("field %s not found", fieldName)
}

func requireEmbeddedFilesNameTree(t *testing.T, doc *raw.Document) {
	t.Helper()
	for _, obj := range doc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if names, ok := dict.Get(raw.NameLiteral("Names")); ok {
			if namesDict, ok := names.(*raw.DictObj); ok {
				if _, ok := namesDict.Get(raw.NameLiteral("EmbeddedFiles")); ok {
					return
				}
			}
		}
	}
	t.Fatalf("embedded files names tree missing")
}

func firstPageRef(t *testing.T, doc *raw.Document) raw.ObjectRef {
	t.Helper()
	for ref, obj := range doc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := dict.Get(raw.NameLiteral("Type")); ok {
			if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
				return ref
			}
		}
	}
	t.Fatalf("page reference not found")
	return raw.ObjectRef{}
}

func assertOutlineDestinations(t *testing.T, doc *raw.Document, pageRef raw.ObjectRef, titles []string) {
	t.Helper()
	expected := make(map[string]bool, len(titles))
	for _, title := range titles {
		expected[title] = false
	}
	for _, obj := range doc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		titleObj, ok := dict.Get(raw.NameLiteral("Title"))
		if !ok {
			continue
		}
		titleStr, ok := titleObj.(raw.StringObj)
		if !ok {
			continue
		}
		title := string(titleStr.Value())
		want, ok := expected[title]
		if !ok || want {
			continue
		}
		dest, ok := dict.Get(raw.NameLiteral("Dest"))
		if !ok {
			t.Fatalf("outline %q missing Dest", title)
		}
		arr, ok := dest.(*raw.ArrayObj)
		if !ok || arr.Len() == 0 {
			t.Fatalf("outline %q dest malformed: %#v", title, dest)
		}
		refObj, ok := arr.Items[0].(raw.RefObj)
		if !ok || refObj.Ref() != pageRef {
			t.Fatalf("outline %q dest does not point to page ref %v: %#v", title, pageRef, arr.Items[0])
		}
		expected[title] = true
	}
	for title, found := range expected {
		if !found {
			t.Fatalf("outline %q not found with correct destination", title)
		}
	}
}
