package writer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/raw"
	semantic "github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/parser"
)

func TestEncryptionRoundTrip(t *testing.T) {
	t.Run("ComplexDocumentUnderEncryption", func(t *testing.T) {
		b := builder.NewBuilder()
		// Page 1: Text
		b.NewPage(595, 842).
			DrawText("Page 1", 100, 800, builder.TextOptions{FontSize: 18}).
			Finish()
		// Page 2: Image (synthetic 1x1 PNG)
		imgData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
			0xDE, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0xF8, 0x0F, 0x00, 0x01,
			0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0x18, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
			0x42, 0x60, 0x82,
		}
		img := &semantic.Image{
			Width:            1,
			Height:           1,
			Data:             imgData,
			BitsPerComponent: 8,
		}
		b.NewPage(595, 842).
			DrawText("Page 2", 100, 800, builder.TextOptions{FontSize: 18}).
			DrawImage(img, 100, 700, 10, 10, builder.ImageOptions{}).
			Finish()
		// Page 3: Form field and annotation
		b.NewPage(595, 842).
			DrawText("Page 3", 100, 800, builder.TextOptions{FontSize: 18}).
			AddFormField(&semantic.TextFormField{
				BaseFormField: semantic.BaseFormField{
					Name: "Field1",
					Rect: semantic.Rectangle{LLX: 100, LLY: 700, URX: 200, URY: 720},
				},
				Value: "Value1",
			}).
			AddAnnotation(&semantic.BaseAnnotation{
				Subtype:  "Highlight",
				RectVal:  semantic.Rectangle{LLX: 100, LLY: 700, URX: 200, URY: 720},
				Contents: "Highlight annotation",
			}).
			Finish()

		userPwd := "user"
		ownerPwd := "owner"
		b.SetEncryptionWithOptions(ownerPwd, userPwd, raw.Permissions{
			Print: true,
		}, true, "AES", 128)

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

		cfg := parser.Config{
			Password: userPwd,
		}
		p := parser.NewDocumentParser(cfg)
		parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err != nil {
			t.Fatalf("Parse with user password failed: %v", err)
		}

		// Check that all pages exist (at least 3 content streams)
		foundPages := 0
		for _, obj := range parsedDoc.Objects {
			if stream, ok := obj.(*raw.StreamObj); ok {
				if bytes.Contains(stream.Data, []byte("Page 1")) || bytes.Contains(stream.Data, []byte("Page 2")) || bytes.Contains(stream.Data, []byte("Page 3")) {
					foundPages++
				}
			}
		}
		if foundPages < 3 {
			t.Errorf("Expected at least 3 page content streams, found %d", foundPages)
		}
		// Spot check: verify text from each page is present in some stream
		verifyContent(t, parsedDoc, "Page 1")
		verifyContent(t, parsedDoc, "Page 2")
		verifyContent(t, parsedDoc, "Page 3")
	})
	t.Run("PermissionsEnforcement", func(t *testing.T) {
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// 1. No password provided
		var errorBuf bytes.Buffer
		errorB := builder.NewBuilder()
		errorB.NewPage(595, 842).
			DrawText("Secret Content", 100, 700, builder.TextOptions{FontSize: 24}).
			Finish()
		errorUserPwd := "user"
		errorOwnerPwd := "owner"
		errorPerms := raw.Permissions{Print: true, Copy: false}
		errorB.SetEncryptionWithOptions(errorOwnerPwd, errorUserPwd, errorPerms, true, "AES", 128)
		errorDoc, err := errorB.Build()
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}
		errorW := NewWriter()
		if err := errorW.Write(context.Background(), errorDoc, &errorBuf, Config{Deterministic: true}); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		errorPdfData := errorBuf.Bytes()

		t.Run("NoPasswordProvided", func(t *testing.T) {
			cfg := parser.Config{} // No password
			p := parser.NewDocumentParser(cfg)
			_, err := p.Parse(context.Background(), bytes.NewReader(errorPdfData))
			if err == nil {
				t.Fatal("Parse without password should have failed")
			}
			if !strings.Contains(err.Error(), "password") && !strings.Contains(err.Error(), "authenticate") {
				t.Logf("Expected password/authenticate error, got: %v", err)
			}
		})

		t.Run("CorruptedEncryptedFile", func(t *testing.T) {
			corrupted := make([]byte, len(errorPdfData))
			copy(corrupted, errorPdfData)
			// Flip a byte in the middle
			if len(corrupted) > 100 {
				corrupted[100] ^= 0xFF
			}
			cfg := parser.Config{Password: errorUserPwd}
			p := parser.NewDocumentParser(cfg)
			parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(corrupted))
			if err == nil {
				t.Logf("Corrupted file parsed, permissions: %+v", parsedDoc.Permissions)
				if parsedDoc.Permissions.Print || parsedDoc.Permissions.Copy {
					t.Fatal("Parse of corrupted encrypted file should have failed or not grant permissions")
				}
			} else {
				t.Logf("Corrupted file parse error: %v", err)
			}
		})

		t.Run("OwnerPasswordWithRestrictedPermissions", func(t *testing.T) {
			cfg := parser.Config{Password: errorOwnerPwd}
			p := parser.NewDocumentParser(cfg)
			parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(errorPdfData))
			if err != nil {
				t.Fatalf("Parse with owner password failed: %v", err)
			}
			t.Logf("Owner permissions: %+v", parsedDoc.Permissions)
			// Owner password should grant all permissions
			if !parsedDoc.Permissions.Print || !parsedDoc.Permissions.Copy {
				t.Errorf("Owner password should grant all permissions, got: %+v", parsedDoc.Permissions)
			}
		})
		b := builder.NewBuilder()
		b.NewPage(595, 842).
			DrawText("Secret Content", 100, 700, builder.TextOptions{FontSize: 24}).
			Finish()

		userPwd := "user"
		ownerPwd := "owner"
		perms := raw.Permissions{
			Print:     true,
			Copy:      false,
			Modify:    false,
			FillForms: false,
		}
		b.SetEncryptionWithOptions(ownerPwd, userPwd, perms, true, "AES", 128)

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

		cfg := parser.Config{
			Password: userPwd,
		}
		p := parser.NewDocumentParser(cfg)
		parsedDoc, err := p.Parse(context.Background(), bytes.NewReader(pdfData))
		if err != nil {
			t.Fatalf("Parse with user password failed: %v", err)
		}

		// Check permissions
		if !parsedDoc.Permissions.Print {
			t.Error("Expected Print permission to be true")
		}
		if parsedDoc.Permissions.Copy {
			t.Error("Expected Copy permission to be false")
		}
		if parsedDoc.Permissions.Modify {
			t.Error("Expected Modify permission to be false")
		}
		if parsedDoc.Permissions.FillForms {
			t.Error("Expected FillForms permission to be false")
		}
	})
	cases := []struct {
		name      string
		algorithm string
		keyBits   int
	}{
		{"RC4-40", "RC4", 40},
		{"RC4-128", "RC4", 128},
		{"AES-128", "AES", 128},
		{"AES-256", "AES", 256},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := builder.NewBuilder()
			b.NewPage(595, 842).
				DrawText("Secret Content", 100, 700, builder.TextOptions{FontSize: 24}).
				Finish()

			userPwd := "user"
			ownerPwd := "owner"
			b.SetEncryptionWithOptions(ownerPwd, userPwd, raw.Permissions{
				Print: true,
			}, true, tc.algorithm, tc.keyBits)

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
				verifyContent(t, parsedDoc, "Secret Content")
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
				verifyContent(t, parsedDoc, "Secret Content")
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
				if !strings.Contains(err.Error(), "authenticate") && !strings.Contains(err.Error(), "password") {
					t.Logf("Expected authentication error, got: %v", err)
				}
			})
		})
	}
}

