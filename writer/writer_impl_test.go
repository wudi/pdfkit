package writer

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"context"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"encoding/ascii85"
	"encoding/hex"
	"fmt"

	"pdflib/builder"
	"pdflib/ir"
	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
	"pdflib/xref"
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
	reID := regexp.MustCompile(`/ID\s*\[\(\s*([0-9a-f]+)\s*\)\s*\(\s*([0-9a-f]+)\s*\)\]`)
	matches := reID.FindSubmatch(data)
	if len(matches) != 3 || !bytes.Equal(matches[1], matches[2]) {
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
		if strings.HasSuffix(trimmed, ">") {
			trimmed = trimmed[:len(trimmed)-1]
		}
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
	rawParser := parser.NewDocumentParser(parser.Config{})
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

func firstID(doc *raw.Document) string {
	idObj, ok := doc.Trailer.Get(raw.NameLiteral("ID"))
	if !ok {
		return ""
	}
	arr, ok := idObj.(*raw.ArrayObj)
	if !ok || arr.Len() == 0 {
		return ""
	}
	if s, ok := arr.Items[0].(raw.StringObj); ok {
		return string(s.Value())
	}
	return ""
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
