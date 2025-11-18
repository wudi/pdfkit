# PDF writer completeness checklist

Status key: Not started / In progress / Done.

- [x] File structure: emit versioned header, unique file ID, body objects, xref table/stream with correct byte offsets, trailer with Size/Root/Info/ID/Encrypt, `%%EOF`, and incremental update support. Status: Done (headers/IDs in place; xref tables/streams now preserve prior object offsets during incremental appends and retain Prev chains).
- [x] Object model: cover all primitive types (null/bool/number/string/name/array/dict/stream/indirect refs), indirect serialization, generation numbers, and reuse. Status: Done (literal and hex strings encoded, primitive serialization covers arrays/dicts/streams/refs, and deduped reusable font objects).
- [x] Page tree: build Catalog/Pages hierarchy with Count/Kids, media/trim boxes, rotation, and inherited Resources. Status: Done (geometry/rotation/UserUnit emitted; Pages aggregates resources across fonts, ExtGState, ColorSpace, XObject, Pattern, and Shading for inheritance).
- [x] Content streams: encode filters (Flate/ASCIIHex/ASCII85; optionally LZW/RunLength/JPX/JBIG2), lengths, and operators for text/graphics state/paths/shadings/images/forms/annotations. Status: Done (content streams support Flate/ASCIIHex/ASCII85/RunLength/LZW/JPX/JBIG2 with correct Length/Filter and serialize semantic operations).
- [x] Resources: handle fonts (Type1/TrueType/CID with encodings and widths), color spaces, patterns, shadings, XObject forms/images, ExtGState, proc sets. Status: Done (font serialization covers Type1, TrueType, and Type0 with CIDFont descendants and width arrays; ProcSet emitted with Image entries when XObjects present; ExtGState dictionaries serialized with line width and alpha; ColorSpace dictionaries emitted; Image and Form XObjects serialized and shared; tiling Pattern resources serialized; Shading dictionaries emitted).
- [x] Metadata & outlines: document info dictionary, XMP metadata stream, outlines/bookmarks, page labels, ViewerPreferences, article threads. Status: Done (Info dictionary writes title/author/subject/creator/producer/keywords; ViewerPreferences sets DisplayDocTitle when Title present; page labels emitted via Nums; outlines emitted with Dest/Prev/Next/First/Last for children plus PageMode UseOutlines; XMP metadata streams implemented; Threads array emitted with linked beads for articles).
- [x] Annotations & forms: annotations (links/text/widgets) with appearance streams; AcroForm fields/apparences/NeedAppearances. Status: Done (link annotations with Rect/URI and contents; color/border/flag support; appearance streams and states serialized; widget fields include rects, colors, flags, appearances, and are attached to page annotations; AcroForm emitted with NeedAppearances and serialized fields).
- [x] Encryption/security: Encrypt dictionary with Standard handler (keys/permissions); embedded file specs if needed. Status: Done (Standard handler entries generated from user/owner passwords and permissions; objects/streams encrypted via handler; IDs stored in trailer; metadata encryption configurable).
- [x] Compliance: PDF/A tagging (StructTreeRoot), role maps, font consistency, OutputIntents, downgrade/clip to target level. Status: Done (StructTreeRoot emitted with RoleMap when provided; OutputIntents serialized with ICC profile streams; Catalog now carries Lang and MarkInfo/Marked for tagged PDF).
- [x] Validation/robustness: length bounds, offset consistency, deterministic ordering, malformed stream avoidance, readback tests. Status: Done (xref table and xref stream startxref offsets validated; deterministic ordering for dict keys and IDs in place; xref table offsets cross-checked against actual object positions and decrypt/metadata paths covered by tests).

# Parser robustness plan

Status key: Not started / In progress / Done.

- [x] Inline image end detection: tighten inline image scanning to avoid false positives in binary content and obey PDF EOD rules. Status: Done (scanner now searches the full inline image region and picks the last valid EI delimiter bounded by size limits, reducing premature termination inside binary data).
- [x] Memory-bounded scanning: support bounded/sliding scanner buffers so large files do not require whole-file buffering while still honoring random-access seeks. Status: Done (scanner maintains a sliding window with pinned regions for streams/inline images, obeys MaxBufferSize, and reloads windows on backward seeks).

# Builder completeness plan

Status key: Not started / In progress / Done.

- [x] Builder metadata helpers: expose document-level language (Lang), tagged flag (Marked), and page labels to align builder with semantic.Document fields from design.md. Status: Done (SetLanguage/SetMarked/AddPageLabel added and covered by builder test).
- [x] Builder outlines: fluent helpers to add outline/bookmark entries that target builder-created pages with XYZ destinations. Status: Done (builder Outline type resolves page pointers to indexes and populates XYZ destinations; writer serializes XYZ Dest arrays).
- [x] Builder encryption: convenience setter for owner/user passwords, permissions, and metadata encryption flag to populate semantic.Document encryption fields. Status: Done (SetEncryption records passwords/permissions, marks metadata encryption, and is exercised by builder tests).

# Font system plan (multi-language)

Status key: Not started / In progress / Done.

- [ ] TrueType font embedding: load TrueType/OpenType fonts, extract widths and metadata, and embed FontFile2 streams with descriptors suitable for Type0/CID fonts. Status: Not started.
- [ ] ToUnicode + Identity-H encoding: build ToUnicode CMaps and CID mappings so Unicode text renders correctly for CJK/multi-language content. Status: Not started.
- [ ] Builder font registration: builder API can register custom TrueType fonts and encode text using the registered fonts (UTF-16BE/CID), covering non-Latin strings. Status: Not started.
