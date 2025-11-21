package validation

import (
	"context"
	"crypto/x509"
	"testing"
)

func TestOCSPChecker_New(t *testing.T) {
	checker := NewOCSPChecker()
	if checker == nil {
		t.Fatal("NewOCSPChecker returned nil")
	}
	if checker.Client == nil {
		t.Fatal("NewOCSPChecker Client is nil")
	}
}

func TestCRLChecker_New(t *testing.T) {
	checker := NewCRLChecker()
	if checker == nil {
		t.Fatal("NewCRLChecker returned nil")
	}
	if checker.Client == nil {
		t.Fatal("NewCRLChecker Client is nil")
	}
}

func TestOCSPChecker_Check_NoServer(t *testing.T) {
	checker := NewOCSPChecker()
	cert := &x509.Certificate{}
	issuer := &x509.Certificate{}

	status, err := checker.Check(context.Background(), cert, issuer)
	if err != nil {
		t.Errorf("Expected no error for empty OCSP server, got %v", err)
	}
	if status != StatusUnknown {
		t.Errorf("Expected StatusUnknown, got %v", status)
	}
}

func TestCRLChecker_Check_NoDP(t *testing.T) {
	checker := NewCRLChecker()
	cert := &x509.Certificate{}
	issuer := &x509.Certificate{}

	status, err := checker.Check(context.Background(), cert, issuer)
	if err != nil {
		t.Errorf("Expected no error for empty CRL DP, got %v", err)
	}
	if status != StatusUnknown {
		t.Errorf("Expected StatusUnknown, got %v", status)
	}
}
