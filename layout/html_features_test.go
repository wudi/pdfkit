package layout

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/contentstream"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/security"
)

// --- Mocks ---

type MockBuilder struct {
	Page *MockPageBuilder
}

func (m *MockBuilder) NewPage(width, height float64) builder.PageBuilder {
	if m.Page == nil {
		m.Page = &MockPageBuilder{}
	}
	return m.Page
}

func (m *MockBuilder) MeasureText(text string, fontSize float64, fontName string) float64 {
	return float64(len(text)) * fontSize * 0.5
}

// Stubs for other methods
func (m *MockBuilder) AddPage(page *semantic.Page) builder.PDFBuilder               { return m }
func (m *MockBuilder) SetInfo(info *semantic.DocumentInfo) builder.PDFBuilder       { return m }
func (m *MockBuilder) SetMetadata(xmp []byte) builder.PDFBuilder                    { return m }
func (m *MockBuilder) SetLanguage(lang string) builder.PDFBuilder                   { return m }
func (m *MockBuilder) SetMarked(marked bool) builder.PDFBuilder                     { return m }
func (m *MockBuilder) AddPageLabel(pageIndex int, prefix string) builder.PDFBuilder { return m }
func (m *MockBuilder) AddOutline(out builder.Outline) builder.PDFBuilder            { return m }
func (m *MockBuilder) SetEncryption(ownerPassword, userPassword string, perms raw.Permissions, encryptMetadata bool) builder.PDFBuilder {
	return m
}
func (m *MockBuilder) SetEncryptionWithOptions(ownerPassword, userPassword string, perms raw.Permissions, encryptMetadata bool, opts security.EncryptionOptions) builder.PDFBuilder {
	return m
}
func (m *MockBuilder) RegisterFont(name string, font *semantic.Font) builder.PDFBuilder   { return m }
func (m *MockBuilder) RegisterTrueTypeFont(name string, data []byte) builder.PDFBuilder   { return m }
func (m *MockBuilder) AddEmbeddedFile(file semantic.EmbeddedFile) builder.PDFBuilder      { return m }
func (m *MockBuilder) SetCalculationOrder(fields []semantic.FormField) builder.PDFBuilder { return m }
func (m *MockBuilder) Form() builder.FormBuilder                                          { return nil }
func (m *MockBuilder) Build() (*semantic.Document, error)                                 { return &semantic.Document{}, nil }

type MockPageBuilder struct {
	DrawnTexts  []DrawnText
	DrawnImages []DrawnImage
	DrawnTables []DrawnTable
	Annotations []semantic.Annotation
	Finished    bool
}

type DrawnText struct {
	Text string
	X, Y float64
	Opts builder.TextOptions
}

type DrawnImage struct {
	Img        *semantic.Image
	X, Y, W, H float64
	Opts       builder.ImageOptions
}

type DrawnTable struct {
	Table builder.Table
	Opts  builder.TableOptions
}

func (m *MockPageBuilder) DrawText(text string, x, y float64, opts builder.TextOptions) builder.PageBuilder {
	m.DrawnTexts = append(m.DrawnTexts, DrawnText{Text: text, X: x, Y: y, Opts: opts})
	return m
}

func (m *MockPageBuilder) DrawImage(img *semantic.Image, x, y, width, height float64, opts builder.ImageOptions) builder.PageBuilder {
	m.DrawnImages = append(m.DrawnImages, DrawnImage{Img: img, X: x, Y: y, W: width, H: height, Opts: opts})
	return m
}

func (m *MockPageBuilder) DrawTable(table builder.Table, opts builder.TableOptions) builder.PageBuilder {
	m.DrawnTables = append(m.DrawnTables, DrawnTable{Table: table, Opts: opts})
	// Simulate table height for layout
	if opts.FinalY != nil {
		*opts.FinalY = opts.Y - 100 // Arbitrary height
	}
	return m
}

func (m *MockPageBuilder) AddAnnotation(ann semantic.Annotation) builder.PageBuilder {
	m.Annotations = append(m.Annotations, ann)
	return m
}

func (m *MockPageBuilder) Finish() builder.PDFBuilder {
	m.Finished = true
	return &MockBuilder{Page: m}
}

// Stubs for other methods
func (m *MockPageBuilder) DrawPath(path *contentstream.Path, opts builder.PathOptions) builder.PageBuilder {
	return m
}
func (m *MockPageBuilder) DrawRectangle(x, y, width, height float64, opts builder.RectOptions) builder.PageBuilder {
	return m
}
func (m *MockPageBuilder) DrawLine(x1, y1, x2, y2 float64, opts builder.LineOptions) builder.PageBuilder {
	return m
}
func (m *MockPageBuilder) AddFormField(field semantic.FormField) builder.PageBuilder { return m }
func (m *MockPageBuilder) SetMediaBox(box semantic.Rectangle) builder.PageBuilder    { return m }
func (m *MockPageBuilder) SetCropBox(box semantic.Rectangle) builder.PageBuilder     { return m }
func (m *MockPageBuilder) SetRotation(degrees int) builder.PageBuilder               { return m }

