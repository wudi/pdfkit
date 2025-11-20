package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

func TestCreatePKCS7Signature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
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

	digest := []byte("test digest 12345678901234567890123456789012") // 32 bytes for SHA256
	sig, err := createPKCS7Signature(key, cert, []*x509.Certificate{cert}, digest)
	if err != nil {
		t.Fatalf("createPKCS7Signature failed: %v", err)
	}

	// Basic ASN.1 check
	var ci contentInfo
	_, err = asn1.Unmarshal(sig, &ci)
	if err != nil {
		t.Fatalf("failed to unmarshal content info: %v", err)
	}
	if !ci.ContentType.Equal(oidSignedData) {
		t.Errorf("expected SignedData OID, got %v", ci.ContentType)
	}
}
