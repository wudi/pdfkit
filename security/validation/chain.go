package validation

import (
	"crypto/x509"
	"time"
)

// ChainBuilder builds and verifies X.509 certificate chains.
type ChainBuilder interface {
	// BuildChain attempts to build a valid chain from the leaf certificate to a trusted root.
	BuildChain(leaf *x509.Certificate, intermediates []*x509.Certificate, roots *x509.CertPool) ([][]*x509.Certificate, error)
}

type ChainBuilderImpl struct {
	// Configuration options (e.g., key usage checks)
}

func NewChainBuilder() *ChainBuilderImpl {
	return &ChainBuilderImpl{}
}

func (b *ChainBuilderImpl) BuildChain(leaf *x509.Certificate, intermediates []*x509.Certificate, roots *x509.CertPool) ([][]*x509.Certificate, error) {
	opts := x509.VerifyOptions{
		Intermediates: x509.NewCertPool(),
		Roots:         roots,
		CurrentTime:   time.Now(), // Should be configurable for historical validation
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	for _, cert := range intermediates {
		opts.Intermediates.AddCert(cert)
	}

	return leaf.Verify(opts)
}
