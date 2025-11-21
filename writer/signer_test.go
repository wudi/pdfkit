package writer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"pdflib/builder"
	"pdflib/ir/raw"
	"pdflib/security"
	"pdflib/xref"
)

func TestSign(t *testing.T) {
	// 1. Create a minimal valid PDF using the builder and writer
	b := builder.NewBuilder()
	b.NewPage(612, 792).DrawText("Hello, World!", 100, 700, builder.TextOptions{FontSize: 12}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Failed to build PDF doc: %v", err)
	}

	var buf bytes.Buffer
	w := NewWriter()
	err = w.Write(context.Background(), doc, &buf, Config{Version: PDF17})
	if err != nil {
		t.Fatalf("Failed to write PDF: %v", err)
	}
	pdfContent := buf.Bytes()

	// 2. Create a signer
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a self-signed cert
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test Signer",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	signer := security.NewRSASigner(key, []*x509.Certificate{cert})

	// 3. Sign it
	var signedBuf bytes.Buffer
	err = Sign(context.Background(), bytes.NewReader(pdfContent), int64(len(pdfContent)), &signedBuf, signer, SignConfig{
		Reason:   "Testing",
		Location: "Lab",
		Contact:  "Tester",
	})
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// 4. Verify the output is a valid PDF
	signedData := signedBuf.Bytes()

	// Check if it has the incremental update structure
	// Should have a new object (ID 4), a new xref section, and a new trailer

	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(context.Background(), bytes.NewReader(signedData))
	if err != nil {
		t.Fatalf("Failed to resolve signed PDF: %v", err)
	}

	// Find the signature object. It should be the last object.
	maxObj := 0
	for _, obj := range table.Objects() {
		if obj > maxObj {
			maxObj = obj
		}
	}

	// Check if we have the signature object
	if _, _, ok := table.Lookup(maxObj); !ok {
		t.Errorf("Signature object %d not found in signed PDF", maxObj)
	}

	// Check trailer
	trailer := resolver.Trailer()
	if trailer == nil {
		t.Fatal("No trailer found in signed PDF")
	}

	// Check Size
	if size, ok := trailer.Get(raw.NameLiteral("Size")); ok {
		if n, ok := size.(raw.NumberObj); ok {
			if int(n.Int()) != maxObj+1 {
				t.Errorf("Expected Size %d, got %d", maxObj+1, n.Int())
			}
		}
	}

	// Check Prev
	if _, ok := trailer.Get(raw.NameLiteral("Prev")); !ok {
		t.Error("Trailer missing Prev entry")
	}
}

func TestSign_PAdES(t *testing.T) {
	// 1. Create a minimal valid PDF
	b := builder.NewBuilder()
	b.NewPage(612, 792).DrawText("PAdES Test", 100, 700, builder.TextOptions{FontSize: 12}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Failed to build PDF doc: %v", err)
	}

	var buf bytes.Buffer
	w := NewWriter()
	err = w.Write(context.Background(), doc, &buf, Config{Version: PDF17})
	if err != nil {
		t.Fatalf("Failed to write PDF: %v", err)
	}
	pdfContent := buf.Bytes()

	// 2. Create a signer
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "PAdES Signer",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	signer := security.NewRSASigner(key, []*x509.Certificate{cert})

	// 3. Sign with PAdES enabled
	var signedBuf bytes.Buffer
	err = Sign(context.Background(), bytes.NewReader(pdfContent), int64(len(pdfContent)), &signedBuf, signer, SignConfig{
		Reason: "PAdES Check",
		PAdES:  true,
	})
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// 4. Verify SubFilter
	signedData := signedBuf.Bytes()
	if !bytes.Contains(signedData, []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Error("Expected /SubFilter /ETSI.CAdES.detached in signed PDF")
	}
}
