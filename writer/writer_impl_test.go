package writer

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"context"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"encoding/ascii85"
	"encoding/hex"
	"fmt"

	"pdflib/builder"
	"pdflib/fonts"
	"pdflib/ir"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/security"
	"pdflib/xref"

	"golang.org/x/image/font/gofont/goregular"
)

type staticCtx struct{}

func (staticCtx) Done() <-chan struct{} { return nil }

func TestWriterRoundTripPipeline(t *testing.T) {
	// Build a simple document with one page and text.
	b := builder.NewBuilder()
	b.NewPage(200, 200).DrawText("Hello", 10, 20, builder.TextOptions{FontSize: 12}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse using default pipeline and ensure stream contains our text.
	p := ir.NewDefault()
	out, err := p.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse roundtrip: %v", err)
	}
	if out.Decoded() == nil || len(out.Decoded().Streams) == 0 {
		t.Fatalf("expected decoded streams")
	}
	found := false
	for _, s := range out.Decoded().Streams {
		if bytes.Contains(s.Data(), []byte("Hello")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("written content not found in decoded streams")
	}
}

func TestWriter_InfoMetadataIDDeterministic(t *testing.T) {
	b := builder.NewBuilder()
	b.SetInfo(&semantic.DocumentInfo{Title: "Sample Title"})
	b.SetMetadata([]byte("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\"></x:xmpmeta>"))
	b.NewPage(100, 100).DrawText("Hi", 5, 5, builder.TextOptions{FontSize: 10}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	cfg := Config{Version: PDFVersion("1.6"), Deterministic: true}
	w := (&WriterBuilder{}).Build()

	var first bytes.Buffer
	if err := w.Write(staticCtx{}, doc, &first, cfg); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := first.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-1.6")) {
		t.Fatalf("expected PDF 1.6 header, got %q", data[:8])
	}
	if !bytes.Contains(data, []byte("/Title (Sample Title)")) {
		t.Fatalf("missing Title in output")
	}
	if !bytes.Contains(data, []byte("/Metadata")) {
		t.Fatalf("missing Metadata reference in catalog")
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse for IDs: %v", err)
	}
	idObj, ok := rawDoc.Trailer.Get(raw.NameLiteral("ID"))
	if !ok {
		t.Fatalf("ID missing from trailer")
	}
	idArr, ok := idObj.(*raw.ArrayObj)
	if !ok || idArr.Len() != 2 {
		t.Fatalf("ID array malformed")
	}
	idA := idBytes(idArr.Items[0])
	idB := idBytes(idArr.Items[1])
	if len(idA) == 0 || len(idB) == 0 {
		t.Fatalf("ID entries empty")
	}
	if !bytes.Equal(idA, idB) {
		t.Fatalf("expected matching ID pair in trailer")
	}

	var second bytes.Buffer
	if err := w.Write(staticCtx{}, doc, &second, cfg); err != nil {
		t.Fatalf("write pdf second: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("deterministic write expected identical output")
	}
}

func TestWriter_PageGeometryAndResources(t *testing.T) {
	page1 := &semantic.Page{
		MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 400},
		CropBox:  semantic.Rectangle{LLX: 10, LLY: 10, URX: 290, URY: 390},
		Rotate:   90,
		UserUnit: 2.0,
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"FBody": {BaseFont: "Helvetica"},
				"FMono": {BaseFont: "Courier"},
			},
		},
		Contents: []semantic.ContentStream{
			{RawBytes: []byte("BT /FBody 12 Tf 0 0 Td (Hi) Tj ET")},
		},
	}
	page2 := &semantic.Page{
		MediaBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 400},
		Resources: &semantic.Resources{
			Fonts: map[string]*semantic.Font{
				"FBody": {BaseFont: "Helvetica"},
			},
		},
		Contents: []semantic.ContentStream{
			{RawBytes: []byte("BT /FBody 10 Tf 10 10 Td (Two) Tj ET")},
		},
	}
	doc := &semantic.Document{Pages: []*semantic.Page{page1, page2}}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	strData := string(data)

	if !strings.Contains(strData, "/Rotate 90") {
		t.Fatalf("expected Rotate entry")
	}
	if !regexp.MustCompile(`/CropBox\s*\[\s*10\.?0*\s+10\.?0*\s+290\.?0*\s+390\.?0*\s*\]`).Match(data) {
		t.Fatalf("expected CropBox in page dictionary")
	}
	if !strings.Contains(strData, "/UserUnit 2") {
		t.Fatalf("expected UserUnit entry")
	}
	if strings.Count(strData, "/BaseFont /Helvetica") != 1 {
		t.Fatalf("expected shared Helvetica font object")
	}
	if strings.Count(strData, "/BaseFont /Courier") != 1 {
		t.Fatalf("expected single Courier font object")
	}
	fontsRegex := regexp.MustCompile(`/Font\s*<<[^>]*?/FBody\s+\d+\s+0\s+R[^>]*?/FMono\s+\d+\s+0\s+R[^>]*?>>`)
	if !fontsRegex.Match(data) {
		t.Fatalf("expected page font resource entries")
	}
}

func TestWriter_XRefStream(t *testing.T) {
	b := builder.NewBuilder()
	b.NewPage(100, 100).DrawText("XRef", 5, 5, builder.TextOptions{FontSize: 9}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	cfg := Config{XRefStreams: true, Deterministic: true}
	if err := w.Write(staticCtx{}, doc, &buf, cfg); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	if !bytes.Contains(data, []byte("/Type /XRef")) {
		t.Fatalf("expected xref stream type")
	}
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	m := re.FindSubmatch(data)
	if len(m) != 2 {
		t.Fatalf("startxref not found")
	}
	startOff, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parse startxref: %v", err)
	}
	if startOff <= 0 || startOff >= len(data) {
		t.Fatalf("startxref out of bounds: %d", startOff)
	}

	res := xref.NewResolver(xref.ResolverConfig{})
	table, err := res.Resolve(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("resolve xref stream: %v", err)
	}
	if table.Type() != "xref-stream" {
		t.Fatalf("expected xref-stream table, got %s", table.Type())
	}
	if off, _, ok := table.Lookup(1); !ok || off == 0 {
		t.Fatalf("catalog entry missing in xref stream")
	}
}

func TestWriter_XRefTableOffsets(t *testing.T) {
	b := builder.NewBuilder()
	b.NewPage(80, 80).DrawText("table", 2, 2, builder.TextOptions{}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true, XRefStreams: false}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	start := startXRef(data)
	if start <= 0 || start >= int64(len(data)) {
		t.Fatalf("invalid startxref: %d", start)
	}
	if !bytes.HasPrefix(data[start:], []byte("xref")) {
		t.Fatalf("startxref does not point to xref table")
	}
	res := xref.NewResolver(xref.ResolverConfig{})
	table, err := res.Resolve(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("resolve xref table: %v", err)
	}
	if table.Type() != "table" && table.Type() != "xref-table" {
		t.Fatalf("expected xref table, got %s", table.Type())
	}
	off, gen, ok := table.Lookup(1)
	if !ok || off == 0 || gen != 0 {
		t.Fatalf("catalog offset missing: off=%d gen=%d ok=%v", off, gen, ok)
	}
	if off >= int64(len(data)) || !bytes.HasPrefix(data[off:], []byte("1 0 obj")) {
		t.Fatalf("offset does not point to catalog object")
	}
	offsetMap := scanObjectOffsets(data)
	for _, objNum := range table.Objects() {
		if objNum == 0 {
			continue
		}
		entryOffset, _, ok := table.Lookup(objNum)
		if !ok {
			continue
		}
		if actual, ok := offsetMap[objNum]; ok && actual != entryOffset {
			t.Fatalf("xref offset mismatch for obj %d: table=%d actual=%d", objNum, entryOffset, actual)
		}
	}
}

