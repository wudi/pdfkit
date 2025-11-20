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

func TestEngine_RenderHTML(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	htmlStr := `
<h1>Title</h1>
<h2>Subtitle</h2>
<p>This is a paragraph with some text. It should wrap if it is long enough.</p>
<ul>
	<li>List item 1</li>
	<li>List item 2</li>
</ul>
<p>Another paragraph.</p>
`
	err := engine.RenderHTML(htmlStr)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
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

func TestEngine_RenderMarkdown_Extended(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	md := `
# Header 1

> This is a blockquote.
> It has multiple lines.

Here is a paragraph with **bold** and *italic* text.
And a [link](http://example.com) and an ![image](img.png).
Inline ` + "`code`" + `.

` + "```go" + `
func main() {
    fmt.Println("Hello")
}
` + "```" + `

---

    Indented code block
    Line 2

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

func TestEngine_RenderHTML_Extended(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	htmlStr := `
<h1>Header 1</h1>
<blockquote>
<p>This is a blockquote.</p>
<p>It has multiple lines.</p>
</blockquote>
<p>Here is a paragraph with <b>bold</b> and <i>italic</i> text.</p>
<p>And a <a href="http://example.com">link</a> and an <img src="img.png" alt="image">.</p>
<p>Inline <code>code</code>.</p>
<pre>
func main() {
    fmt.Println("Hello")
}
</pre>
<hr>
<p>Line after hr.</p>
`
	err := engine.RenderHTML(htmlStr)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
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
