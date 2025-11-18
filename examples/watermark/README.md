# Watermark Example

This sample shows how to layer a translucent diagonal text watermark on top of pages that were produced with the fluent `builder` API. It demonstrates:

- mixing high-level drawing helpers with low-level `semantic.Operation` sequences
- adding an `ExtGState` entry for transparency control
- reusing the page resources to keep the watermark logic simple

## Run It

```bash
go run ./examples/watermark [output.pdf]
```

The program defaults to `watermark.pdf` in the current directory.