// --- Tests ---

func TestRenderHTML_RichText(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `<p>Normal <b>Bold</b> <i>Italic</i> <strong>Strong</strong> <em>Em</em></p>`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	texts := mb.Page.DrawnTexts
	if len(texts) == 0 {
		t.Fatal("No text drawn")
	}

	// Check for Bold
	foundBold := false
	for _, dt := range texts {
		if dt.Text == "Bold" && dt.Opts.Font == "Helvetica-Bold" {
			foundBold = true
		}
	}
	if !foundBold {
		t.Error("Did not find 'Bold' text with Helvetica-Bold font")
	}

	// Check for Italic
	foundItalic := false
	for _, dt := range texts {
		if dt.Text == "Italic" && dt.Opts.Font == "Helvetica-Oblique" {
			foundItalic = true
		}
	}
	if !foundItalic {
		t.Error("Did not find 'Italic' text with Helvetica-Oblique font")
	}
}

func TestRenderHTML_Links(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `<p>Click <a href="http://example.com">here</a>.</p>`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	// Check annotations
	if len(mb.Page.Annotations) == 0 {
		t.Fatal("No annotations created")
	}

	linkAnn, ok := mb.Page.Annotations[0].(*semantic.LinkAnnotation)
	if !ok {
		t.Fatalf("Expected LinkAnnotation, got %T", mb.Page.Annotations[0])
	}

	uriAction, ok := linkAnn.Action.(semantic.URIAction)
	if !ok {
		t.Fatalf("Expected URIAction, got %T", linkAnn.Action)
	}

	if uriAction.URI != "http://example.com" {
		t.Errorf("Expected URI 'http://example.com', got '%s'", uriAction.URI)
	}
}

func TestRenderHTML_Image(t *testing.T) {
	// Create temp image
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	createTestImage(t, imgPath)

	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `<img src="` + imgPath + `" width="100" height="50">`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	if len(mb.Page.DrawnImages) == 0 {
		t.Fatal("No images drawn")
	}

	img := mb.Page.DrawnImages[0]
	if img.W != 100 || img.H != 50 {
		t.Errorf("Expected image size 100x50, got %vx%v", img.W, img.H)
	}
}

func TestRenderHTML_Table(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `
	<table>
		<thead>
			<tr><th>Header 1</th><th>Header 2</th></tr>
		</thead>
		<tbody>
			<tr><td>Cell 1</td><td>Cell 2</td></tr>
		</tbody>
	</table>
	`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	if len(mb.Page.DrawnTables) == 0 {
		t.Fatal("No tables drawn")
	}

	tbl := mb.Page.DrawnTables[0].Table
	if len(tbl.Rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(tbl.Rows))
	}
	if tbl.HeaderRows != 1 {
		t.Errorf("Expected 1 header row, got %d", tbl.HeaderRows)
	}
	if len(tbl.Rows[0].Cells) != 2 {
		t.Errorf("Expected 2 cells in header, got %d", len(tbl.Rows[0].Cells))
	}
	if tbl.Rows[0].Cells[0].Text != "Header 1" {
		t.Errorf("Expected 'Header 1', got '%s'", tbl.Rows[0].Cells[0].Text)
	}
}

func TestRenderHTML_OrderedList(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `
	<ol>
		<li>Item 1</li>
		<li>Item 2</li>
	</ol>
	`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	texts := mb.Page.DrawnTexts
	found1 := false
	found2 := false

	for _, dt := range texts {
		if dt.Text == "1." {
			found1 = true
		}
		if dt.Text == "2." {
			found2 = true
		}
	}

	if !found1 {
		t.Error("Did not find '1.' marker")
	}
	if !found2 {
		t.Error("Did not find '2.' marker")
	}
}

func TestRenderHTML_Pre(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	html := `<pre>code block</pre>`
	err := engine.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	texts := mb.Page.DrawnTexts
	foundCode := false
	for _, dt := range texts {
		if dt.Text == "code block" && dt.Opts.Font == "Courier" {
			foundCode = true
		}
	}

	if !foundCode {
		t.Error("Did not find code block with Courier font")
	}
}

func createTestImage(t *testing.T, path string) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create image file: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("Failed to encode image: %v", err)
	}
}
