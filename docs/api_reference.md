# API Reference

This document provides an overview of the core packages in PDFKit.

## Core Packages

### `builder`
High-level fluent API for creating PDF documents.
- **Key Types**: `Builder`, `PageBuilder`
- **Usage**: Use this package to programmatically generate PDFs.

### `parser`
Responsible for parsing PDF files into the Intermediate Representation (IR).
- **Key Types**: `Parser`, `Config`
- **Usage**: Use this package to read existing PDFs.

### `writer`
Handles serialization of the IR back to PDF file format.
- **Key Types**: `Writer`, `Config`
- **Usage**: Use this package to save documents to disk or stream.

### `ir` (Intermediate Representation)
The core data structures representing the PDF.
- **`ir/raw`**: Low-level PDF objects (Dictionaries, Arrays, Streams).
- **`ir/decoded`**: Objects with streams decompressed and decrypted.
- **`ir/semantic`**: High-level semantic objects (Pages, Fonts, Annotations).

## Support Packages

### `compliance`
Unified compliance engine for PDF/A, PDF/X, and PDF/UA.
- **Subpackages**: `pdfa`, `pdfua`, `pdfvt`, `pdfx`

### `contentstream`
Parses and processes PDF content streams (drawing operators).
- **Key Types**: `Processor`, `GraphicsState`

### `filters`
Implements standard PDF stream filters (Flate, DCT, JPX, etc.).
- **Key Types**: `Decoder`, `Pipeline`

### `fonts`
Handles font parsing, subsetting, and embedding.
- **Key Types**: `SubsettingPipeline`, `GlyphAnalyzer`

### `security`
Manages encryption, decryption, and digital signatures.
- **Key Types**: `Handler`, `Permissions`

### `xref`
Resolves cross-reference tables and streams.
- **Key Types**: `Table`, `Resolver`

## Extension Packages

### `extensions`
Plugin system for inspecting, sanitizing, transforming, and validating PDFs.
- **Key Types**: `Hub`, `Extension`

### `layout`
Layout engine for converting Markdown and HTML to PDF.
- **Key Types**: `Engine`

### `scripting`
JavaScript execution environment for PDF forms and actions.
- **Key Types**: `Engine`
