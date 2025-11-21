# Extract Command

`cmd/extract` is a utility that exercises the `github.com/wudi/pdfkit/extractor` helpers to pull common artifacts out of PDFs without writing custom code. It runs on top of the existing parser/decoder pipeline, so it benefits from any filter/security updates automatically.

## Usage

```
go run ./cmd/extract [flags] <pdf>
```

Flags:

- `-text` — emit page text extracted from `Tj/TJ` operators.
- `-images` — decode image XObjects and write them under `-out/images`.
- `-annotations` — list per-page annotations, URIs, and colors.
- `-metadata` — dump Info dictionary fields, tagging flags, and XMP bytes (base64 encoded within JSON).
- `-bookmarks` — report the outline tree.
- `-toc` — flatten the table of contents (outline + page labels).
- `-fonts` — list unique fonts plus the pages they appear on.
- `-attachments` — export embedded files into `-out/attachments`.
- `-out` — destination directory for binary artifacts (defaults to `extract_output`).
- `-password` — password to open encrypted PDFs.

If no feature flags are provided, the tool runs all extractors.

## Examples

Extract everything from `testdata/basic.pdf`:

```
go run ./cmd/extract testdata/basic.pdf
```

Extract only annotations and attachments into a custom directory:

```
go run ./cmd/extract -annotations -attachments -out tmp/out testdata/basic.pdf
```

The command prints JSON summaries for each feature and writes binary payloads (images/attachments) to disk for further inspection.
