package security

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"time"
)

var (
	oidData                   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData             = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidDigestAlgorithmSHA256  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidEncryptionAlgorithmRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidAttributeContentType   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidAttributeMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidAttributeSigningTime   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
)

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,tag:0"`
}

type signedData struct {
	Version          int
	DigestAlgorithms []pkix.AlgorithmIdentifier
	EncapContentInfo encapsulatedContentInfo
	Certificates     []asn1.RawValue `asn1:"optional,tag:0,set"`
	CRLs             []asn1.RawValue `asn1:"optional,tag:1,set"`
	SignerInfos      []signerInfo
}

type encapsulatedContentInfo struct {
	EContentType asn1.ObjectIdentifier
	EContent     asn1.RawValue `asn1:"optional,explicit,tag:0"`
}

type signerInfo struct {
	Version                   int
	IssuerAndSerialNumber     issuerAndSerialNumber
	DigestAlgorithm           pkix.AlgorithmIdentifier
	AuthenticatedAttributes   []attribute `asn1:"optional,tag:0"`
	DigestEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedDigest           []byte
	UnauthenticatedAttributes []attribute `asn1:"optional,tag:1"`
}

type issuerAndSerialNumber struct {
	Issuer       asn1.RawValue
	SerialNumber *big.Int
}

type attribute struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

// createPKCS7Signature creates a detached PKCS#7 signature for the given content digest.
func createPKCS7Signature(priv *rsa.PrivateKey, cert *x509.Certificate, chain []*x509.Certificate, contentDigest []byte, extraAttrs []attribute) ([]byte, error) {
	if cert == nil {
		return nil, fmt.Errorf("signer certificate is required")
	}

	// 1. Prepare Authenticated Attributes
	// ContentType: pkcs7-data
	// MessageDigest: contentDigest
	// SigningTime: now

	attrs := []attribute{
		{
			Type: oidAttributeContentType,
			Value: asn1.RawValue{
				Tag:   6, // asn1.TagObjectIdentifier
				Bytes: marshalOID(oidData),
			},
		},
		{
			Type: oidAttributeSigningTime,
			Value: asn1.RawValue{
				Tag:   23, // asn1.TagUTCTime
				Bytes: []byte(time.Now().UTC().Format("060102150405Z")),
			},
		},
		{
			Type: oidAttributeMessageDigest,
			Value: asn1.RawValue{
				Tag:   4, // asn1.TagOctetString
				Bytes: contentDigest,
			},
		},
	}

	// Add extra attributes (e.g. PAdES signing-certificate-v2)
	attrs = append(attrs, extraAttrs...)

	// Marshal attributes to bytes for signing
	// Note: The tag for AuthenticatedAttributes is [0] IMPLICIT SET OF Attribute
	// But when signing, we sign the SET OF Attribute (tag 17).
	// We need to marshal the slice of attributes as a SET.

	// However, encoding/asn1 doesn't easily let us marshal a slice as a SET OF without a struct.
	// Let's construct the raw bytes for the SET OF.

	// Actually, `signerInfo` defines `AuthenticatedAttributes` as `[]attribute` with `tag:0`.
	// `encoding/asn1` will marshal this as `[0] IMPLICIT SET OF Attribute`.
	// But for signing, we need the DER encoding of `SET OF Attribute` (tag 17).
	// We can achieve this by marshaling the attributes with a different struct or manually changing the tag.

	attrBytes, err := marshalAttributes(attrs)
	if err != nil {
		return nil, fmt.Errorf("marshal attributes: %w", err)
	}

	// Hash the attributes
	h := sha256.New()
	h.Write(attrBytes)
	attrDigest := h.Sum(nil)

	// Sign the attributes hash
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, attrDigest)
	if err != nil {
		return nil, fmt.Errorf("sign attributes: %w", err)
	}

	// 2. Construct SignerInfo
	issuerBytes := cert.RawIssuer
	// We need to extract the raw bytes of the issuer from the certificate because
	// re-marshaling cert.Issuer might produce different bytes if order changes (though it shouldn't for DNs).
	// cert.RawIssuer is exactly what we want.

	si := signerInfo{
		Version: 1,
		IssuerAndSerialNumber: issuerAndSerialNumber{
			Issuer:       asn1.RawValue{FullBytes: issuerBytes},
			SerialNumber: cert.SerialNumber,
		},
		DigestAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  oidDigestAlgorithmSHA256,
			Parameters: asn1.RawValue{Tag: 5}, // asn1.TagNull
		},
		AuthenticatedAttributes: attrs,
		DigestEncryptionAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  oidEncryptionAlgorithmRSA,
			Parameters: asn1.RawValue{Tag: 5}, // asn1.TagNull
		},
		EncryptedDigest: signature,
	}

	// 3. Construct SignedData
	var certs []asn1.RawValue
	// Add signer cert
	certs = append(certs, asn1.RawValue{FullBytes: cert.Raw})
	// Add chain
	for _, c := range chain {
		if !c.Equal(cert) {
			certs = append(certs, asn1.RawValue{FullBytes: c.Raw})
		}
	}

	sd := signedData{
		Version: 1,
		DigestAlgorithms: []pkix.AlgorithmIdentifier{
			{
				Algorithm:  oidDigestAlgorithmSHA256,
				Parameters: asn1.RawValue{Tag: 5}, // asn1.TagNull
			},
		},
		EncapContentInfo: encapsulatedContentInfo{
			EContentType: oidData,
			// EContent is empty for detached signature
		},
		Certificates: certs,
		SignerInfos:  []signerInfo{si},
	}

	sdBytes, err := asn1.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("marshal signed data: %w", err)
	}

	// 4. Wrap in ContentInfo
	ci := contentInfo{
		ContentType: oidSignedData,
		Content: asn1.RawValue{
			Class:      asn1.ClassContextSpecific,
			Tag:        0,
			IsCompound: true,
			Bytes:      sdBytes,
		},
	}

	return asn1.Marshal(ci)
}

