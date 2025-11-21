package security

// LTVData contains validation data for Long Term Validation (LTV).
type LTVData struct {
	Certs [][]byte // DER encoded certificates
	OCSPs [][]byte // DER encoded OCSP responses
	CRLs  [][]byte // DER encoded CRLs
}