func TestWriter_IncrementalAppend(t *testing.T) {
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()

	// First revision
	orig := builder.NewBuilder()
	orig.NewPage(50, 50).DrawText("v1", 1, 1, builder.TextOptions{}).Finish()
	doc1, _ := orig.Build()
	if err := w.Write(staticCtx{}, doc1, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write first: %v", err)
	}
	baseData := buf.Bytes()
	baseMax := maxObjNum(baseData)
	prevOffset := startXRef(baseData)
	firstLen := buf.Len()

	// Second revision (incremental) with new page
	rev := builder.NewBuilder()
	rev.NewPage(50, 50).DrawText("v1", 1, 1, builder.TextOptions{}).Finish()
	rev.NewPage(60, 60).DrawText("v2", 2, 2, builder.TextOptions{}).Finish()
	doc2, _ := rev.Build()
	cfg := Config{Incremental: true, Deterministic: true, XRefStreams: true}
	if err := w.Write(staticCtx{}, doc2, &buf, cfg); err != nil {
		t.Fatalf("write incremental: %v", err)
	}
	data := buf.Bytes()
	if len(data) <= firstLen {
		t.Fatalf("incremental write did not append data")
	}
	if !bytes.Contains(data[firstLen:], []byte("/Prev")) {
		t.Fatalf("expected Prev in incremental trailer")
	}
	if !bytes.Contains(data[firstLen:], []byte(strconv.FormatInt(prevOffset, 10))) {
		t.Fatalf("Prev does not reference prior xref offset")
	}
	if maxObjNum(data) <= baseMax {
		t.Fatalf("expected object numbers to increase for incremental update")
	}
	p := ir.NewDefault()
	if _, err := p.Parse(context.Background(), bytes.NewReader(data)); err != nil {
		t.Fatalf("parse incremental: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("raw parse incremental: %v", err)
	}
	if len(rawDoc.Objects) <= baseMax {
		t.Fatalf("expected raw object count to grow")
	}
}

func TestWriter_IncrementalPreservesOffsets(t *testing.T) {
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()

	base := builder.NewBuilder()
	base.NewPage(40, 40).DrawText("base", 1, 1, builder.TextOptions{}).Finish()
	doc1, _ := base.Build()
	if err := w.Write(staticCtx{}, doc1, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write base: %v", err)
	}
	baseData := buf.Bytes()
	baseOffsets := parseXRefTableOffsets(baseData)
	if len(baseOffsets) == 0 {
		t.Fatalf("failed to parse base xref offsets")
	}
	firstObj := baseOffsets[1]
	if firstObj == 0 {
		t.Fatalf("expected catalog offset to be tracked")
	}
	firstLen := len(baseData)

	rev := builder.NewBuilder()
	rev.NewPage(40, 40).DrawText("base", 1, 1, builder.TextOptions{}).Finish()
	rev.NewPage(50, 50).DrawText("rev", 2, 2, builder.TextOptions{}).Finish()
	doc2, _ := rev.Build()
	cfg := Config{Incremental: true, Deterministic: true}
	if err := w.Write(staticCtx{}, doc2, &buf, cfg); err != nil {
		t.Fatalf("write incremental: %v", err)
	}
	data := buf.Bytes()
	if len(data) <= firstLen {
		t.Fatalf("incremental write did not append data")
	}
	offsets := parseXRefTableOffsets(data)
	if offsets[1] != firstObj {
		t.Fatalf("expected prior object offset preserved, got %d want %d", offsets[1], firstObj)
	}
	if !bytes.Contains(data[firstLen:], []byte("/Prev")) {
		t.Fatalf("incremental trailer missing Prev")
	}
}

func TestWriter_StringEscaping(t *testing.T) {
	b := builder.NewBuilder()
	text := "Hello (world) \\ \n\t\r"
	b.SetInfo(&semantic.DocumentInfo{Title: text})
	b.NewPage(100, 100).DrawText("dummy", 1, 1, builder.TextOptions{}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	out := buf.String()
	infoStart := strings.Index(out, "/Title")
	if infoStart == -1 {
		t.Fatalf("info dict missing Title")
	}
	if !strings.Contains(out, `\n`) || !strings.Contains(out, `\t`) {
		t.Fatalf("expected escaped newline and tab in Title")
	}
	if strings.Contains(out, "(Hello (world) \\ ") {
		t.Fatalf("expected parentheses and backslash to be escaped")
	}
	if !strings.Contains(out, `\(world\)`) {
		t.Fatalf("escaped parentheses missing")
	}
}

func TestWriter_ContentStreamCompression(t *testing.T) {
	b := builder.NewBuilder()
	b.NewPage(200, 200).DrawText("compress me", 10, 20, builder.TextOptions{FontSize: 8}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true, Compression: flate.BestSpeed}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw pdf: %v", err)
	}
	found := false
	for _, obj := range rawDoc.Objects {
		stream, ok := obj.(*raw.StreamObj)
		if !ok {
			continue
		}
		if tObj, ok := stream.Dict.Get(raw.NameLiteral("Type")); ok {
			if n, ok := tObj.(raw.NameObj); ok && n.Value() == "XRef" {
				continue
			}
		}
		filterObj, ok := stream.Dict.Get(raw.NameLiteral("Filter"))
		if !ok {
			continue
		}
		nameObj, ok := filterObj.(raw.NameObj)
		if !ok || nameObj.Value() != "FlateDecode" {
			continue
		}
		found = true
		r := flate.NewReader(bytes.NewReader(stream.Data))
		defer r.Close()
		decoded, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read flate: %v", err)
		}
		if !strings.Contains(string(decoded), "compress me") {
			t.Fatalf("decoded stream missing text: %q", decoded)
		}
	}
	if !found {
		t.Fatalf("compressed content stream not found")
	}
}

func TestWriter_ContentStream_ASCIIHexAndASCII85(t *testing.T) {
	text := "encode me please"
	check := func(filter ContentFilter, expectedFilter string, decoder func(data []byte) ([]byte, error)) {
		b := builder.NewBuilder()
		b.NewPage(100, 100).DrawText(text, 1, 1, builder.TextOptions{}).Finish()
		doc, err := b.Build()
		if err != nil {
			t.Fatalf("build doc: %v", err)
		}
		var buf bytes.Buffer
		w := (&WriterBuilder{}).Build()
		if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true, ContentFilter: filter}); err != nil {
			t.Fatalf("write pdf (%s): %v", expectedFilter, err)
		}
		rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
		rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("parse raw pdf: %v", err)
		}
		found := false
		for _, obj := range rawDoc.Objects {
			stream, ok := obj.(*raw.StreamObj)
			if !ok {
				continue
			}
			if tObj, ok := stream.Dict.Get(raw.NameLiteral("Type")); ok {
				if n, ok := tObj.(raw.NameObj); ok && n.Value() == "XRef" {
					continue
				}
			}
			filterObj, ok := stream.Dict.Get(raw.NameLiteral("Filter"))
			if !ok {
				continue
			}
			if name, ok := filterObj.(raw.NameObj); ok && name.Value() == expectedFilter {
				found = true
				decoded, err := decoder(stream.Data)
				if err != nil {
					t.Fatalf("decode stream (%s): %v", expectedFilter, err)
				}
				if !strings.Contains(string(decoded), text) {
					t.Fatalf("decoded stream missing text (%s): %q", expectedFilter, decoded)
				}
			}
		}
		if !found {
			t.Fatalf("content stream with filter %s not found", expectedFilter)
		}
	}

	check(FilterASCIIHex, "ASCIIHexDecode", func(data []byte) ([]byte, error) {
		trimmed := strings.TrimSpace(string(data))
		trimmed = strings.TrimSuffix(trimmed, ">")
		decoded, err := hex.DecodeString(trimmed)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	})
	check(FilterASCII85, "ASCII85Decode", func(data []byte) ([]byte, error) {
		s := string(data)
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "<~")
		s = strings.TrimSuffix(s, "~>")
		decoded := make([]byte, len(data))
		n, _, err := ascii85.Decode(decoded, []byte(s), true)
		if err != nil {
			return nil, err
		}
		return decoded[:n], nil
	})
	check(FilterRunLength, "RunLengthDecode", func(data []byte) ([]byte, error) {
		return runLengthDecode(data)
	})
	check(FilterLZW, "LZWDecode", func(data []byte) ([]byte, error) {
		r := lzw.NewReader(bytes.NewReader(data), lzw.LSB, 8)
		defer r.Close()
		return io.ReadAll(r)
	})
}

func TestWriter_ContentStreamJPXJBIG2(t *testing.T) {
	content := []byte{0x00, 0x01, 0x02}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 5, URY: 5}, Contents: []semantic.ContentStream{{RawBytes: content}}},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{ContentFilter: FilterJPX, Deterministic: true}); err != nil {
		t.Fatalf("write jpx: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Filter /JPXDecode")) {
		t.Fatalf("JPX filter not set")
	}
	buf.Reset()
	if err := w.Write(staticCtx{}, doc, &buf, Config{ContentFilter: FilterJBIG2, Deterministic: true}); err != nil {
		t.Fatalf("write jbig2: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Filter /JBIG2Decode")) {
		t.Fatalf("JBIG2 filter not set")
	}
}

func TestWriter_FontWidths(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": {BaseFont: "Helvetica", Widths: map[int]int{65: 500, 66: 505}, Encoding: "WinAnsiEncoding"},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundWidths := false
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if tval, ok := d.Get(raw.NameLiteral("Type")); !ok {
			continue
		} else if n, ok := tval.(raw.NameObj); !ok || n.Value() != "Font" {
			continue
		}
		if enc, ok := d.Get(raw.NameLiteral("Encoding")); !ok {
			t.Fatalf("encoding missing")
		} else if n, ok := enc.(raw.NameObj); !ok || n.Value() != "WinAnsiEncoding" {
			t.Fatalf("encoding mismatch: %#v", enc)
		}
		if fc, ok := d.Get(raw.NameLiteral("FirstChar")); ok {
			if fi, ok := fc.(raw.NumberObj); ok && fi.Int() == 65 {
				lc, _ := d.Get(raw.NameLiteral("LastChar"))
				if li, ok := lc.(raw.NumberObj); ok && li.Int() == 66 {
					widthsVal, _ := d.Get(raw.NameLiteral("Widths"))
					if arr, ok := widthsVal.(*raw.ArrayObj); ok && arr.Len() == 2 {
						if w1, ok := arr.Items[0].(raw.NumberObj); ok && w1.Int() == 500 {
							if w2, ok := arr.Items[1].(raw.NumberObj); ok && w2.Int() == 505 {
								foundWidths = true
							}
						}
					}
				}
			}
		}
	}
	if !foundWidths {
		t.Fatalf("font widths not found")
	}
}

