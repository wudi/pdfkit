package writer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
)

// SignConfig configures the digital signature.
type SignConfig struct {
	Reason    string
	Location  string
	Contact   string
	FieldName string // Name of the signature field (optional)
	PAdES     bool   // Enable PAdES (ETSI.CAdES.detached)
}

// Sign appends a digital signature to an existing PDF.
// It writes the signed PDF to w.
func Sign(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, signer security.Signer, cfg SignConfig) error {
	// 1. Parse original file to find Trailer and Size
	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(ctx, r)
	if err != nil {
		return fmt.Errorf("resolve xref: %w", err)
	}

	// Find max object number to determine new ID
	maxObj := 0
	for _, obj := range table.Objects() {
		if obj > maxObj {
			maxObj = obj
		}
	}
	sigObjID := maxObj + 1

	// 2. Read original file
	originalData := make([]byte, size)
	if _, err := r.ReadAt(originalData, 0); err != nil {
		return fmt.Errorf("read original: %w", err)
	}

	// 3. Prepare Signature Dictionary Parts
	const sigLen = 8192 // Reserve 8KB for signature

	var updateBuf bytes.Buffer

	// Object Header
	fmt.Fprintf(&updateBuf, "%d 0 obj\n", sigObjID)

	subFilter := "/adbe.pkcs7.detached"
	if cfg.PAdES {
		subFilter = "/ETSI.CAdES.detached"
		// Auto-configure RSASigner if possible
		if rsaSigner, ok := signer.(*security.RSASigner); ok {
			rsaSigner.SetPAdES(true)
		}
	}

	updateBuf.WriteString("<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter " + subFilter)

	if cfg.Reason != "" {
		fmt.Fprintf(&updateBuf, " /Reason (%s)", cfg.Reason)
	}
	if cfg.Location != "" {
		fmt.Fprintf(&updateBuf, " /Location (%s)", cfg.Location)
	}
	if cfg.Contact != "" {
		fmt.Fprintf(&updateBuf, " /ContactInfo (%s)", cfg.Contact)
	}
	fmt.Fprintf(&updateBuf, " /M (%s)", formatDate(time.Now()))

	// ByteRange
	// We use a fixed format for ByteRange to ensure stable length.
	// Format: "0 %020d %020d %020d]"
	const byteRangeFormat = "0 %020d %020d %020d]"
	dummyByteRange := fmt.Sprintf(byteRangeFormat, 0, 0, 0)
	byteRangeStrLen := len(dummyByteRange)

	updateBuf.WriteString(" /ByteRange [")

	// Current offset in updateBuf
	prefixLen := updateBuf.Len()

	contentsKey := " /Contents <"
	contentsKeyLen := len(contentsKey)

	// holeStart is the offset in the file where the signature hex string begins.
	holeStart := size + int64(prefixLen) + int64(byteRangeStrLen) + int64(contentsKeyLen)
	holeLen := int64(sigLen * 2)
	holeEnd := holeStart + holeLen

	// Buffer the rest of the update (Trailer, XRef)
	var trailerBuf bytes.Buffer
	trailerBuf.WriteString("> >>\nendobj\n")

	// XRef Table for the new object
	// The xref table starts after the signature object is closed.
	xrefOffset := holeEnd + int64(trailerBuf.Len())

	// Construct XRef section
	fmt.Fprintf(&trailerBuf, "xref\n0 1\n0000000000 65535 f \n%d 1\n%010d 00000 n \n", sigObjID, size)

	// Trailer
	// We need previous Root.
	var rootRef raw.ObjectRef
	if trailer := resolver.Trailer(); trailer != nil {
		if r, ok := trailer.Get(raw.NameLiteral("Root")); ok {
			if ref, ok := r.(raw.RefObj); ok {
				rootRef = ref.Ref()
			}
		}
	}

	// Find previous xref offset
	prevXRef, err := xref.FindStartXRef(r, size)
	if err != nil {
		return fmt.Errorf("find startxref: %w", err)
	}

	fmt.Fprintf(&trailerBuf, "trailer\n<< /Size %d /Root %d %d R /Prev %d >>\n", sigObjID+1, rootRef.Num, rootRef.Gen, prevXRef)

	fmt.Fprintf(&trailerBuf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	lenAfterHole := int64(trailerBuf.Len())

	// ByteRange: [0, holeStart, holeEnd, lenAfterHole]
	byteRangeStr := fmt.Sprintf(byteRangeFormat, holeStart, holeEnd, lenAfterHole)
	if len(byteRangeStr) != byteRangeStrLen {
		// This should theoretically not happen given the %020d format and int64 limits,
		// but as a safety measure we can pad with spaces if it's too short.
		if len(byteRangeStr) < byteRangeStrLen {
			diff := byteRangeStrLen - len(byteRangeStr)
			for i := 0; i < diff; i++ {
				byteRangeStr += " "
			}
		} else {
			return fmt.Errorf("calculated ByteRange string too long: %d > %d", len(byteRangeStr), byteRangeStrLen)
		}
	}

	// Write to output
	if _, err := w.Write(originalData); err != nil {
		return err
	}
	if _, err := w.Write(updateBuf.Bytes()); err != nil {
		return err
	}
	if _, err := w.Write([]byte(byteRangeStr)); err != nil {
		return err
	}
	if _, err := w.Write([]byte(contentsKey)); err != nil {
		return err
	}

	// Calculate Hash
	hasher := sha256.New()
	hasher.Write(originalData)
	hasher.Write(updateBuf.Bytes())
	hasher.Write([]byte(byteRangeStr))
	hasher.Write([]byte(contentsKey))

	// Now we need to hash the second part: trailerBuf
	hasher.Write(trailerBuf.Bytes())

	digest := hasher.Sum(nil)

	// Sign
	signature, err := signer.Sign(digest)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	// Encode signature to hex
	sigHex := make([]byte, hex.EncodedLen(len(signature)))
	hex.Encode(sigHex, signature)

	// Pad with 0s to fill hole
	if int64(len(sigHex)) > holeLen {
		return fmt.Errorf("signature too large: %d > %d", len(sigHex), holeLen)
	}

	// Write Signature
	if _, err := w.Write(sigHex); err != nil {
		return err
	}
	padding := holeLen - int64(len(sigHex))
	pad := make([]byte, padding)
	for i := range pad {
		pad[i] = '0'
	}
	if _, err := w.Write(pad); err != nil {
		return err
	}

	// Write Trailer
	if _, err := w.Write(trailerBuf.Bytes()); err != nil {
		return err
	}

	return nil
}

func formatDate(t time.Time) string {
	_, offset := t.Zone()
	sign := '+'
	if offset < 0 {
		sign = '-'
		offset = -offset
	}
	h := offset / 3600
	m := (offset % 3600) / 60
	return fmt.Sprintf("D:%04d%02d%02d%02d%02d%02d%c%02d'%02d'",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), sign, h, m)
}
