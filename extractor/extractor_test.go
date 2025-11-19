package extractor

import (
	"fmt"
	"testing"

	"pdflib/ir/decoded"
	"pdflib/ir/raw"
)

func TestExtractor_Features(t *testing.T) {
	dec := buildFixtureDecodedDoc(t)
	ext, err := New(dec)
	if err != nil {
		t.Fatalf("new extractor: %v", err)
	}

	texts, err := ext.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) != 1 || texts[0].Content != "Hello" {
		t.Fatalf("unexpected text result: %+v", texts)
	}

	images, err := ext.ExtractImages()
	if err != nil {
		t.Fatalf("extract images: %v", err)
	}
	if len(images) != 1 || images[0].Width != 1 || images[0].Height != 1 {
		t.Fatalf("unexpected image result: %+v", images)
	}

	annots, err := ext.ExtractAnnotations()
	if err != nil {
		t.Fatalf("extract annotations: %v", err)
	}
	if len(annots) != 1 || annots[0].URI != "https://example.com" {
		t.Fatalf("unexpected annotation result: %+v", annots)
	}

	meta := ext.ExtractMetadata()
	if meta.Lang != "en-US" || !meta.Marked || meta.Info.Title != "Fixture" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	bookmarks := ext.ExtractBookmarks()
	if len(bookmarks) != 1 || bookmarks[0].Title != "Intro" || bookmarks[0].Page != 0 {
		t.Fatalf("unexpected bookmarks: %+v", bookmarks)
	}

	toc := ext.ExtractTableOfContents()
	if len(toc) != 1 || toc[0].Label != "A-1" {
		t.Fatalf("unexpected toc: %+v", toc)
	}

	fonts := ext.ExtractFonts()
	if len(fonts) != 1 || fonts[0].BaseFont != "Helvetica" {
		t.Fatalf("unexpected fonts: %+v", fonts)
	}

	files := ext.ExtractEmbeddedFiles()
	if len(files) != 1 || files[0].Name != "attachment.txt" || string(files[0].Data) != "embedded" {
		t.Fatalf("unexpected embedded files: %+v", files)
	}
}

func TestExtractor_ObjectStreamOutlines(t *testing.T) {
	dec := buildObjStreamOutlinesDoc(t)
	ext, err := New(dec)
	if err != nil {
		t.Fatalf("new extractor: %v", err)
	}
	toc := ext.ExtractTableOfContents()
	if len(toc) != 1 || toc[0].Title != "ObjStream" || toc[0].Page != 0 {
		t.Fatalf("unexpected toc from object stream: %+v", toc)
	}
}