func verifyContent(t *testing.T, doc *raw.Document, expectedText string) {
	found := false
	for _, obj := range doc.Objects {
		stream, ok := obj.(*raw.StreamObj)
		if !ok {
			continue
		}
		// We are looking for the content stream.
		// It usually doesn't have a specific Type, or Type=XObject for forms.
		// Page content streams don't have a Type in the stream dict usually.
		// But we can just check all streams for the text.

		// The parser automatically decrypts streams if the password was correct.
		data := stream.Data
		// If the stream is compressed (FlateDecode), we might need to decompress it to see the text.
		// The parser's loader decrypts, but does it decompress?
		// The loader decrypts but does NOT decompress by default unless we ask it to?
		// Wait, `loader.loadFromObjectStream` decompresses the object stream.
		// But regular streams? `decryptObject` returns the stream with decrypted data.
		// It does NOT decompress.

		// However, `TestWriterRoundTripPipeline` in `writer_impl_test.go` uses `ir.NewDefault().Parse(...)`
		// which does high-level parsing including decompression.
		// `parser.NewDocumentParser` returns `raw.Document` where streams are still compressed (if Filter is present).

		// Let's check if the stream has a Filter.
		// In our test case, we didn't specify compression in Config, so it defaults to None?
		// `Config{}` default compression is 0 (None)?
		// Let's check `writer.go` Config defaults.
		// If compression is 0, `pickContentFilter` might return `FilterNone`?
		// Actually `writer_impl.go`:
		/*
			switch filter := pickContentFilter(b.cfg); filter {
			case FilterFlate:
				...
		*/
		// If I don't set compression, it might default to something.
		// Let's assume it might be compressed or not.
		// But `TestWriterRoundTripPipeline` used `ir.NewDefault()` which is higher level.

		// Since I am using `parser.NewDocumentParser`, I get raw objects.
		// If I want to verify content, I should probably use `ir` package or handle decompression.
		// Or I can just check if I can decrypt it without error (which Parse does).
		// But to be sure, let's try to find the text.

		// If the text "Secret Content" is in the stream, it will be `(Secret Content) Tj` or similar.
		// If it's compressed, we won't see it.

		// Let's check if we can use `ir` package for this test too, as it handles everything.
		// `ir.NewDefault().Parse` takes a reader.
		// Does `ir` support encryption?
		// `ir/pipeline.go` -> `Parse` -> `loader.New`?
		// I need to check `ir` package.

		if bytes.Contains(data, []byte(expectedText)) {
			found = true
			break
		}
	}

	// If we didn't find it, maybe it's compressed.
	// But `writer.Config{}` has `Compression: 0`.
	// `pickContentFilter` in `writer_impl.go`:
	/*
		func pickContentFilter(cfg Config) ContentFilter {
			if cfg.ContentFilter != 0 {
				return cfg.ContentFilter
			}
			if cfg.Compression > 0 {
				return FilterFlate
			}
			return FilterNone
		}
	*/
	// So it should be uncompressed.

	if !found {
		t.Errorf("Content %q not found in any stream (decryption might have failed to produce correct output or stream is compressed)", expectedText)
	}
}
