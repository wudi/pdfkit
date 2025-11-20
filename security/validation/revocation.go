package validation

import (
	"context"
	"crypto/x509"
)

// RevocationStatus represents the status of a certificate.
type RevocationStatus int

const (
	StatusGood RevocationStatus = iota
	StatusRevoked
	StatusUnknown
)

// RevocationChecker checks the revocation status of a certificate.
type RevocationChecker interface {
	// Check checks if the certificate is revoked.
	Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error)
}

// OCSPChecker implements RevocationChecker using OCSP.
type OCSPChecker struct {
	// HTTP client, cache, etc.
}

func NewOCSPChecker() *OCSPChecker {
	return &OCSPChecker{}
}

func (c *OCSPChecker) Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	// TODO: Implement OCSP request generation and parsing
	// 1. Extract OCSP server URL from cert.OCSPServer
	// 2. Create OCSP request
	// 3. Send request
	// 4. Parse response
	return StatusUnknown, nil
}

// CRLChecker implements RevocationChecker using CRLs.
type CRLChecker struct {
	// Cache, etc.
}

func NewCRLChecker() *CRLChecker {
	return &CRLChecker{}
}

func (c *CRLChecker) Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	// TODO: Implement CRL downloading and parsing
	// 1. Extract CRL Distribution Points from cert.CRLDistributionPoints
	// 2. Download CRL
	// 3. Check if cert.SerialNumber is in CRL
	return StatusUnknown, nil
}
