package layout

import (
	"testing"
)

func TestRenderMarkdown_Features(t *testing.T) {
	mb := &MockBuilder{}
	engine := NewEngine(mb)

	// Test Markdown with various features
	md := `
# Header 1
## Header 2

Paragraph with **bold** and *italic* text.

- List item 1
- List item 2

` + "```go" + `
func main() {
	fmt.Println("Hello")
}
` + "```" + `

[Link](http://example.com)
`

	err := engine.RenderMarkdown(md)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	if mb.Page == nil {
		t.Fatal("No page created")
	}

	texts := mb.Page.DrawnTexts
	if len(texts) == 0 {
		t.Fatal("No text drawn")
	}

	// Verify Header 1 (should be large font)
	// Note: renderSpans splits text into words, so we look for "Header" with size 24
	foundH1 := false
	for _, dt := range texts {
		if dt.Text == "Header" && dt.Opts.FontSize == engine.DefaultFontSize*2.0 {
			foundH1 = true
			break
		}
	}
	if !foundH1 {
		t.Logf("Drawn texts:")
		for _, dt := range texts {
			t.Logf("  Text: %q, Font: %s, Size: %f", dt.Text, dt.Opts.Font, dt.Opts.FontSize)
		}
		t.Error("Did not find 'Header' with correct font size")
	}

	// Verify Bold text
	foundBold := false
	for _, dt := range texts {
		if dt.Text == "bold" && dt.Opts.Font == "Helvetica-Bold" {
			foundBold = true
			break
		}
	}
	if !foundBold {
		t.Error("Did not find 'bold' text with Helvetica-Bold font")
	}

	// Verify Code block (should be Courier)
	foundCode := false
	for _, dt := range texts {
		// Goldmark might split the code block text, but we look for parts
		if (dt.Text == "func main() {" || dt.Text == "fmt.Println(\"Hello\")") && dt.Opts.Font == "Courier" {
			foundCode = true
			break
		}
	}
	// Note: Goldmark HTML renderer might render code blocks as <pre><code>...</code></pre>
	// My RenderHTML handles <pre> by setting Courier.
	// Let's check if any text is Courier.
	if !foundCode {
		for _, dt := range texts {
			if dt.Opts.Font == "Courier" {
				foundCode = true
				break
			}
		}
	}
	if !foundCode {
		t.Error("Did not find code block text with Courier font")
	}

	// Verify Link
	if len(mb.Page.Annotations) == 0 {
		t.Error("No annotations created for link")
	}
}
