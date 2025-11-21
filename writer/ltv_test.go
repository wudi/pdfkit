package writer

import (
	"bytes"
	"context"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/parser"
	"github.com/wudi/pdfkit/security"
	"github.com/wudi/pdfkit/xref"
)

func TestAddLTV(t *testing.T) {
	// 1. Create a minimal valid PDF
	b := builder.NewBuilder()
	b.NewPage(612, 792).DrawText("LTV Test", 100, 700, builder.TextOptions{FontSize: 12}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Failed to build PDF doc: %v", err)
	}

	var buf bytes.Buffer
	w := NewWriter()
	err = w.Write(context.Background(), doc, &buf, Config{Version: PDF17})
	if err != nil {
		t.Fatalf("Failed to write PDF: %v", err)
	}
	pdfContent := buf.Bytes()

	// 2. Prepare LTV Data
	ltvData := security.LTVData{
		Certs: [][]byte{[]byte("cert1"), []byte("cert2")},
		OCSPs: [][]byte{[]byte("ocsp1")},
		CRLs:  [][]byte{[]byte("crl1")},
	}

	// 3. Add LTV
	var ltvBuf bytes.Buffer
	err = AddLTV(context.Background(), bytes.NewReader(pdfContent), int64(len(pdfContent)), &ltvBuf, ltvData)
	if err != nil {
		t.Fatalf("AddLTV failed: %v", err)
	}

	// 4. Verify Output
	// The output should contain the original PDF + incremental update
	// We can check for /DSS and the streams
	outBytes := ltvBuf.Bytes()
	if !bytes.Contains(outBytes, []byte("/DSS")) {
		t.Error("Output does not contain /DSS")
	}
	if !bytes.Contains(outBytes, []byte("cert1")) {
		t.Error("Output does not contain cert1")
	}
	if !bytes.Contains(outBytes, []byte("ocsp1")) {
		t.Error("Output does not contain ocsp1")
	}

	// Verify structure with resolver
	resolver := xref.NewResolver(xref.ResolverConfig{})
	table, err := resolver.Resolve(context.Background(), bytes.NewReader(outBytes))
	if err != nil {
		t.Fatalf("Failed to resolve LTV PDF: %v", err)
	}

	loader, err := (&parser.ObjectLoaderBuilder{}).
		WithReader(bytes.NewReader(outBytes)).
		WithXRef(table).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Check Root has DSS
	trailer := resolver.Trailer()
	rootRefObj, _ := trailer.Get(raw.NameLiteral("Root"))
	rootRef := rootRefObj.(raw.RefObj).Ref()
	rootObj, _ := loader.Load(context.Background(), rootRef)
	rootDict := rootObj.(*raw.DictObj)

	dssRefObj, ok := rootDict.Get(raw.NameLiteral("DSS"))
	if !ok {
		t.Fatal("Root does not have DSS entry")
	}
	dssRef := dssRefObj.(raw.RefObj).Ref()
	dssObj, _ := loader.Load(context.Background(), dssRef)
	dssDict := dssObj.(*raw.DictObj)

	// Check Certs
	certsObj, ok := dssDict.Get(raw.NameLiteral("Certs"))
	if !ok {
		t.Fatal("DSS does not have Certs entry")
	}
	certsArr := certsObj.(*raw.ArrayObj)
	if len(certsArr.Items) != 2 {
		t.Errorf("Expected 2 certs, got %d", len(certsArr.Items))
	}
}
