# Implementation Plan

This document tracks the detailed implementation progress of features defined in `goal-features.md`.

## 1. Core Syntax & File Structure
- [x] **Header**: Support `%PDF-1.0` to `%PDF-2.0` all version headers.
- [x] **Trailer Dictionary**: Root, Encrypt, Info, ID, Previous (for incremental updates).
- [x] **Cross-Reference Table (xref)**:
    - [x] Classic xref table (plain text table).
    - [x] **XRef Streams**: Compressed cross-reference streams (PDF 1.5+).
    - [x] **Hybrid Reference**: Mixed use of traditional table and streams (common in transitional files).
- [x] **Incremental Updates**: Ability to read and write modifications appended to the end of the file, keeping original signatures valid.
- [x] **Linearization (Fast Web View)**: Support Hint Tables, allowing byte-streaming loading.
- [x] **Object Streams**: Parse and generate compressed object streams (ObjStm).

## 2. Basic Objects (Cos Objects)
- [x] **Primitives**: Null, Boolean, Integer, Real.
- [x] **Strings**:
    - [x] Literal Strings (including octal escapes `\ddd` and balanced parentheses).
    - [x] Hex Strings (hexadecimal `<...>`).
    - [x] UTF-16BE Strings (starting with BOM).
- [x] **Names**: Correctly handle escape sequences after `/` (e.g., `#20`).
- [x] **Arrays & Dictionaries**.
- [x] **Streams**: Handle `Length` as an indirect object, handle external file references (F key).

## 3. Filters & Compression
- [x] **FlateDecode**: (Zlib/Deflate) Support Predictor functions (PNG predictors).
- [x] **LZWDecode**: Support EarlyChange parameter.
- [x] **ASCII85Decode** & **ASCIIHexDecode**.
- [x] **RunLengthDecode**.
- [x] **CCITTFaxDecode**: Group 3 (1D/2D) and Group 4.
- [x] **JBIG2Decode**: Handle embedded Global Segments.
- [x] **DCTDecode**: JPEG image processing.
- [x] **JPXDecode**: JPEG 2000 (PDF 1.5+).
- [x] **Crypt**: Filter specifically for decrypting streams.

## 4. Graphics & Imaging
- [x] **Color Spaces**:
    - [x] Implement `cmm` package (ICC Profile parsing).
	- [x] **CMM Transform**: Implement actual color conversion logic (Basic Gray/RGB/CMYK support added).
	- [x] Support `DeviceN` and `Separation` (Spot Colors).
	- [x] Support `Pattern` color space.
- [x] **Patterns & Shading**:
    - [x] Tiling Patterns (Type 1).
    - [x] Shading Patterns (Type 1-7).
- [x] **Images**:
    - [x] Image Dictionary (Basic).
    - [x] SMask (Soft Mask).
    - [x] Inline Images.
- [x] **Transparency**:
    - [x] Blend Modes (All 16).
    - [x] Transparency Groups (Isolated, Knockout).

## 5. Fonts & Text
- [x] **TrueType**: Parsing and extraction.
- [x] **Type 1**: Parsing .pfb/.pfm (PFB parsing with metrics extraction implemented).
    - [x] Handle Length1/Length2/Length3 for embedding.
- [x] **Type 3**: Custom glyphs.
- [x] **OpenType/CFF**: Parsing.
- [x] **Composite Fonts**: CID-Keyed (Type 0) full support.
- [x] **ToUnicode**: Full generation/parsing.

## 6. Interactivity
- [x] **Annotations**:
    - [x] Basic (Text, Link, Widget).
    - [x] Markup (Highlight, Underline, etc.).
    - [x] Advanced (3D, Redact, Projection, Sound, Movie).
- [x] **Actions**:
    - [x] Basic (GoTo, URI).
    - [x] JavaScript (Engine Implemented).
    - [x] RichMedia/3D.

## 7. Forms
- [x] **AcroForms**:
    - [x] Basic Fields (Text, Button, Choice).
    - [x] Appearance Generation (NeedAppearances=false).
    - [x] Calculation Order.
- [x] **XFA**:
    - [x] Full Schema Implementation.
    - [x] Data Binding.
    - [x] **Layout Engine**: Improve naive implementation (support flow, auto-height, pagination).
- [x] **HTML Forms**: Recognition/Embedding.

## 8. Compliance
- [x] **PDF/A**:
    - [x] PDF/A-1b Basic enforcement.
    - [x] **Validation**: Fix overly strict JS check for PDF/A-2+.
    - [x] **OutputIntent**: Embed valid ICC profile (sRGB embedded).
    - [x] **Attachments**: Verify compliance of embedded files (Basic check for PDF/A-2).
- [x] **PDF/X**: OutputIntent, TrimBox/BleedBox enforcement.
- [x] **PDF/UA**: Tagged PDF validation.
- [x] **PDF/E & PDF/VT**.

## 9. Security
- [x] **Encryption**:
    - [x] RC4 (40/128).
    - [x] Owner Password Authentication (V < 5).
    - [x] AES (128).
    - [x] AES-256 (PDF 2.0).

## Encryption Roundtrip Test Improvements

- [ ] 1. Test different encryption strengths (40-bit, 128-bit, 256-bit) and algorithms (RC4, AES)
- [ ] 2. Test permissions enforcement (e.g., printing, copying) after parsing
- [ ] 3. Test more complex documents (multiple pages, images, forms, annotations) under encryption
- [ ] 4. Test error handling for:
    - No password provided
    - Corrupted/tampered encrypted files
    - Owner password with restricted permissions
- [ ] 5. Test encrypted streams with compression enabled (e.g., FlateDecode)
- [ ] 6. Add end-to-end tests using the ir package for encryption
- [ ] 7. Test alternate security handlers (e.g., public-key, custom handlers) if supported
- [ ] 8. Test password edge cases (empty, long, non-ASCII, Unicode)
- [ ] 9. Test encrypted documents in PDF/A or compliance modes
- [x] **Signatures**:
    - [x] Basic PKCS#7.
    - [x] PAdES (ETSI).
    - [x] LTV (DSS, OCSP/CRL).

## 10. Structure & Metadata
- [x] **Metadata**: Info Dict, XMP.
- [x] **Tagged PDF**:
    - [x] Full StructTree support (Writer/Parser).
    - [x] RoleMap.
- [x] **Associated Files (AF)**:
    - [x] Document-level (Catalog) AF.
    - [x] Object-level (Page, XObject, Annotation) AF serialization.
- [x] **GeoSpatial**: GeoPDF support.

## 11. PDF 2.0 Specifics
- [x] Page-level Output Intents.
- [x] Black Point Compensation.
- [x] CxF support.

## 12. Planned Examples
- [x] **Merge PDFs**: Combine multiple PDF files into a single document.
- [x] **Split PDF**: Split a PDF into individual pages or ranges.
- [x] **Rotate Pages**: Rotate specific pages in a PDF.
- [x] **Encrypt PDF**: Add password protection to a PDF.
- [x] **Add Metadata**: Update document information (Title, Author, etc.).
- [x] **Add Image**: Insert an image into an existing PDF page.
- [x] **PDF/A Basic**: Create a PDF/A-1b compliant document.
- [x] **PDF/A Advanced**: Create a PDF/A-3b compliant document with embedded XML (ZUGFeRD).
- [x] **Dashboard**: Create a complex one-page executive dashboard with charts, tables, and forms.
