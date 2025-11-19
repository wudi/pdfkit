# Extract Text Example

This example wires the high-level `extractor` package into a tiny CLI and prints the consolidated text discovered on each page. Because it leverages the shared extraction pipeline, it automatically benefits from ToUnicode maps, embedded font decoding, and any future improvements made to the core library.

## Run

1. Produce or pick a PDF. For instance, reuse the invoice sample:
   ```bash
   go run ./examples/invoice invoice.pdf
   ```
2. Extract text:
   ```bash
   go run ./examples/extract_text invoice.pdf
   ```

The program reports each page (with labels when available) and a best-effort concatenation of the text that the extractor resolves.