func TestWriter_FontTrueTypeAndCID(t *testing.T) {
	trueType := &semantic.Font{BaseFont: "MyTrueType", Subtype: "TrueType", Encoding: "MacRomanEncoding", Widths: map[int]int{65: 600}}
	cidFont := &semantic.Font{
		BaseFont: "MyCIDFont",
		Subtype:  "Type0",
		Encoding: "Identity-H",
		DescendantFont: &semantic.CIDFont{
			Subtype:  "CIDFontType2",
			BaseFont: "MyCIDFont",
			CIDSystemInfo: semantic.CIDSystemInfo{
				Registry:   "Adobe",
				Ordering:   "Identity",
				Supplement: 0,
			},
			DW: 750,
			W:  map[int]int{1: 500, 2: 500, 5: 700},
		},
	}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"FTrue": trueType,
						"FCID":  cidFont,
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT /FTrue 12 Tf (Hi) Tj ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	if !regexp.MustCompile(`/Subtype\s*/TrueType`).Match(data) {
		t.Fatalf("expected TrueType subtype in font dictionary")
	}
	if !regexp.MustCompile(`/Encoding\s*/MacRomanEncoding`).Match(data) {
		t.Fatalf("expected MacRomanEncoding for TrueType font")
	}
	if !regexp.MustCompile(`/Subtype\s*/Type0`).Match(data) {
		t.Fatalf("expected Type0 font for CID entry")
	}
	if !strings.Contains(string(data), "/DescendantFonts") {
		t.Fatalf("expected DescendantFonts entry for CID font")
	}
	if !regexp.MustCompile(`/Subtype\s*/CIDFontType2`).Match(data) {
		t.Fatalf("expected CIDFontType2 descendant")
	}
	if !regexp.MustCompile(`/CIDSystemInfo\s*<<`).Match(data) {
		t.Fatalf("expected CIDSystemInfo dictionary")
	}
	if !regexp.MustCompile(`/W\s*\[\s*1\s+2\s+500\s+5\s+5\s+700\s*\]`).Match(data) {
		t.Fatalf("expected W array for CID widths")
	}
}

func TestWriter_EmbedTrueTypeFont(t *testing.T) {
	ttFont, err := fonts.LoadTrueType("GoRegular", goregular.TTF)
	if err != nil {
		t.Fatalf("load truetype: %v", err)
	}
	var sampleCID int
	var sampleRune rune
	for cid, runes := range ttFont.ToUnicode {
		for _, r := range runes {
			if r > 127 {
				sampleCID = cid
				sampleRune = r
				break
			}
		}
		if sampleCID != 0 {
			break
		}
	}
	if sampleCID == 0 {
		for cid, runes := range ttFont.ToUnicode {
			if len(runes) > 0 {
				sampleCID = cid
				sampleRune = runes[0]
				break
			}
		}
	}
	if sampleCID == 0 {
		t.Fatalf("no ToUnicode mappings found in font")
	}
	cidBytes := []byte{byte(sampleCID >> 8), byte(sampleCID)}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					Fonts: map[string]*semantic.Font{
						"F1": ttFont,
					},
				},
				Contents: []semantic.ContentStream{{
					Operations: []semantic.Operation{
						{Operator: "BT"},
						{Operator: "Tf", Operands: []semantic.Operand{semantic.NameOperand{Value: "F1"}, semantic.NumberOperand{Value: 12}}},
						{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: cidBytes}}},
						{Operator: "ET"},
					},
				}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	if !bytes.Contains(data, []byte("/FontFile2")) {
		t.Fatalf("expected embedded FontFile2 stream")
	}
	if !bytes.Contains(data, []byte(fmt.Sprintf("/Length %d", len(goregular.TTF)))) {
		t.Fatalf("font file length missing")
	}
	if !bytes.Contains(data, []byte("/Type0")) || !bytes.Contains(data, []byte("/CIDFontType2")) {
		t.Fatalf("expected Type0 font with CIDFontType2 descendant")
	}
	if !bytes.Contains(data, []byte("/Identity-H")) {
		t.Fatalf("expected Identity-H encoding for embedded font")
	}
	expectedMap := fmt.Sprintf("<%04X> <%s>", sampleCID, toUnicodeHex([]rune{sampleRune}))
	if !bytes.Contains(data, []byte(expectedMap)) {
		t.Fatalf("expected ToUnicode mapping for CID %d rune %U", sampleCID, sampleRune)
	}
}

func toUnicodeHex(runes []rune) string {
	encoded := utf16.Encode(runes)
	var b strings.Builder
	for _, u := range encoded {
		fmt.Fprintf(&b, "%04X", u)
	}
	return b.String()
}

func TestWriter_EncryptDictionary(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 5, URY: 5}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Encrypted: true,
		Permissions: raw.Permissions{
			Print:  true,
			Modify: true,
		},
	}
	w := (&WriterBuilder{}).Build()
	for _, cfg := range []Config{{Deterministic: true}, {Deterministic: true, XRefStreams: true}} {
		var buf bytes.Buffer
		if err := w.Write(staticCtx{}, doc, &buf, cfg); err != nil {
			t.Fatalf("write encrypted (streams=%v): %v", cfg.XRefStreams, err)
		}
		data := buf.Bytes()
		if !bytes.Contains(data, []byte("/Encrypt")) {
			t.Fatalf("Encrypt missing from output (streams=%v)", cfg.XRefStreams)
		}
		if !bytes.Contains(data, []byte("/Filter /Standard")) || !bytes.Contains(data, []byte("/O ")) || !bytes.Contains(data, []byte("/U ")) {
			t.Fatalf("Encrypt dictionary fields missing (streams=%v)", cfg.XRefStreams)
		}
	}
}

func TestWriter_EmbeddedFilesAndAF(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		EmbeddedFiles: []semantic.EmbeddedFile{
			{
				Name:         "invoice.xml",
				Description:  "ZUGFeRD payload",
				Relationship: "Data",
				Subtype:      "application/xml",
				Data:         []byte("<Invoice></Invoice>"),
			},
		},
	}
	w := (&WriterBuilder{}).Build()
	var buf bytes.Buffer
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	os.WriteFile("/tmp/writer_test_output.pdf", buf.Bytes(), 0644)

	pdf := buf.Bytes()
	if !bytes.Contains(pdf, []byte("/AFRelationship /Data")) {
		t.Fatalf("expected AFRelationship /Data entry")
	}
	if !bytes.Contains(pdf, []byte("/Subtype /application#2Fxml")) {
		t.Fatalf("expected MIME subtype encoding")
	}
	if !regexp.MustCompile(`/AF\s*\[\s*\d+\s+0\s+R`).Match(pdf) {
		t.Fatalf("expected catalog AF array with file spec reference")
	}
	if !regexp.MustCompile(`/Names\s*<<[^>]*EmbeddedFiles`).Match(pdf) {
		t.Fatalf("expected names dictionary with EmbeddedFiles entry")
	}
	if !regexp.MustCompile(`\(invoice\.xml\)\s+\d+\s+0\s+R`).Match(pdf) {
		t.Fatalf("expected filename mapping in names array")
	}
	if !bytes.Contains(pdf, []byte("ZUGFeRD payload")) {
		t.Fatalf("expected embedded file description")
	}
}

func TestSerializePrimitive_HexString(t *testing.T) {
	out := serializePrimitive(raw.HexStr([]byte{0x00, 0xAB, 0x10, 0xFF}))
	if string(out) != "<00AB10FF>" {
		t.Fatalf("unexpected hex string serialization: %s", out)
	}
}

func runLengthDecode(data []byte) ([]byte, error) {
	var out bytes.Buffer
	for i := 0; i < len(data); {
		b := data[i]
		i++
		if b == 128 {
			break
		}
		if b <= 127 {
			count := int(b) + 1
			if i+count > len(data) {
				return nil, fmt.Errorf("literal run out of bounds")
			}
			out.Write(data[i : i+count])
			i += count
			continue
		}
		// repeat next byte 257 - b times
		if i >= len(data) {
			return nil, fmt.Errorf("repeat without byte")
		}
		val := data[i]
		i++
		count := 257 - int(b)
		for j := 0; j < count; j++ {
			out.WriteByte(val)
		}
	}
	return out.Bytes(), nil
}

func TestWriter_ProcSetIncluded(t *testing.T) {
	b := builder.NewBuilder()
	b.NewPage(50, 50).DrawText("procset", 1, 1, builder.TextOptions{}).Finish()
	doc, err := b.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundPageProcSet := false
	foundPagesProcSet := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if ps, ok := resDict.Get(raw.NameLiteral("ProcSet")); ok {
						if arr, ok := ps.(*raw.ArrayObj); ok && arr.Len() >= 2 {
							foundPageProcSet = true
						}
					}
				}
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if ps, ok := resDict.Get(raw.NameLiteral("ProcSet")); ok {
						if arr, ok := ps.(*raw.ArrayObj); ok && arr.Len() >= 2 {
							foundPagesProcSet = true
						}
					}
				}
			}
		}
	}
	if !foundPageProcSet || !foundPagesProcSet {
		t.Fatalf("procset missing (page=%v, pages=%v)", foundPageProcSet, foundPagesProcSet)
	}
}