func buildFixtureDecodedDoc(t *testing.T) *decoded.DecodedDocument {
	t.Helper()

	root := raw.Dict()
	pages := raw.Dict()
	page := raw.Dict()
	contentsDict := raw.Dict()
	contentStream := raw.NewStream(contentsDict, []byte("BT (Hello) Tj ET"))

	imgDict := raw.Dict()
	imgDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Image"))
	imgDict.Set(raw.NameLiteral("Width"), raw.NumberInt(1))
	imgDict.Set(raw.NameLiteral("Height"), raw.NumberInt(1))
	imgDict.Set(raw.NameLiteral("BitsPerComponent"), raw.NumberInt(8))
	imgDict.Set(raw.NameLiteral("ColorSpace"), raw.NameLiteral("DeviceGray"))
	imgStream := raw.NewStream(imgDict, []byte{0xff})

	fontDict := raw.Dict()
	fontDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("Font"))
	fontDict.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Type1"))
	fontDict.Set(raw.NameLiteral("BaseFont"), raw.NameLiteral("Helvetica"))
	fontDict.Set(raw.NameLiteral("Encoding"), raw.NameLiteral("WinAnsiEncoding"))

	annot := raw.Dict()
	annot.Set(raw.NameLiteral("Subtype"), raw.NameLiteral("Link"))
	annot.Set(raw.NameLiteral("Rect"), raw.NewArray(raw.NumberInt(0), raw.NumberInt(0), raw.NumberInt(10), raw.NumberInt(10)))
	annot.Set(raw.NameLiteral("Contents"), raw.Str([]byte("see site")))
	action := raw.Dict()
	action.Set(raw.NameLiteral("S"), raw.NameLiteral("URI"))
	action.Set(raw.NameLiteral("URI"), raw.Str([]byte("https://example.com")))
	annot.Set(raw.NameLiteral("A"), action)

	outlines := raw.Dict()
	outlineItem := raw.Dict()
	outlineItem.Set(raw.NameLiteral("Title"), raw.Str([]byte("Intro")))
	outlineItem.Set(raw.NameLiteral("Dest"), raw.NewArray(raw.Ref(3, 0), raw.NameLiteral("Fit")))
	outlines.Set(raw.NameLiteral("First"), raw.Ref(9, 0))
	outlines.Set(raw.NameLiteral("Last"), raw.Ref(9, 0))
	outlines.Set(raw.NameLiteral("Count"), raw.NumberInt(1))

	metadataStream := raw.NewStream(raw.Dict(), []byte("<x:xmpmeta/>"))
	embeddedStream := raw.NewStream(raw.Dict(), []byte("embedded"))

	fileSpec := raw.Dict()
	fileSpec.Set(raw.NameLiteral("Type"), raw.NameLiteral("Filespec"))
	fileSpec.Set(raw.NameLiteral("F"), raw.Str([]byte("attachment.txt")))
	fileSpec.Set(raw.NameLiteral("Desc"), raw.Str([]byte("fixture")))
	fileSpec.Set(raw.NameLiteral("AFRelationship"), raw.NameLiteral("Data"))
	efDict := raw.Dict()
	efDict.Set(raw.NameLiteral("F"), raw.Ref(11, 0))
	fileSpec.Set(raw.NameLiteral("EF"), efDict)

	namesDict := raw.Dict()
	embeddedDict := raw.Dict()
	embeddedDict.Set(raw.NameLiteral("Names"), raw.NewArray(raw.Str([]byte("attachment.txt")), raw.Ref(12, 0)))
	namesDict.Set(raw.NameLiteral("EmbeddedFiles"), embeddedDict)

	markInfo := raw.Dict()
	markInfo.Set(raw.NameLiteral("Marked"), raw.Bool(true))

	pageLabels := raw.Dict()
	labelEntry := raw.Dict()
	labelEntry.Set(raw.NameLiteral("P"), raw.Str([]byte("A-")))
	pageLabels.Set(raw.NameLiteral("Nums"), raw.NewArray(raw.NumberInt(0), labelEntry))

	root.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	root.Set(raw.NameLiteral("Pages"), raw.Ref(2, 0))
	root.Set(raw.NameLiteral("Outlines"), raw.Ref(8, 0))
	root.Set(raw.NameLiteral("Lang"), raw.Str([]byte("en-US")))
	root.Set(raw.NameLiteral("MarkInfo"), markInfo)
	root.Set(raw.NameLiteral("PageLabels"), pageLabels)
	root.Set(raw.NameLiteral("Metadata"), raw.Ref(10, 0))
	root.Set(raw.NameLiteral("Names"), namesDict)

	pages.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pages.Set(raw.NameLiteral("Kids"), raw.NewArray(raw.Ref(3, 0)))
	pages.Set(raw.NameLiteral("Count"), raw.NumberInt(1))

	resources := raw.Dict()
	fontsDict := raw.Dict()
	fontsDict.Set(raw.NameLiteral("F1"), raw.Ref(6, 0))
	resources.Set(raw.NameLiteral("Font"), fontsDict)
	xobjects := raw.Dict()
	xobjects.Set(raw.NameLiteral("Im0"), raw.Ref(5, 0))
	resources.Set(raw.NameLiteral("XObject"), xobjects)

	page.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
	page.Set(raw.NameLiteral("Parent"), raw.Ref(2, 0))
	page.Set(raw.NameLiteral("Contents"), raw.Ref(4, 0))
	page.Set(raw.NameLiteral("Resources"), resources)
	page.Set(raw.NameLiteral("Annots"), raw.NewArray(raw.Ref(7, 0)))

	doc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}:  root,
			{Num: 2, Gen: 0}:  pages,
			{Num: 3, Gen: 0}:  page,
			{Num: 4, Gen: 0}:  contentStream,
			{Num: 5, Gen: 0}:  imgStream,
			{Num: 6, Gen: 0}:  fontDict,
			{Num: 7, Gen: 0}:  annot,
			{Num: 8, Gen: 0}:  outlines,
			{Num: 9, Gen: 0}:  outlineItem,
			{Num: 10, Gen: 0}: metadataStream,
			{Num: 11, Gen: 0}: embeddedStream,
			{Num: 12, Gen: 0}: fileSpec,
		},
		Trailer:     raw.Dict(),
		Version:     "1.7",
		Metadata:    raw.DocumentMetadata{Title: "Fixture", Author: "Tester"},
		Permissions: raw.Permissions{Copy: true},
	}
	doc.Trailer.Set(raw.NameLiteral("Root"), raw.Ref(1, 0))

	dec := &decoded.DecodedDocument{
		Raw: doc,
		Streams: map[raw.ObjectRef]decoded.Stream{
			{Num: 4, Gen: 0}:  testStream{stream: contentStream},
			{Num: 5, Gen: 0}:  testStream{stream: imgStream},
			{Num: 10, Gen: 0}: testStream{stream: metadataStream},
			{Num: 11, Gen: 0}: testStream{stream: embeddedStream},
		},
		Perms:             doc.Permissions,
		Encrypted:         doc.Encrypted,
		MetadataEncrypted: doc.MetadataEncrypted,
	}
	return dec
}