func marshalOID(oid asn1.ObjectIdentifier) []byte {
	b, _ := asn1.Marshal(oid)
	// Strip tag and length, we just want the value bytes for the RawValue which adds its own tag/length?
	// No, RawValue.Bytes is the content bytes.
	// asn1.Marshal returns Tag + Length + Value.
	// We want Value.
	// But wait, `asn1.RawValue` with `Tag: asn1.TagObjectIdentifier` expects `Bytes` to be the content.

	// Let's parse it back to get the content bytes.
	var raw asn1.RawValue
	asn1.Unmarshal(b, &raw)
	return raw.Bytes
}

func marshalAttributes(attrs []attribute) ([]byte, error) {
	// We need to marshal as SET OF Attribute.
	// We can define a temporary type.
	type setOfAttributes struct {
		Attrs []attribute `asn1:"set"`
	}
	// But `attribute` itself is a SEQUENCE.
	// `[]attribute` inside a struct with `asn1:"set"` will be marshaled as SET OF SEQUENCE.

	// However, `signerInfo` uses `[0] IMPLICIT`.
	// Here we want the standard SET OF tag (17).

	// Let's try to marshal the slice directly.
	// asn1.Marshal(slice) marshals as SEQUENCE OF by default.
	// We need SET OF.

	// We can use a wrapper struct.
	wrapper := struct {
		Attrs []attribute `asn1:"set"`
	}{Attrs: attrs}

	b, err := asn1.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	// The wrapper adds a SEQUENCE tag around the SET.
	// We need to strip the outer SEQUENCE tag.

	var raw asn1.RawValue
	_, err = asn1.Unmarshal(b, &raw)
	if err != nil {
		return nil, err
	}

	// raw.Bytes is the content of the SEQUENCE, which is the SET OF Attributes.
	// But we want the SET OF Attributes including its tag/length.
	// The struct wrapper: SEQUENCE { SET OF Attributes }
	// So raw.Bytes is exactly the SET OF Attributes (Tag 17 + Length + Content).

	return raw.Bytes, nil
}
