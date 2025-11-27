# Implementation Gaps and TODOs

## Layout Engine (`layout/`)

### HTML Rendering (`layout/html.go`)
- [x] **Image Support**:
    - Current implementation only supports local file paths (`os.Open`). Need to add support for HTTP/HTTPS URLs.
    - Error handling is silent. Should render a placeholder or error text if image loading fails.
    - `imageToSemantic` is inefficient (raw RGB copy). Should support pass-through for JPEG/PNG to reduce PDF size.
- [x] **Table Support**:
    - `rowspan` is not implemented (only `colspan`).
    - Borders and padding are hardcoded (Black, 1pt, 5pt padding). Should respect attributes or styles.
    - No support for nested tables.
- [x] **Form Elements**:
    - Inputs, Textareas, and Selects have hardcoded dimensions (e.g., width=100) and colors.
    - `renderHTMLSelect` defaults to Combo box; List box style is missing.
- [x] **Rich Text & Styling**:
    - `extractSpans` ignores `<u>` (underline), `<s>` (strikethrough), and `<code>`.
    - No support for `style="..."` attributes (CSS).
    - `resolveFont` is hardcoded for Helvetica/Times/Courier.

### Markdown Rendering (`layout/markdown.go`)
- [x] **List Items**:
    - `renderMarkdownListItem` only renders the first child if it's a paragraph or text. It drops complex content (multiple paragraphs, nested lists).
- [x] **Code Blocks**:
    - `renderMarkdownCodeBlock` uses `e.DefaultFont` instead of a monospace font.
    - No line wrapping for long code lines; they will bleed off the page.
- [x] **Rich Text Loss**:
    - `renderMarkdownHeader` and `renderMarkdownListItem` use `extractInlineText` which flattens the content to plain string, losing bold/italic/link formatting.

### Core Layout (`layout/layout.go`)
- [x] **Text Rendering**:
    - `renderSpans` uses `strings.Fields`, which destroys intentional multiple spaces (though HTML often collapses them, `pre-wrap` or non-breaking spaces are lost).
    - No character-level wrapping for words longer than the page width.
    - `getSpaceWidth` is an approximation.

## General
- [x] **Error Handling**: Many functions silently ignore errors (e.g., `strconv.ParseFloat`, `image.Decode`).
    - `image.Decode` errors are now handled with placeholder text.
    - `strconv` errors are intentionally ignored to provide fallback defaults (robust HTML parsing).
- [x] **Configuration**: Hardcoded values for margins, font sizes, and line heights should be configurable via `Engine` options.