func buildObjStreamOutlinesDoc(t *testing.T) *decoded.DecodedDocument {
	t.Helper()

	root := raw.Dict()
	pages := raw.Dict()
	page := raw.Dict()
	contents := raw.NewStream(raw.Dict(), []byte("BT ET"))

	root.Set(raw.NameLiteral("Type"), raw.NameLiteral("Catalog"))
	root.Set(raw.NameLiteral("Pages"), raw.Ref(2, 0))
	root.Set(raw.NameLiteral("Outlines"), raw.Ref(5, 0))

	pages.Set(raw.NameLiteral("Type"), raw.NameLiteral("Pages"))
	pages.Set(raw.NameLiteral("Kids"), raw.NewArray(raw.Ref(3, 0)))
	pages.Set(raw.NameLiteral("Count"), raw.NumberInt(1))

	page.Set(raw.NameLiteral("Type"), raw.NameLiteral("Page"))
	page.Set(raw.NameLiteral("Parent"), raw.Ref(2, 0))
	page.Set(raw.NameLiteral("Contents"), raw.Ref(4, 0))

	obj5 := []byte("<< /Type /Outlines /First 6 0 R /Last 6 0 R /Count 1 >>")
	obj6 := []byte("<< /Title (ObjStream) /Dest [3 0 R /Fit] >>")
	body := append(append([]byte{}, obj5...), '\n')
	obj6Offset := len(body)
	body = append(body, obj6...)
	header := []byte(fmt.Sprintf("5 0 6 %d ", obj6Offset))
	data := append(header, body...)
	objstmDict := raw.Dict()
	objstmDict.Set(raw.NameLiteral("Type"), raw.NameLiteral("ObjStm"))
	objstmDict.Set(raw.NameLiteral("N"), raw.NumberInt(2))
	objstmDict.Set(raw.NameLiteral("First"), raw.NumberInt(int64(len(header))))
	objstm := raw.NewStream(objstmDict, data)

	doc := &raw.Document{
		Objects: map[raw.ObjectRef]raw.Object{
			{Num: 1, Gen: 0}:  root,
			{Num: 2, Gen: 0}:  pages,
			{Num: 3, Gen: 0}:  page,
			{Num: 4, Gen: 0}:  contents,
			{Num: 10, Gen: 0}: objstm,
		},
		Trailer: raw.Dict(),
	}
	doc.Trailer.Set(raw.NameLiteral("Root"), raw.Ref(1, 0))

	streams := map[raw.ObjectRef]decoded.Stream{
		{Num: 4, Gen: 0}:  testStream{stream: contents},
		{Num: 10, Gen: 0}: testStream{stream: objstm},
	}

	return &decoded.DecodedDocument{Raw: doc, Streams: streams}
}

type testStream struct {
	stream raw.Stream
}

func (t testStream) Raw() raw.Object            { return t.stream }
func (t testStream) Type() string               { return t.stream.Type() }
func (t testStream) Dictionary() raw.Dictionary { return t.stream.Dictionary() }
func (t testStream) Data() []byte               { return append([]byte(nil), t.stream.RawData()...) }
func (t testStream) Filters() []string          { return nil }
