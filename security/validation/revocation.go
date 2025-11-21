package validation

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ocsp"
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
	Client *http.Client
}

func NewOCSPChecker() *OCSPChecker {
	return &OCSPChecker{
		Client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *OCSPChecker) Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	if len(cert.OCSPServer) == 0 {
		return StatusUnknown, nil
	}

	// Try all OCSP servers
	var lastErr error
	for _, serverURL := range cert.OCSPServer {
		status, err := c.checkOne(ctx, serverURL, cert, issuer)
		if err == nil && status != StatusUnknown {
			return status, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	return StatusUnknown, lastErr
}

func (c *OCSPChecker) checkOne(ctx context.Context, serverURL string, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	req, err := ocsp.CreateRequest(cert, issuer, &ocsp.RequestOptions{
		Hash: crypto.SHA1, // SHA1 is standard for OCSP requests
	})
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to create OCSP request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", serverURL, bytes.NewReader(req))
	if err != nil {
		return StatusUnknown, err
	}
	httpRequest.Header.Set("Content-Type", "application/ocsp-request")
	httpRequest.Header.Set("Accept", "application/ocsp-response")

	resp, err := c.Client.Do(httpRequest)
	if err != nil {
		return StatusUnknown, fmt.Errorf("OCSP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to read OCSP response: %w", err)
	}

	ocspResp, err := ocsp.ParseResponse(body, issuer)
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to parse OCSP response: %w", err)
	}

	switch ocspResp.Status {
	case ocsp.Good:
		return StatusGood, nil
	case ocsp.Revoked:
		return StatusRevoked, nil
	default:
		return StatusUnknown, nil
	}
}

// CRLChecker implements RevocationChecker using CRLs.
type CRLChecker struct {
	Client *http.Client
}

func NewCRLChecker() *CRLChecker {
	return &CRLChecker{
		Client: &http.Client{Timeout: 30 * time.Second}, // CRLs can be large
	}
}

func (c *CRLChecker) Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	if len(cert.CRLDistributionPoints) == 0 {
		return StatusUnknown, nil
	}

	var lastErr error
	for _, crlURL := range cert.CRLDistributionPoints {
		status, err := c.checkOne(ctx, crlURL, cert, issuer)
		if err == nil && status != StatusUnknown {
			return status, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	return StatusUnknown, lastErr
}

func (c *CRLChecker) checkOne(ctx context.Context, crlURL string, cert *x509.Certificate, issuer *x509.Certificate) (RevocationStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", crlURL, nil)
	if err != nil {
		return StatusUnknown, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return StatusUnknown, fmt.Errorf("CRL request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to read CRL: %w", err)
	}

	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return StatusUnknown, fmt.Errorf("failed to parse CRL: %w", err)
	}

	// Verify CRL signature
	if err := crl.CheckSignatureFrom(issuer); err != nil {
		return StatusUnknown, fmt.Errorf("CRL signature invalid: %w", err)
	}

	for _, revoked := range crl.RevokedCertificateEntries {
		if revoked.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return StatusRevoked, nil
		}
	}

	return StatusGood, nil
}