func TestWriter_ProcSetWithImages(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 20, URY: 20},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": {Subtype: "Image", Width: 2, Height: 2, BitsPerComponent: 8, ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceRGB"}, Data: data},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	pageHas := false
	pagesHas := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if ps, ok := resDict.Get(raw.NameLiteral("ProcSet")); ok {
						if arr, ok := ps.(*raw.ArrayObj); ok {
							for _, item := range arr.Items {
								if n, ok := item.(raw.NameObj); ok && n.Value() == "ImageC" {
									pageHas = true
								}
							}
						}
					}
				}
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if ps, ok := resDict.Get(raw.NameLiteral("ProcSet")); ok {
						if arr, ok := ps.(*raw.ArrayObj); ok {
							for _, item := range arr.Items {
								if n, ok := item.(raw.NameObj); ok && n.Value() == "ImageC" {
									pagesHas = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !pageHas || !pagesHas {
		t.Fatalf("image procset missing (page=%v pages=%v)", pageHas, pagesHas)
	}
}

func TestWriter_ExtGStateResources(t *testing.T) {
	lw := 2.5
	stroke := 0.5
	fill := 0.25
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					ExtGStates: map[string]semantic.ExtGState{
						"GS1": {LineWidth: &lw, StrokeAlpha: &stroke, FillAlpha: &fill},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	out := buf.String()
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundPage := false
	foundPages := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if gs, ok := resDict.Get(raw.NameLiteral("ExtGState")); ok {
						if gsDict, ok := gs.(*raw.DictObj); ok {
							if entry, ok := gsDict.Get(raw.NameLiteral("GS1")); ok {
								if _, ok := entry.(*raw.DictObj); ok {
									foundPage = true
								}
							}
						}
					}
				}
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if gs, ok := resDict.Get(raw.NameLiteral("ExtGState")); ok {
						if gsDict, ok := gs.(*raw.DictObj); ok {
							if entry, ok := gsDict.Get(raw.NameLiteral("GS1")); ok {
								if _, ok := entry.(*raw.DictObj); ok {
									foundPages = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !foundPage || !foundPages {
		t.Fatalf("extgstate missing (page=%v pages=%v)", foundPage, foundPages)
	}
	expected := "/ExtGState <</GS1 <</CA 0.500000/LW 2.500000/ca 0.250000>>"
	if strings.Count(out, expected) < 2 {
		t.Fatalf("extgstate values not serialized twice as expected")
	}
}

func TestWriter_ColorSpaceResources(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					ColorSpaces: map[string]semantic.ColorSpace{
						"CS1": &semantic.DeviceColorSpace{Name: "DeviceRGB"},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	pageHas := false
	pagesHas := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if cs, ok := resDict.Get(raw.NameLiteral("ColorSpace")); ok {
						if csDict, ok := cs.(*raw.DictObj); ok {
							if v, ok := csDict.Get(raw.NameLiteral("CS1")); ok {
								if name, ok := v.(raw.NameObj); ok && name.Value() == "DeviceRGB" {
									pageHas = true
								}
							}
						}
					}
				}
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if res, ok := dict.Get(raw.NameLiteral("Resources")); ok {
				if resDict, ok := res.(*raw.DictObj); ok {
					if cs, ok := resDict.Get(raw.NameLiteral("ColorSpace")); ok {
						if csDict, ok := cs.(*raw.DictObj); ok {
							if v, ok := csDict.Get(raw.NameLiteral("CS1")); ok {
								if name, ok := v.(raw.NameObj); ok && name.Value() == "DeviceRGB" {
									pagesHas = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !pageHas || !pagesHas {
		t.Fatalf("color space missing (page=%v pages=%v)", pageHas, pagesHas)
	}
}

func TestWriter_XObjectResources(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 20, URY: 20},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Im1": {Subtype: "Image", Width: 2, Height: 2, BitsPerComponent: 8, ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceGray"}, Data: data},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	pageHas := false
	pagesHas := false
	for ref, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if hasXObjectResource(rawDoc, dict, "Im1") {
				pageHas = true
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if hasXObjectResource(rawDoc, dict, "Im1") {
				pagesHas = true
			}
		}
		_ = ref
	}
	if !pageHas || !pagesHas {
		t.Fatalf("xobject missing (page=%v pages=%v)", pageHas, pagesHas)
	}
}

func TestWriter_FormXObjectResources(t *testing.T) {
	formData := []byte("q 1 0 0 1 0 0 cm BT ET Q")
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 20, URY: 20},
				Resources: &semantic.Resources{
					XObjects: map[string]semantic.XObject{
						"Fm1": {Subtype: "Form", BBox: semantic.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10}, Data: formData},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	var pageDict, pagesDict *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if tval, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := tval.(raw.NameObj); ok && n.Value() == "Page" {
					pageDict = d
				}
				if n, ok := tval.(raw.NameObj); ok && n.Value() == "Pages" {
					pagesDict = d
				}
			}
		}
	}
	if pageDict == nil || pagesDict == nil {
		t.Fatalf("page or pages dict missing")
	}
	if !hasXObjectResource(rawDoc, pageDict, "Fm1") {
		t.Fatalf("form xobject missing on page resources")
	}
	if !hasXObjectResource(rawDoc, pagesDict, "Fm1") {
		t.Fatalf("form xobject missing on pages resources")
	}
	for _, obj := range rawDoc.Objects {
		if s, ok := obj.(*raw.StreamObj); ok {
			if sub, ok := s.Dict.Get(raw.NameLiteral("Subtype")); ok {
				if n, ok := sub.(raw.NameObj); ok && n.Value() == "Form" {
					if _, ok := s.Dict.Get(raw.NameLiteral("BBox")); !ok {
						t.Fatalf("form missing BBox")
					}
				}
			}
		}
	}
}

func TestWriter_PatternResources(t *testing.T) {
	patContent := []byte("0 0 m 1 0 l 1 1 l h f")
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					Patterns: map[string]semantic.Pattern{
						"P1": {
							PatternType: 1,
							PaintType:   1,
							TilingType:  1,
							BBox:        semantic.Rectangle{LLX: 0, LLY: 0, URX: 2, URY: 2},
							XStep:       2,
							YStep:       2,
							Content:     patContent,
						},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	pageHas := false
	pagesHas := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if hasPatternResource(rawDoc, dict, "P1") {
				pageHas = true
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if hasPatternResource(rawDoc, dict, "P1") {
				pagesHas = true
			}
		}
	}
	if !pageHas || !pagesHas {
		t.Fatalf("pattern missing (page=%v pages=%v)", pageHas, pagesHas)
	}
}

func TestWriter_ShadingResources(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 10, URY: 10},
				Resources: &semantic.Resources{
					Shadings: map[string]semantic.Shading{
						"S1": &semantic.FunctionShading{
							BaseShading: semantic.BaseShading{
								Type:       2,
								ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceRGB"},
							},
							Coords: []float64{0, 0, 10, 0},
							Domain: []float64{0, 1},
						},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	pageHas := false
	pagesHas := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Page" {
			if hasShadingResource(rawDoc, dict, "S1") {
				pageHas = true
			}
		}
		if name, ok := typ.(raw.NameObj); ok && name.Value() == "Pages" {
			if hasShadingResource(rawDoc, dict, "S1") {
				pagesHas = true
			}
		}
	}
	if !pageHas || !pagesHas {
		t.Fatalf("shading missing (page=%v pages=%v)", pageHas, pagesHas)
	}
}

func TestWriter_OutputIntents(t *testing.T) {
	profile := []byte{1, 2, 3, 4, 5}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		OutputIntents: []semantic.OutputIntent{{
			S:                         "GTS_PDFA1",
			OutputConditionIdentifier: "sRGB",
			Info:                      "Test profile",
			DestOutputProfile:         profile,
		}},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	var catalog *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if tval, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := tval.(raw.NameObj); ok && n.Value() == "Catalog" {
					catalog = d
				}
			}
		}
	}
	if catalog == nil {
		t.Fatalf("catalog not found")
	}
	oiVal, ok := catalog.Get(raw.NameLiteral("OutputIntents"))
	if !ok {
		t.Fatalf("OutputIntents missing")
	}
	arr, ok := oiVal.(*raw.ArrayObj)
	if !ok || arr.Len() != 1 {
		t.Fatalf("unexpected output intents array: %#v", oiVal)
	}
	refObj, ok := arr.Items[0].(raw.RefObj)
	if !ok {
		t.Fatalf("output intent not ref: %#v", arr.Items[0])
	}
	intentObj, ok := rawDoc.Objects[refObj.Ref()]
	if !ok {
		t.Fatalf("intent ref missing")
	}
	io, ok := intentObj.(*raw.DictObj)
	if !ok {
		t.Fatalf("intent not dict")
	}
	if oc, ok := io.Get(raw.NameLiteral("OutputConditionIdentifier")); !ok {
		t.Fatalf("OCI missing")
	} else if s, ok := oc.(raw.StringObj); !ok || string(s.Value()) != "sRGB" {
		t.Fatalf("OCI mismatch: %#v", oc)
	}
	profVal, ok := io.Get(raw.NameLiteral("DestOutputProfile"))
	if !ok {
		t.Fatalf("DestOutputProfile missing")
	}
	pRef, ok := profVal.(raw.RefObj)
	if !ok {
		t.Fatalf("profile not ref")
	}
	profObj, ok := rawDoc.Objects[pRef.Ref()]
	if !ok {
		t.Fatalf("profile object missing")
	}
	stream, ok := profObj.(*raw.StreamObj)
	if !ok {
		t.Fatalf("profile not stream")
	}
	if !bytes.Equal(stream.Data, profile) {
		t.Fatalf("profile data mismatch")
	}
}

func TestWriter_SerializeOperations(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Contents: []semantic.ContentStream{
					{
						Operations: []semantic.Operation{
							{Operator: "BT"},
							{Operator: "Tf", Operands: []semantic.Operand{semantic.NameOperand{Value: "F1"}, semantic.NumberOperand{Value: 12}}},
							{Operator: "Tj", Operands: []semantic.Operand{semantic.StringOperand{Value: []byte("Hello")}}},
							{Operator: "ET"},
						},
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	found := false
	for _, obj := range rawDoc.Objects {
		stream, ok := obj.(*raw.StreamObj)
		if !ok {
			continue
		}
		if tObj, ok := stream.Dict.Get(raw.NameLiteral("Type")); ok {
			if n, ok := tObj.(raw.NameObj); ok && n.Value() == "XRef" {
				continue
			}
		}
		data := string(stream.Data)
		if strings.Contains(data, "BT\n/F1 12 Tf\n(Hello) Tj\nET\n") {
			found = true
		}
	}
	if !found {
		t.Fatalf("serialized operations not present in content stream")
	}
}

func TestWriter_TrimBox(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 200, URY: 200},
				CropBox:  semantic.Rectangle{URX: 180, URY: 180},
				TrimBox:  semantic.Rectangle{LLX: 10, LLY: 20, URX: 150, URY: 160},
				BleedBox: semantic.Rectangle{LLX: 5, LLY: 5, URX: 170, URY: 170},
				ArtBox:   semantic.Rectangle{LLX: 15, LLY: 15, URX: 140, URY: 140},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundTrim := false
	foundBleed := false
	foundArt := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := dict.Get(raw.NameLiteral("Type"))
		if n, ok := typ.(raw.NameObj); !ok || n.Value() != "Page" {
			continue
		}
		if trim, ok := dict.Get(raw.NameLiteral("TrimBox")); ok {
			if arr, ok := trim.(*raw.ArrayObj); ok && arr.Len() == 4 {
				foundTrim = true
			}
		}
		if bleed, ok := dict.Get(raw.NameLiteral("BleedBox")); ok {
			if arr, ok := bleed.(*raw.ArrayObj); ok && arr.Len() == 4 {
				foundBleed = true
			}
		}
		if art, ok := dict.Get(raw.NameLiteral("ArtBox")); ok {
			if arr, ok := art.(*raw.ArrayObj); ok && arr.Len() == 4 {
				foundArt = true
			}
		}
	}
	if !foundTrim || !foundBleed || !foundArt {
		t.Fatalf("boxes missing (trim=%v, bleed=%v, art=%v)", foundTrim, foundBleed, foundArt)
	}
}

func TestWriter_InfoFields(t *testing.T) {
	info := &semantic.DocumentInfo{
		Title:    "Sample",
		Author:   "Author Name",
		Subject:  "Subject Line",
		Creator:  "creator app",
		Producer: "prod",
		Keywords: []string{"k1", "k2"},
	}
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Info: info,
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	if rawDoc.Metadata.Title != info.Title || rawDoc.Metadata.Author != info.Author || rawDoc.Metadata.Subject != info.Subject || rawDoc.Metadata.Creator != info.Creator || rawDoc.Metadata.Producer != info.Producer {
		t.Fatalf("info fields mismatch: %+v", rawDoc.Metadata)
	}
	if len(rawDoc.Metadata.Keywords) != len(info.Keywords) || rawDoc.Metadata.Keywords[0] != "k1" || rawDoc.Metadata.Keywords[1] != "k2" {
		t.Fatalf("keywords mismatch: %+v", rawDoc.Metadata.Keywords)
	}
}

func TestWriter_IDChangesWithInfo(t *testing.T) {
	makeDoc := func(title, author string) []byte {
		doc := &semantic.Document{
			Pages: []*semantic.Page{
				{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
			},
			Info: &semantic.DocumentInfo{Title: title, Author: author},
		}
		var buf bytes.Buffer
		w := (&WriterBuilder{}).Build()
		if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
			t.Fatalf("write: %v", err)
		}
		return buf.Bytes()
	}
	docA := makeDoc("Title", "AuthorA")
	docB := makeDoc("Title", "AuthorB")

	rawParser := parser.NewDocumentParser(parser.Config{})
	rawA, err := rawParser.Parse(context.Background(), bytes.NewReader(docA))
	if err != nil {
		t.Fatalf("parse A: %v", err)
	}
	rawB, err := rawParser.Parse(context.Background(), bytes.NewReader(docB))
	if err != nil {
		t.Fatalf("parse B: %v", err)
	}
	idA := firstID(rawA)
	idB := firstID(rawB)
	if idA == "" || idB == "" {
		t.Fatalf("missing IDs: %q %q", idA, idB)
	}
	if idA == idB {
		t.Fatalf("expected differing IDs when info changes")
	}
}

func TestWriter_ViewerPreferences(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Info:       &semantic.DocumentInfo{Title: "Show Title"},
		PageLabels: map[int]string{0: "A-", 1: "B-"},
		Outlines: []semantic.OutlineItem{
			{Title: "One", PageIndex: 0},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Catalog" {
				vp, ok := d.Get(raw.NameLiteral("ViewerPreferences"))
				if !ok {
					t.Fatalf("viewer preferences missing")
				}
				vpd, ok := vp.(*raw.DictObj)
				if !ok {
					t.Fatalf("viewer preferences not a dict: %#v", vp)
				}
				ddt, ok := vpd.Get(raw.NameLiteral("DisplayDocTitle"))
				if !ok {
					t.Fatalf("DisplayDocTitle missing")
				}
				if b, ok := ddt.(raw.BoolObj); !ok || !b.Value() {
					t.Fatalf("DisplayDocTitle not true: %#v", ddt)
				}
				if pm, ok := d.Get(raw.NameLiteral("PageMode")); !ok {
					t.Fatalf("PageMode missing")
				} else if n, ok := pm.(raw.NameObj); !ok || n.Value() != "UseOutlines" {
					t.Fatalf("unexpected PageMode: %#v", pm)
				}
				if pl, ok := d.Get(raw.NameLiteral("PageLabels")); ok {
					if plDict, ok := pl.(*raw.DictObj); ok {
						if nums, ok := plDict.Get(raw.NameLiteral("Nums")); ok {
							if arr, ok := nums.(*raw.ArrayObj); !ok || arr.Len() == 0 {
								t.Fatalf("PageLabels Nums missing entries")
							}
						} else {
							t.Fatalf("PageLabels missing Nums")
						}
					} else {
						t.Fatalf("PageLabels not dict: %#v", pl)
					}
				} else {
					t.Fatalf("PageLabels missing")
				}
				return
			}
		}
	}
	t.Fatalf("catalog not found")
}

