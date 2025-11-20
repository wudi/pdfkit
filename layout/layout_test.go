package layout

import (
	"testing"

	"pdflib/builder"
)

func TestEngine_RenderMarkdown(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	md := `# Title
## Subtitle

This is a paragraph with some text. It should wrap if it is long enough.

- List item 1
- List item 2

Another paragraph.
`
	err := engine.RenderMarkdown(md)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least one page")
	}
	
	// Basic verification of content presence
	page := doc.Pages[0]
	if len(page.Contents) == 0 {
		t.Fatal("Expected content stream")
	}
}
