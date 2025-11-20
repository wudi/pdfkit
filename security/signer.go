package security

import (
	"crypto/rsa"
	"crypto/x509"
	"errors"
)

// Signer represents an entity capable of signing data.
type Signer interface {
	// Sign signs the given data and returns the signature bytes (PKCS#7/CMS).
	Sign(data []byte) ([]byte, error)

	// Certificate returns the signer's certificate chain.
	Certificate() []*x509.Certificate
}

// RSASigner implements Signer using an RSA private key.
type RSASigner struct {
	priv  *rsa.PrivateKey
	chain []*x509.Certificate
}

// NewRSASigner creates a new RSA signer.
func NewRSASigner(priv *rsa.PrivateKey, chain []*x509.Certificate) *RSASigner {
	return &RSASigner{
		priv:  priv,
		chain: chain,
	}
}

func (s *RSASigner) Sign(data []byte) ([]byte, error) {
	// data is the digest of the PDF content (calculated by the caller).

	if len(s.chain) == 0 {
		return nil, errors.New("signer certificate chain is empty")
	}
	cert := s.chain[0]

	return createPKCS7Signature(s.priv, cert, s.chain, data)
}

func (s *RSASigner) Certificate() []*x509.Certificate {
	return s.chain
}

// MockSigner for testing without keys
type MockSigner struct{}

func (m *MockSigner) Sign(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	return []byte("mock-signature"), nil
}

func (m *MockSigner) Certificate() []*x509.Certificate {
	return nil
}