func TestWriter_OutlinesDest(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Outlines: []semantic.OutlineItem{
			{Title: "First", PageIndex: 0, Children: []semantic.OutlineItem{{Title: "Child", PageIndex: 1}}},
			{Title: "Second", PageIndex: 1},
		},
		StructTree: &semantic.StructureTree{RoleMap: semantic.RoleMap{"H1": "Heading1"}},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	outlinesFound := false
	destOK := 0
	childCountOK := false
	roleMapOK := false
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := d.Get(raw.NameLiteral("Type"))
		if n, ok := typ.(raw.NameObj); ok && n.Value() == "Outlines" {
			outlinesFound = true
		}
		if titleObj, ok := d.Get(raw.NameLiteral("Title")); ok {
			if _, ok := titleObj.(raw.StringObj); ok {
				if dest, ok := d.Get(raw.NameLiteral("Dest")); ok {
					if arr, ok := dest.(*raw.ArrayObj); ok && arr.Len() >= 2 {
						if refObj, ok := arr.Items[0].(raw.RefObj); ok && refObj.Ref().Num > 0 {
							destOK++
						}
					}
				}
				if first, ok := d.Get(raw.NameLiteral("First")); ok {
					if _, ok := first.(raw.RefObj); ok {
						childCountOK = true
					}
				}
			}
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "StructTreeRoot" {
				if rm, ok := d.Get(raw.NameLiteral("RoleMap")); ok {
					if rmd, ok := rm.(*raw.DictObj); ok {
						if mapped, ok := rmd.Get(raw.NameLiteral("H1")); ok {
							if nv, ok := mapped.(raw.NameObj); ok && nv.Value() == "Heading1" {
								roleMapOK = true
							}
						}
					}
				}
			}
		}
	}
	if !outlinesFound {
		t.Fatalf("Outlines dict missing")
	}
	if destOK < 2 {
		t.Fatalf("outline destinations missing: %d", destOK)
	}
	if !childCountOK {
		t.Fatalf("outline child relationships missing")
	}
	if !roleMapOK {
		t.Fatalf("StructTreeRoot or RoleMap not found")
	}
}

