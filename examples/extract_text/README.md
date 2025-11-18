# Extract Text Example

This example walks the PDF page tree produced by the raw parser/decoder pipeline and prints the text shown by `Tj`/`TJ` operators on each page. It is intentionally simple and meant as a starting point for building richer extraction logic.

## Run

1. Produce or pick a PDF. For instance, reuse the invoice sample:
   ```bash
   go run ./examples/invoice invoice.pdf
   ```
2. Extract text:
   ```bash
   go run ./examples/extract_text invoice.pdf
   ```

The program reports each page and a best-effort concatenation of the text show operators it finds.
