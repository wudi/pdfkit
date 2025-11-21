package security

import (
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
)

var (
	oidAttributeSigningCertificateV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
)

// SigningCertificateV2 identifies the signing certificate.
// RFC 5035
type signingCertificateV2 struct {
	Certs []essCertIDv2
}

type essCertIDv2 struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	CertHash      []byte
	IssuerSerial  *issuerSerial `asn1:"optional"`
}

type issuerSerial struct {
	Issuer       asn1.RawValue // GeneralNames
	SerialNumber *big.Int
}

// createSigningCertificateV2Attribute creates the id-aa-signingCertificateV2 attribute.
func createSigningCertificateV2Attribute(cert *x509.Certificate) (attribute, error) {
	// 1. Calculate hash of the certificate
	h := sha256.New()
	h.Write(cert.Raw)
	certHash := h.Sum(nil)

	// 2. Create ESSCertIDv2
	essCert := essCertIDv2{
		HashAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  oidDigestAlgorithmSHA256,
			Parameters: asn1.RawValue{Tag: 5}, // asn1.TagNull
		},
		CertHash: certHash,
		// IssuerSerial is optional and often omitted if the signer info already contains it.
		// However, for strict compliance, it's good to have.
		// But constructing GeneralNames from x509.Certificate is painful in Go's encoding/asn1.
		// We'll omit it for now as it's optional and the cert hash is the critical binding.
	}

	// 3. Create SigningCertificateV2
	scv2 := signingCertificateV2{
		Certs: []essCertIDv2{essCert},
	}

	scv2Bytes, err := asn1.Marshal(scv2)
	if err != nil {
		return attribute{}, fmt.Errorf("marshal SigningCertificateV2: %w", err)
	}

	return attribute{
		Type: oidAttributeSigningCertificateV2,
		Value: asn1.RawValue{
			Tag: 4, // asn1.TagOctetString? No, it's a SET of AttributeValue.
			// AttributeValue is ANY.
			// For this attribute, it's SigningCertificateV2.
			// Wait, `attribute` struct in pkcs7.go has `Value asn1.RawValue 'asn1:"set"'`
			// So we need to provide the bytes of the SigningCertificateV2 structure.
			// But `asn1.Marshal` above gave us the SEQUENCE (SigningCertificateV2).
			// We need to wrap it in a SET if we were marshaling the whole attribute manually.
			// But `attribute` struct handles the SET tag.
			// We just need the content of the SET, which is the SigningCertificateV2 SEQUENCE.
			// However, `Value` is `asn1.RawValue`.
			// If we put the bytes directly, `encoding/asn1` might try to re-encode them if we don't set the tag correctly.
			// Actually, `Value` is `asn1.RawValue` with `set` tag in the struct field.
			// This means `Value` *is* the SET.
			// No, `Value` is an element *inside* the SET.
			// The struct definition is:
			// type attribute struct {
			// 	Type  asn1.ObjectIdentifier
			// 	Value asn1.RawValue `asn1:"set"`
			// }
			// This means `Value` will be marshaled as the elements of the SET.
			// Since we have only one value (the SigningCertificateV2 sequence),
			// we should put that sequence in `Value`.
			// `asn1.RawValue` should preserve the tag of the sequence.

			FullBytes: scv2Bytes,
		},
	}, nil
}