func TestWriter_OutlinesXYZDest(t *testing.T) {
	x := 42.0
	y := 100.0
	zoom := 2.0
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Outlines: []semantic.OutlineItem{
			{Title: "Point", PageIndex: 0, Dest: &semantic.OutlineDestination{X: &x, Y: &y, Zoom: &zoom}},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundXYZ := false
	for _, obj := range rawDoc.Objects {
		dict, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		title, ok := dict.Get(raw.NameLiteral("Title"))
		if !ok {
			continue
		}
		if ts, ok := title.(raw.StringObj); !ok || string(ts.Value()) != "Point" {
			continue
		}
		destObj, ok := dict.Get(raw.NameLiteral("Dest"))
		if !ok {
			continue
		}
		arr, ok := destObj.(*raw.ArrayObj)
		if !ok || arr.Len() != 5 {
			continue
		}
		if name, ok := arr.Items[1].(raw.NameObj); ok && name.Value() == "XYZ" {
			if xv, ok := arr.Items[2].(raw.NumberObj); ok {
				if yv, ok := arr.Items[3].(raw.NumberObj); ok {
					if zv, ok := arr.Items[4].(raw.NumberObj); ok && xv.Float() == x && yv.Float() == y && zv.Float() == zoom {
						foundXYZ = true
					}
				}
			}
		}
	}
	if !foundXYZ {
		t.Fatalf("XYZ destination not serialized")
	}
}

func TestWriter_TableTaggingAndParentTree(t *testing.T) {
	bld := builder.NewBuilder()
	rows := []builder.TableRow{
		{Cells: []builder.TableCell{{Text: "Header 1"}, {Text: "Header 2"}}},
	}
	for i := 0; i < 10; i++ {
		rows = append(rows, builder.TableRow{
			Cells: []builder.TableCell{
				{Text: fmt.Sprintf("R%dC1", i)},
				{Text: fmt.Sprintf("R%dC2", i)},
			},
		})
	}
	table := builder.Table{
		Columns:    []float64{80, 80},
		Rows:       rows,
		HeaderRows: 1,
	}
	bld.NewPage(160, 170).DrawTable(table, builder.TableOptions{
		X:             20,
		Y:             150,
		Tagged:        true,
		RepeatHeaders: true,
		BorderWidth:   0.5,
		CellPadding:   3,
		DefaultSize:   10,
		HeaderFill:    builder.Color{R: 0.9, G: 0.9, B: 0.9},
	}).Finish()
	doc, err := bld.Build()
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if len(doc.Pages) < 2 {
		t.Fatalf("expected pagination from table, got %d pages", len(doc.Pages))
	}
	var buf bytes.Buffer
	if err := (&WriterBuilder{}).Build().Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	rawDoc, err := parser.NewDocumentParser(parser.Config{}).Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse written doc: %v", err)
	}
	foundStructRoot := false
	foundParentTree := false
	structParentPages := 0
	foundTableElem := false
	mcidTagged := false

	for _, obj := range rawDoc.Objects {
		switch v := obj.(type) {
		case *raw.DictObj:
			if typ, ok := v.Get(raw.NameLiteral("Type")); ok {
				if name, ok := typ.(raw.NameObj); ok {
					switch name.Value() {
					case "StructTreeRoot":
						foundStructRoot = true
						if _, ok := v.Get(raw.NameLiteral("ParentTree")); ok {
							foundParentTree = true
						}
					case "StructElem":
						if s, ok := v.Get(raw.NameLiteral("S")); ok {
							if n, ok := s.(raw.NameObj); ok && n.Value() == "Table" {
								foundTableElem = true
							}
						}
					case "Page":
						if _, ok := v.Get(raw.NameLiteral("StructParents")); ok {
							structParentPages++
						}
					}
				}
			}
		case *raw.StreamObj:
			if bytes.Contains(v.Data, []byte("MCID")) && bytes.Contains(v.Data, []byte("BDC")) {
				mcidTagged = true
			}
		}
	}
	if !foundStructRoot || !foundParentTree {
		t.Fatalf("structure tree missing: root=%v parentTree=%v", foundStructRoot, foundParentTree)
	}
	if structParentPages < 2 {
		t.Fatalf("expected StructParents on paginated pages, got %d", structParentPages)
	}
	if !foundTableElem {
		t.Fatalf("table struct element not found")
	}
	if !mcidTagged {
		t.Fatalf("expected tagged content with MCIDs")
	}
}

func TestWriter_ComplianceFlags(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Lang:   "en-US",
		Marked: true,
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundLang := false
	foundMark := false
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := typ.(raw.NameObj); ok && n.Value() == "Catalog" {
					if lang, ok := d.Get(raw.NameLiteral("Lang")); ok {
						if s, ok := lang.(raw.StringObj); ok && string(s.Value()) == "en-US" {
							foundLang = true
						}
					}
					if mk, ok := d.Get(raw.NameLiteral("MarkInfo")); ok {
						if md, ok := mk.(*raw.DictObj); ok {
							if marked, ok := md.Get(raw.NameLiteral("Marked")); ok {
								if b, ok := marked.(raw.BoolObj); ok && b.Value() {
									foundMark = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !foundLang {
		t.Fatalf("catalog Lang missing or mismatched")
	}
	if !foundMark {
		t.Fatalf("MarkInfo not set")
	}
}

func TestWriter_ArticleThreads(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
			{MediaBox: semantic.Rectangle{URX: 10, URY: 10}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		Articles: []semantic.ArticleThread{
			{
				Title: "Article One",
				Beads: []semantic.ArticleBead{
					{PageIndex: 0, Rect: semantic.Rectangle{LLX: 0, LLY: 0, URX: 5, URY: 5}},
					{PageIndex: 1, Rect: semantic.Rectangle{LLX: 1, LLY: 1, URX: 6, URY: 6}},
				},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Threads")) {
		t.Fatalf("Threads entry missing from catalog")
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	threadFound := false
	beadCount := 0
	beadLinks := 0
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Thread" {
				threadFound = true
				if _, ok := d.Get(raw.NameLiteral("K")); !ok {
					t.Fatalf("thread missing K")
				}
				if _, ok := d.Get(raw.NameLiteral("T")); !ok {
					t.Fatalf("thread missing title")
				}
			}
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Bead" {
				beadCount++
				if _, ok := d.Get(raw.NameLiteral("P")); !ok {
					t.Fatalf("bead missing page reference")
				}
				if _, ok := d.Get(raw.NameLiteral("R")); !ok {
					t.Fatalf("bead missing rectangle")
				}
				if _, ok := d.Get(raw.NameLiteral("N")); ok {
					beadLinks++
				}
				if _, ok := d.Get(raw.NameLiteral("V")); ok {
					beadLinks++
				}
			}
		}
	}
	if !threadFound {
		t.Fatalf("thread dictionary not found")
	}
	if beadCount != 2 {
		t.Fatalf("expected two beads, got %d", beadCount)
	}
	if beadLinks < 2 {
		t.Fatalf("expected bead links between beads")
	}
}

func TestWriter_LinkAnnotation(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Annotations: []semantic.Annotation{
					&semantic.LinkAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype: "Link",
							RectVal: semantic.Rectangle{LLX: 5, LLY: 5, URX: 50, URY: 50},
						},
						URI: "https://example.com",
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	var annotObj *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			typ, _ := d.Get(raw.NameLiteral("Type"))
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Annot" {
				annotObj = d
				break
			}
		}
	}
	if annotObj == nil {
		t.Fatalf("annotation object not found")
	}
	if sub, ok := annotObj.Get(raw.NameLiteral("Subtype")); !ok {
		t.Fatalf("annotation subtype missing")
	} else if n, ok := sub.(raw.NameObj); !ok || n.Value() != "Link" {
		t.Fatalf("unexpected subtype: %#v", sub)
	}
	action, ok := annotObj.Get(raw.NameLiteral("A"))
	if !ok {
		t.Fatalf("annotation action missing")
	}
	if ad, ok := action.(*raw.DictObj); ok {
		if uri, ok := ad.Get(raw.NameLiteral("URI")); !ok {
			t.Fatalf("URI missing")
		} else if s, ok := uri.(raw.StringObj); !ok || string(s.Value()) != "https://example.com" {
			t.Fatalf("unexpected URI: %#v", uri)
		}
	} else {
		t.Fatalf("action not dict: %#v", action)
	}
}

func TestWriter_TextAnnotationContents(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 50, URY: 50},
				Annotations: []semantic.Annotation{
					&semantic.GenericAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype:  "Text",
							RectVal:  semantic.Rectangle{LLX: 1, LLY: 1, URX: 10, URY: 10},
							Contents: "note here",
						},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	found := false
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := d.Get(raw.NameLiteral("Type"))
		if n, ok := typ.(raw.NameObj); !ok || n.Value() != "Annot" {
			continue
		}
		sub, _ := d.Get(raw.NameLiteral("Subtype"))
		if n, ok := sub.(raw.NameObj); !ok || n.Value() != "Text" {
			continue
		}
		contents, ok := d.Get(raw.NameLiteral("Contents"))
		if !ok {
			t.Fatalf("text annotation missing contents")
		}
		if s, ok := contents.(raw.StringObj); !ok || string(s.Value()) != "note here" {
			t.Fatalf("unexpected contents: %#v", contents)
		}
		found = true
	}
	if !found {
		t.Fatalf("text annotation not found")
	}
}

func TestWriter_AnnotationAppearance(t *testing.T) {
	ap := []byte("q 1 0 0 1 0 0 cm BT (hello) Tj ET Q")
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 20, URY: 20},
				Annotations: []semantic.Annotation{
					&semantic.GenericAnnotation{
						BaseAnnotation: semantic.BaseAnnotation{
							Subtype:    "Text",
							RectVal:    semantic.Rectangle{LLX: 1, LLY: 1, URX: 5, URY: 5},
							Contents:   "note",
							Appearance: ap,
						},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	foundAppearance := false
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		typ, _ := d.Get(raw.NameLiteral("Type"))
		if n, ok := typ.(raw.NameObj); ok && n.Value() == "Annot" {
			apDict, ok := d.Get(raw.NameLiteral("AP"))
			if !ok {
				continue
			}
			if apDictObj, ok := apDict.(*raw.DictObj); ok {
				if nRef, ok := apDictObj.Get(raw.NameLiteral("N")); ok {
					if ref, ok := nRef.(raw.RefObj); ok {
						if stream, ok := rawDoc.Objects[ref.Ref()].(*raw.StreamObj); ok {
							if bytes.Equal(stream.Data, ap) {
								foundAppearance = true
							}
						}
					}
				}
			}
		}
	}
	if !foundAppearance {
		t.Fatalf("appearance stream not found for annotation")
	}
}

