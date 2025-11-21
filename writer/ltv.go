package writer

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/parser"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
)

// AddLTV adds Long Term Validation (LTV) data (DSS dictionary) to an existing PDF.
// It performs an incremental update.
func AddLTV(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, data security.LTVData) error {
	// 1. Parse original file to find Trailer and Size
	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(ctx, r)
	if err != nil {
		return fmt.Errorf("resolve xref: %w", err)
	}

	// Find max object number
	maxObj := 0
	for _, obj := range table.Objects() {
		if obj > maxObj {
			maxObj = obj
		}
	}
	nextObjID := maxObj + 1

	// 2. Read original file
	originalData := make([]byte, size)
	if _, err := r.ReadAt(originalData, 0); err != nil {
		return fmt.Errorf("read original: %w", err)
	}
	if _, err := w.Write(originalData); err != nil {
		return err
	}

	var updateBuf bytes.Buffer
	var newXRefEntries []string // "ID Gen Offset"

	// Helper to write object and track xref
	writeObj := func(id int, content []byte) {
		offset := size + int64(updateBuf.Len())
		newXRefEntries = append(newXRefEntries, fmt.Sprintf("%d 1\n%010d 00000 n \n", id, offset))
		fmt.Fprintf(&updateBuf, "%d 0 obj\n", id)
		updateBuf.Write(content)
		updateBuf.WriteString("\nendobj\n")
	}

	// 3. Create Streams for Certs, OCSPs, CRLs
	var certRefs []string
	for _, cert := range data.Certs {
		id := nextObjID
		nextObjID++
		// Write stream
		content := fmt.Sprintf("<< /Length %d >> stream\n", len(cert))
		contentBytes := append([]byte(content), cert...)
		contentBytes = append(contentBytes, []byte("\nendstream")...)
		writeObj(id, contentBytes)
		certRefs = append(certRefs, fmt.Sprintf("%d 0 R", id))
	}

	var ocspRefs []string
	for _, ocsp := range data.OCSPs {
		id := nextObjID
		nextObjID++
		content := fmt.Sprintf("<< /Length %d >> stream\n", len(ocsp))
		contentBytes := append([]byte(content), ocsp...)
		contentBytes = append(contentBytes, []byte("\nendstream")...)
		writeObj(id, contentBytes)
		ocspRefs = append(ocspRefs, fmt.Sprintf("%d 0 R", id))
	}

	var crlRefs []string
	for _, crl := range data.CRLs {
		id := nextObjID
		nextObjID++
		content := fmt.Sprintf("<< /Length %d >> stream\n", len(crl))
		contentBytes := append([]byte(content), crl...)
		contentBytes = append(contentBytes, []byte("\nendstream")...)
		writeObj(id, contentBytes)
		crlRefs = append(crlRefs, fmt.Sprintf("%d 0 R", id))
	}

	// 4. Create DSS Dictionary
	dssID := nextObjID
	nextObjID++
	var dssContent bytes.Buffer
	dssContent.WriteString("<<")
	if len(certRefs) > 0 {
		dssContent.WriteString(" /Certs [")
		for _, ref := range certRefs {
			dssContent.WriteString(" " + ref)
		}
		dssContent.WriteString(" ]")
	}
	if len(ocspRefs) > 0 {
		dssContent.WriteString(" /OCSPs [")
		for _, ref := range ocspRefs {
			dssContent.WriteString(" " + ref)
		}
		dssContent.WriteString(" ]")
	}
	if len(crlRefs) > 0 {
		dssContent.WriteString(" /CRLs [")
		for _, ref := range crlRefs {
			dssContent.WriteString(" " + ref)
		}
		dssContent.WriteString(" ]")
	}
	dssContent.WriteString(" >>")
	writeObj(dssID, dssContent.Bytes())

	// 5. Update Root
	// We need to read the existing Root and add /DSS
	trailer := resolver.Trailer()
	if trailer == nil {
		return fmt.Errorf("no trailer found")
	}
	rootRefObj, ok := trailer.Get(raw.NameLiteral("Root"))
	if !ok {
		return fmt.Errorf("no root in trailer")
	}
	rootRef, ok := rootRefObj.(raw.RefObj)
	if !ok {
		return fmt.Errorf("root is not a reference")
	}

	// Resolve Root object
	loader, err := (&parser.ObjectLoaderBuilder{}).
		WithReader(r).
		WithXRef(table).
		Build()
	if err != nil {
		return fmt.Errorf("build loader: %w", err)
	}

	rootObj, err := loader.Load(ctx, rootRef.Ref())
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	rootDict, ok := rootObj.(*raw.DictObj)
	if !ok {
		return fmt.Errorf("root is not a dictionary")
	}

	// Create new Root dictionary content
	// We can't easily serialize the existing raw.DictObj back to string perfectly without a serializer.
	// But we can try to reconstruct it or just append if we are careful.
	// Better: Use the `writer` package's serialization capabilities if available, or manual reconstruction.
	// Since `raw.DictObj` is a map, we can iterate.

	// Let's manually serialize the Root dict, adding/replacing /DSS
	// We can use serializePrimitive from helpers.go (it is in the same package)

	// Update Root dictionary in memory
	rootDict.Set(raw.NameLiteral("DSS"), raw.Ref(dssID, 0))

	// Serialize Root
	rootContent := serializePrimitive(rootDict)
	writeObj(rootRef.Ref().Num, rootContent)

	// 6. Write Trailer
	// Find previous xref offset
	prevXRef, err := xref.FindStartXRef(r, size)
	if err != nil {
		return fmt.Errorf("find startxref: %w", err)
	}

	// Construct XRef section
	xrefOffset := size + int64(updateBuf.Len())
	fmt.Fprintf(&updateBuf, "xref\n0 1\n0000000000 65535 f \n")
	for _, entry := range newXRefEntries {
		updateBuf.WriteString(entry)
	}

	// Trailer
	fmt.Fprintf(&updateBuf, "trailer\n<< /Size %d /Root %d %d R /Prev %d >>\n", nextObjID, rootRef.Ref().Num, rootRef.Ref().Gen, prevXRef)
	fmt.Fprintf(&updateBuf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	// Write update buffer
	if _, err := w.Write(updateBuf.Bytes()); err != nil {
		return err
	}

	return nil
}
