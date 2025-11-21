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
- [ ] **Linearization (Fast Web View)**: Support Hint Tables, allowing byte-streaming loading.
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
- [ ] **Color Spaces**:
    - [x] Implement `cmm` package (ICC Profile parsing/transform).
    - [x] Support `DeviceN` and `Separation` (Spot Colors).
    - [x] Support `Pattern` color space.
- [ ] **Patterns & Shading**:
    - [x] Tiling Patterns (Type 1).
    - [x] Shading Patterns (Type 1-7).
- [ ] **Images**:
    - [x] Image Dictionary (Basic).
    - [x] SMask (Soft Mask).
    - [x] Inline Images.
- [ ] **Transparency**:
    - [x] Blend Modes (All 16).
    - [x] Transparency Groups (Isolated, Knockout).

## 5. Fonts & Text
- [x] **TrueType**: Parsing and extraction.
- [x] **Type 1**: Parsing .pfb/.pfm.
- [x] **Type 3**: Custom glyphs.
- [x] **OpenType/CFF**: Parsing.
- [x] **Composite Fonts**: CID-Keyed (Type 0) full support.
- [x] **ToUnicode**: Full generation/parsing.

## 6. Interactivity
- [ ] **Annotations**:
    - [x] Basic (Text, Link, Widget).
    - [x] Markup (Highlight, Underline, etc.).
    - [x] Advanced (3D, Redact, Projection, Sound, Movie).
- [ ] **Actions**:
    - [x] Basic (GoTo, URI).
    - [x] JavaScript (Need Engine).
    - [x] RichMedia/3D.

## 7. Forms
- [x] **AcroForms**:
    - [x] Basic Fields (Text, Button, Choice).
    - [x] Appearance Generation (NeedAppearances=false).
    - [x] Calculation Order.
- [ ] **XFA**:
    - [x] Full Schema Implementation.
    - [x] Data Binding.
    - [x] Layout Engine.
- [x] **HTML Forms**: Recognition/Embedding.

## 8. Compliance
- [x] **PDF/A-1b**: Basic enforcement.
- [x] **PDF/A-2/3/4**: Full validation and conversion.
- [x] **PDF/X**: OutputIntent, TrimBox/BleedBox enforcement.
- [x] **PDF/UA**: Tagged PDF validation.
- [x] **PDF/E & PDF/VT**.

## 9. Security
- [x] **Encryption**:
    - [x] RC4 (40/128).
    - [x] AES (128).
    - [ ] AES-256 (PDF 2.0).
- [ ] **Signatures**:
    - [x] Basic PKCS#7.
    - [ ] PAdES (ETSI).
    - [ ] LTV (DSS, OCSP/CRL).

## 10. Structure & Metadata
- [x] **Metadata**: Info Dict, XMP.
- [ ] **Tagged PDF**:
    - [ ] Full StructTree support (Writer/Parser).
    - [ ] RoleMap.
- [ ] **GeoSpatial**: GeoPDF support.

## 11. PDF 2.0 Specifics
- [ ] Page-level Output Intents.
- [ ] Black Point Compensation.
- [ ] CxF support.