func TestWriter_AcroFormNeedAppearances(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 20, URY: 20}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		AcroForm: &semantic.AcroForm{NeedAppearances: true},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	found := false
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if fields, ok := d.Get(raw.NameLiteral("Fields")); ok {
				if arr, ok := fields.(*raw.ArrayObj); ok && arr.Len() == 0 {
					if na, ok := d.Get(raw.NameLiteral("NeedAppearances")); ok {
						if b, ok := na.(raw.BoolObj); ok && b.Value() {
							found = true
							break
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("AcroForm not serialized with NeedAppearances")
	}
}

func TestWriter_AcroFormFields(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 20, URY: 20}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		AcroForm: &semantic.AcroForm{
			Fields: []semantic.FormField{
				&semantic.TextFormField{
					BaseFormField: semantic.BaseFormField{Name: "Field1"},
					Value:         "hello",
				},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	var acro *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if fields, ok := d.Get(raw.NameLiteral("Fields")); ok {
				if _, ok := fields.(*raw.ArrayObj); ok {
					acro = d
					break
				}
			}
		}
	}
	if acro == nil {
		t.Fatalf("acroform not found")
	}
	fieldsVal, _ := acro.Get(raw.NameLiteral("Fields"))
	arr, _ := fieldsVal.(*raw.ArrayObj)
	if arr.Len() != 1 {
		t.Fatalf("expected one field, got %d", arr.Len())
	}
	ref, ok := arr.Items[0].(raw.RefObj)
	if !ok {
		t.Fatalf("field not ref: %#v", arr.Items[0])
	}
	fieldObj, ok := rawDoc.Objects[ref.Ref()]
	if !ok {
		t.Fatalf("field object missing")
	}
	fd, ok := fieldObj.(*raw.DictObj)
	if !ok {
		t.Fatalf("field not dict")
	}
	if ft, ok := fd.Get(raw.NameLiteral("FT")); !ok {
		t.Fatalf("field type missing")
	} else if n, ok := ft.(raw.NameObj); !ok || n.Value() != "Tx" {
		t.Fatalf("field type mismatch: %#v", ft)
	}
	if val, ok := fd.Get(raw.NameLiteral("V")); !ok {
		t.Fatalf("value missing")
	} else if s, ok := val.(raw.StringObj); !ok || string(s.Value()) != "hello" {
		t.Fatalf("value mismatch: %#v", val)
	}
}

func TestWriter_AcroFormWidgetAppearance(t *testing.T) {
	ap := []byte("0 0 m 1 1 l S")
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 20, URY: 20}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
		AcroForm: &semantic.AcroForm{
			Fields: []semantic.FormField{
				&semantic.ButtonFormField{
					BaseFormField: semantic.BaseFormField{
						Name:            "Check",
						PageIndex:       0,
						Rect:            semantic.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10},
						Flags:           1,
						Appearance:      ap,
						AppearanceState: "Yes",
						Border:          []float64{0, 0, 2},
						Color:           []float64{1, 0, 0},
					},
					IsCheck: true,
					Checked: true,
					OnState: "Yes",
				},
			},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	var acro *raw.DictObj
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if fields, ok := d.Get(raw.NameLiteral("Fields")); ok {
				if _, ok := fields.(*raw.ArrayObj); ok {
					acro = d
					break
				}
			}
		}
	}
	if acro == nil {
		t.Fatalf("AcroForm not found")
	}
	fieldsVal, _ := acro.Get(raw.NameLiteral("Fields"))
	arr, _ := fieldsVal.(*raw.ArrayObj)
	if arr.Len() != 1 {
		t.Fatalf("expected one field")
	}
	ref, _ := arr.Items[0].(raw.RefObj)
	fieldObj, _ := rawDoc.Objects[ref.Ref()]
	fd := fieldObj.(*raw.DictObj)
	if sub, ok := fd.Get(raw.NameLiteral("Subtype")); !ok {
		t.Fatalf("widget subtype missing")
	} else if n, ok := sub.(raw.NameObj); !ok || n.Value() != "Widget" {
		t.Fatalf("unexpected widget subtype: %#v", sub)
	}
	if as, ok := fd.Get(raw.NameLiteral("AS")); !ok {
		t.Fatalf("appearance state missing")
	} else if n, ok := as.(raw.NameObj); !ok || n.Value() != "Yes" {
		t.Fatalf("appearance state mismatch: %#v", as)
	}
	if f, ok := fd.Get(raw.NameLiteral("F")); !ok {
		t.Fatalf("widget flags missing")
	} else if num, ok := f.(raw.NumberObj); !ok || num.Int() != 1 {
		t.Fatalf("widget flag mismatch: %#v", f)
	}
	if c, ok := fd.Get(raw.NameLiteral("C")); !ok {
		t.Fatalf("color missing")
	} else if arr, ok := c.(*raw.ArrayObj); !ok || arr.Len() == 0 {
		t.Fatalf("color array missing entries")
	}
	if apDict, ok := fd.Get(raw.NameLiteral("AP")); ok {
		if apD, ok := apDict.(*raw.DictObj); ok {
			if nRef, ok := apD.Get(raw.NameLiteral("N")); ok {
				if refObj, ok := nRef.(raw.RefObj); ok {
					if stream, ok := rawDoc.Objects[refObj.Ref()].(*raw.StreamObj); ok {
						if !bytes.Equal(stream.Data, ap) {
							t.Fatalf("appearance stream mismatch")
						}
					} else {
						t.Fatalf("AP N not stream")
					}
				}
			}
		}
	} else {
		t.Fatalf("appearance dictionary missing")
	}
	pageAnnotFound := false
	for _, obj := range rawDoc.Objects {
		if d, ok := obj.(*raw.DictObj); ok {
			if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
				if n, ok := typ.(raw.NameObj); ok && n.Value() == "Page" {
					if annots, ok := d.Get(raw.NameLiteral("Annots")); ok {
						if arr, ok := annots.(*raw.ArrayObj); ok {
							for _, item := range arr.Items {
								if r, ok := item.(raw.RefObj); ok && r.Ref() == ref.Ref() {
									pageAnnotFound = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !pageAnnotFound {
		t.Fatalf("widget annotation not attached to page")
	}
}

func TestWriter_EncryptsContentStream(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 20, URY: 20}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT (Secret) Tj ET")}}},
		},
		Metadata:          &semantic.XMPMetadata{Raw: []byte("<meta>visible</meta>")},
		Encrypted:         true,
		UserPassword:      "user",
		OwnerPassword:     "owner",
		MetadataEncrypted: false,
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	rawParser := parser.NewDocumentParser(parser.Config{Security: security.NoopHandler()})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}
	encVal, ok := rawDoc.Trailer.Get(raw.NameLiteral("Encrypt"))
	if !ok {
		t.Fatalf("encrypt entry missing")
	}
	encRef, ok := encVal.(raw.RefObj)
	if !ok {
		t.Fatalf("encrypt entry not ref: %#v", encVal)
	}
	encDictObj, ok := rawDoc.Objects[encRef.Ref()].(*raw.DictObj)
	if !ok {
		t.Fatalf("encrypt dictionary not found")
	}
	handler, err := (&security.HandlerBuilder{}).WithEncryptDict(encDictObj).WithTrailer(rawDoc.Trailer).Build()
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	if err := handler.Authenticate("user"); err != nil {
		t.Fatalf("authenticate user: %v", err)
	}
	var contentsRef raw.RefObj
	var metadataRef raw.RefObj
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Page" {
				if cVal, ok := d.Get(raw.NameLiteral("Contents")); ok {
					if r, ok := cVal.(raw.RefObj); ok {
						contentsRef = r
					}
				}
			}
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Catalog" {
				if md, ok := d.Get(raw.NameLiteral("Metadata")); ok {
					if r, ok := md.(raw.RefObj); ok {
						metadataRef = r
					}
				}
			}
		}
	}
	if contentsRef.Ref().Num == 0 {
		t.Fatalf("contents reference not found")
	}
	streamObj, ok := rawDoc.Objects[contentsRef.Ref()].(*raw.StreamObj)
	if !ok {
		t.Fatalf("content stream not found")
	}
	if bytes.Contains(streamObj.Data, []byte("Secret")) {
		t.Fatalf("content stream appears unencrypted")
	}
	dec, err := handler.Decrypt(contentsRef.Ref().Num, contentsRef.Ref().Gen, streamObj.Data, security.DataClassStream)
	if err != nil {
		t.Fatalf("decrypt stream: %v", err)
	}
	if !bytes.Contains(dec, []byte("Secret")) {
		t.Fatalf("decrypted stream missing content: %q", dec)
	}
	if metadataRef.Ref().Num > 0 {
		if mdStream, ok := rawDoc.Objects[metadataRef.Ref()].(*raw.StreamObj); ok {
			if !bytes.Contains(mdStream.Data, []byte("<meta>visible</meta>")) {
				t.Fatalf("metadata stream unexpectedly encrypted")
			}
		}
	}
}

func TestWriter_XRefStreamStartOffset(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{MediaBox: semantic.Rectangle{URX: 50, URY: 50}, Contents: []semantic.ContentStream{{RawBytes: []byte("BT ET")}}},
		},
	}
	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true, XRefStreams: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	data := buf.Bytes()
	off := startXRef(data)
	if off <= 0 || int(off) >= len(data) {
		t.Fatalf("invalid startxref: %d", off)
	}
	snippet := data[off:]
	if !regexp.MustCompile(`^\d+\s+0\s+obj`).Match(snippet) {
		t.Fatalf("startxref does not point to object header")
	}
	window := snippet
	if len(window) > 200 {
		window = window[:200]
	}
	if !bytes.Contains(window, []byte("/Type /XRef")) {
		t.Fatalf("xref stream type missing near startxref")
	}
}

