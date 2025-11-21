# PDFKit

> **⚠️ Warning: Experimental Phase**
>
> This library is currently in an experimental phase. APIs are subject to change without notice, and features may not be fully stable. Use with caution in production environments.

PDFKit is a comprehensive, pure Go library for PDF manipulation, designed to support the full spectrum of PDF standards from PDF 1.0 to PDF 2.0, including PDF/A, PDF/X, PDF/UA, and PDF/VT.

## Features

### Core Syntax & Structure
- **Full Version Support**: Read and write PDF 1.0 through PDF 2.0.
- **Advanced Structures**: Object streams, cross-reference streams, hybrid references, and linearization (Fast Web View).
- **Incremental Updates**: Modify files without invalidating existing signatures.

### Graphics & Imaging
- **Color Spaces**: DeviceGray, DeviceRGB, DeviceCMYK, Lab, ICCBased, Separation, DeviceN, and NChannel.
- **Patterns & Shading**: Tiling patterns and all shading types (1-7), including tensor-product patch meshes.
- **Images**: Full support for image dictionaries, masks, soft masks (SMask), and inline images.
- **Transparency**: All 16 blend modes, transparency groups (isolated/knockout), and soft masks.

### Fonts & Text
- **Format Support**: TrueType, Type 1, Type 3, OpenType (CFF/TTF), and CID-Keyed fonts.
- **Unicode**: Full ToUnicode CMap support for text extraction.

### Interactivity & Forms
- **Annotations**: Full support for all standard annotation types (Text, Link, Widget, 3D, Redact, etc.).
- **Forms**: AcroForms, XFA (XML Forms Architecture), and HTML Forms.
- **Actions**: JavaScript, GoTo, Launch, RichMedia, and more.

### Compliance & Security
- **PDF/A**: Support for PDF/A-1, A-2, A-3, and A-4 (Archiving).
- **PDF/X**: Support for PDF/X (Graphics Exchange).
- **PDF/UA**: Support for PDF/UA (Universal Accessibility).
- **Encryption**: RC4 (40/128), AES (128/256).
- **Signatures**: PKCS#7, PAdES, LTV (Long Term Validation).

## Installation

```bash
go get github.com/wudi/pdfkit
```

## Usage

Check the `examples/` directory for usage examples, including:
- Merging and splitting PDFs
- Extracting text and images
- Adding watermarks
- Creating invoices
- Handling CJK fonts

## Testing

Run the full test suite:

```bash
go test ./...
```

To run tests with native OpenJPEG support (requires `libopenjp2`):

```bash
go test ./... -tags openjpeg
```
