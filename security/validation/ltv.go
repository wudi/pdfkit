package validation

import (
	"context"
	"pdflib/ir/semantic"
)

// LTVManager manages Long Term Validation (LTV) information in the PDF.
// It handles the Document Security Store (DSS) dictionary.
type LTVManager interface {
	// AddValidationInfo adds validation information (certs, OCSP, CRLs) to the DSS.
	AddValidationInfo(ctx context.Context, doc *semantic.Document, vri *ValidationRelatedInfo) error
}

// ValidationRelatedInfo contains the validation data to be added.
type ValidationRelatedInfo struct {
	Certs [][]byte // DER encoded certificates
	OCSPs [][]byte // DER encoded OCSP responses
	CRLs  [][]byte // DER encoded CRLs
}

type LTVManagerImpl struct{}

func NewLTVManager() *LTVManagerImpl {
	return &LTVManagerImpl{}
}

func (m *LTVManagerImpl) AddValidationInfo(ctx context.Context, doc *semantic.Document, vri *ValidationRelatedInfo) error {
	// 1. Find or create the DSS dictionary in the Catalog
	// Note: semantic.Document needs to expose DSS or allow access to Catalog dictionary.
	// Currently semantic.Catalog is empty struct in semantic.go, so we might need to extend it
	// or use raw access.

	// 2. Add Certs to /Certs array in DSS

	// 3. Add OCSPs to /OCSPs array in DSS

	// 4. Add CRLs to /CRLs array in DSS

	// 5. Create VRI dictionary for specific signature (optional but recommended)

	return nil
}