func firstID(doc *raw.Document) string {
	idObj, ok := doc.Trailer.Get(raw.NameLiteral("ID"))
	if !ok {
		return ""
	}
	arr, ok := idObj.(*raw.ArrayObj)
	if !ok || arr.Len() == 0 {
		return ""
	}
	return hex.EncodeToString(idBytes(arr.Items[0]))
}

func idBytes(obj raw.Object) []byte {
	switch v := obj.(type) {
	case raw.StringObj:
		return v.Value()
	case raw.HexStringObj:
		return v.Value()
	default:
		return nil
	}
}

func maxObjNum(data []byte) int {
	re := regexp.MustCompile(`\s(\d+)\s+0\s+obj`)
	matches := re.FindAllSubmatch(data, -1)
	max := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		n, _ := strconv.Atoi(string(m[1]))
		if n > max {
			max = n
		}
	}
	return max
}

func startXRef(data []byte) int64 {
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	m := re.FindSubmatch(data)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return v
}

func parseXRefTableOffsets(data []byte) map[int]int64 {
	off := startXRef(data)
	if off <= 0 || int(off) >= len(data) {
		return nil
	}
	start := data[off:]
	if !bytes.HasPrefix(start, []byte("xref")) {
		return nil
	}
	start = start[len("xref"):]
	start = bytes.TrimLeft(start, "\r\n")
	lines := strings.Split(string(start), "\n")
	if len(lines) < 2 {
		return nil
	}
	offsets := map[int]int64{}
	current := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "trailer") {
			break
		}
		parts := strings.Fields(line)
		if len(parts) == 2 {
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			current = start - 1
			continue
		}
		if len(parts) < 3 {
			continue
		}
		current++
		off, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		offsets[current] = off
	}
	return offsets
}

func hasXObjectResource(doc *raw.Document, dict *raw.DictObj, name string) bool {
	res, ok := dict.Get(raw.NameLiteral("Resources"))
	if !ok {
		return false
	}
	resDict, ok := res.(*raw.DictObj)
	if !ok {
		return false
	}
	xo, ok := resDict.Get(raw.NameLiteral("XObject"))
	if !ok {
		return false
	}
	xoDict, ok := xo.(*raw.DictObj)
	if !ok {
		return false
	}
	entry, ok := xoDict.Get(raw.NameLiteral(name))
	if !ok {
		return false
	}
	ref, ok := entry.(raw.RefObj)
	if !ok {
		return false
	}
	stream, ok := doc.Objects[ref.Ref()]
	if !ok {
		return false
	}
	s, ok := stream.(*raw.StreamObj)
	if !ok {
		return false
	}
	if typ, ok := s.Dict.Get(raw.NameLiteral("Type")); !ok {
		return false
	} else if n, ok := typ.(raw.NameObj); !ok || n.Value() != "XObject" {
		return false
	}
	if sub, ok := s.Dict.Get(raw.NameLiteral("Subtype")); ok {
		if n, ok := sub.(raw.NameObj); ok {
			switch n.Value() {
			case "Image":
				if wVal, ok := s.Dict.Get(raw.NameLiteral("Width")); !ok {
					return false
				} else if wNum, ok := wVal.(raw.NumberObj); !ok || wNum.Int() != 2 {
					return false
				}
				return true
			case "Form":
				return true
			default:
				return false
			}
		}
	}
	return false
}

func hasPatternResource(doc *raw.Document, dict *raw.DictObj, name string) bool {
	res, ok := dict.Get(raw.NameLiteral("Resources"))
	if !ok {
		return false
	}
	resDict, ok := res.(*raw.DictObj)
	if !ok {
		return false
	}
	pat, ok := resDict.Get(raw.NameLiteral("Pattern"))
	if !ok {
		return false
	}
	patDict, ok := pat.(*raw.DictObj)
	if !ok {
		return false
	}
	entry, ok := patDict.Get(raw.NameLiteral(name))
	if !ok {
		return false
	}
	ref, ok := entry.(raw.RefObj)
	if !ok {
		return false
	}
	stream, ok := doc.Objects[ref.Ref()]
	if !ok {
		return false
	}
	s, ok := stream.(*raw.StreamObj)
	if !ok {
		return false
	}
	if typ, ok := s.Dict.Get(raw.NameLiteral("Type")); !ok {
		return false
	} else if n, ok := typ.(raw.NameObj); !ok || n.Value() != "Pattern" {
		return false
	}
	if pt, ok := s.Dict.Get(raw.NameLiteral("PatternType")); ok {
		if n, ok := pt.(raw.NumberObj); !ok || n.Int() != 1 {
			return false
		}
	}
	return true
}

func hasShadingResource(doc *raw.Document, dict *raw.DictObj, name string) bool {
	res, ok := dict.Get(raw.NameLiteral("Resources"))
	if !ok {
		return false
	}
	resDict, ok := res.(*raw.DictObj)
	if !ok {
		return false
	}
	sh, ok := resDict.Get(raw.NameLiteral("Shading"))
	if !ok {
		return false
	}
	shDict, ok := sh.(*raw.DictObj)
	if !ok {
		return false
	}
	entry, ok := shDict.Get(raw.NameLiteral(name))
	if !ok {
		return false
	}
	ref, ok := entry.(raw.RefObj)
	if !ok {
		return false
	}
	obj, ok := doc.Objects[ref.Ref()]
	if !ok {
		return false
	}
	if sDict, ok := obj.(*raw.DictObj); ok {
		if typ, ok := sDict.Get(raw.NameLiteral("ShadingType")); ok {
			if num, ok := typ.(raw.NumberObj); !ok || num.Int() != 2 {
				return false
			}
		}
		return true
	}
	return false
}

func TestWriter_AdvancedColorAndShading(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
				Resources: &semantic.Resources{
					ColorSpaces: map[string]semantic.ColorSpace{
						"CS_Sep": &semantic.SeparationColorSpace{
							Name:          "PantoneOrange",
							Alternate:     &semantic.DeviceColorSpace{Name: "DeviceCMYK"},
							TintTransform: []byte("...function..."),
						},
						"CS_DevN": &semantic.DeviceNColorSpace{
							Names:         []string{"Orange", "Green"},
							Alternate:     &semantic.DeviceColorSpace{Name: "DeviceCMYK"},
							TintTransform: []byte("...function..."),
							Attributes:    &semantic.DeviceNAttributes{Subtype: "DeviceN"},
						},
					},
					Shadings: map[string]semantic.Shading{
						"SH_Mesh": &semantic.MeshShading{
							BaseShading: semantic.BaseShading{
								Type:       4, // Free-form Gouraud-shaded triangle mesh
								ColorSpace: &semantic.DeviceColorSpace{Name: "DeviceRGB"},
							},
							BitsPerCoordinate: 8,
							BitsPerComponent:  8,
							BitsPerFlag:       8,
							Decode:            []float64{0, 100, 0, 100, 0, 1, 0, 1, 0, 1},
							Stream:            []byte("...mesh data..."),
						},
					},
				},
				Contents: []semantic.ContentStream{{RawBytes: []byte("/CS_Sep cs 0.5 scn /SH_Mesh sh")}},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundSep := false
	foundDevN := false
	foundMesh := false

	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		// Check Page Resources
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Page" {
				res, _ := d.Get(raw.NameLiteral("Resources"))
				resDict, _ := res.(*raw.DictObj)

				// Check ColorSpaces
				cs, _ := resDict.Get(raw.NameLiteral("ColorSpace"))
				csDict, _ := cs.(*raw.DictObj)

				if sepObj, ok := csDict.Get(raw.NameLiteral("CS_Sep")); ok {
					if arr, ok := sepObj.(*raw.ArrayObj); ok && arr.Len() == 4 {
						if n, ok := arr.Items[0].(raw.NameObj); ok && n.Value() == "Separation" {
							if n, ok := arr.Items[1].(raw.NameObj); ok && n.Value() == "PantoneOrange" {
								foundSep = true
							}
						}
					}
				}

				if devNObj, ok := csDict.Get(raw.NameLiteral("CS_DevN")); ok {
					if arr, ok := devNObj.(*raw.ArrayObj); ok && arr.Len() >= 4 {
						if n, ok := arr.Items[0].(raw.NameObj); ok && n.Value() == "DeviceN" {
							if names, ok := arr.Items[1].(*raw.ArrayObj); ok && names.Len() == 2 {
								foundDevN = true
							}
						}
					}
				}

				// Check Shading
				sh, _ := resDict.Get(raw.NameLiteral("Shading"))
				shDict, _ := sh.(*raw.DictObj)
				if meshRef, ok := shDict.Get(raw.NameLiteral("SH_Mesh")); ok {
					if ref, ok := meshRef.(raw.RefObj); ok {
						meshObj := rawDoc.Objects[ref.Ref()]
						if stream, ok := meshObj.(*raw.StreamObj); ok {
							if st, ok := stream.Dict.Get(raw.NameLiteral("ShadingType")); ok {
								if n, ok := st.(raw.NumberObj); ok && n.Int() == 4 {
									foundMesh = true
								}
							}
						}
					}
				}
			}
		}
	}

	if !foundSep {
		t.Fatalf("Separation color space not found or malformed")
	}
	if !foundDevN {
		t.Fatalf("DeviceN color space not found or malformed")
	}
	if !foundMesh {
		t.Fatalf("Mesh shading not found or malformed")
	}
}
