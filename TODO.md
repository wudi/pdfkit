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

- [x] TrueType font embedding: load TrueType/OpenType fonts, extract widths and metadata, and embed FontFile2 streams with descriptors suitable for Type0/CID fonts. Status: Done (sfnt-based loader builds Identity-H Type0 fonts with descriptor/BBox metrics and embeds FontFile2 streams).
- [x] ToUnicode + Identity-H encoding: build ToUnicode CMaps and CID mappings so Unicode text renders correctly for CJK/multi-language content. Status: Done (TrueType loader derives CID-to-Unicode maps and writer emits ToUnicode CMaps for Identity-H Type0 fonts).
- [x] Builder font registration: builder API can register custom TrueType fonts and encode text using the registered fonts (UTF-16BE/CID), covering non-Latin strings. Status: Done (builder supports TrueType registration, auto-encodes text as Identity-H CIDs using rune-to-CID maps, and exercises multi-language text in tests).

# Streaming plan

Status key: Not started / In progress / Done.

- [x] Document start/end streaming: implement streaming.Parser that emits DocumentStart and DocumentEnd events using the existing parse pipeline. Status: Done (streaming parser now parses once and publishes start/end events with version/encryption info).
- [x] Page/content streaming: emit PageStart/PageEnd and ContentOperation events with basic resources so consumers can iterate without loading whole documents. Status: Done (streaming parser walks the page tree via raw parser, decodes content streams, and emits PageStart/PageEnd plus per-stream ContentStream events).

# Design alignment plan

Status key: Not started / In progress / Done.

- [x] Remove observability references from design docs: scrub observability/logging mention from design.md cross-cutting concerns so the docs match the current codebase with observability removed. Status: Done (cross-cutting concerns updated to focus on context, security limits, and error recovery).
- [x] Honor write cancellation: make the writer respond to context cancellation signals during document serialization, returning an error when the provided context is done to align with the design’s context/cancellation guidance. Status: Done (writer checks ctx.Done() at build, per-page, encryption, and serialization phases and aborts early with error).

# New alignment and coverage tasks (2024-XX-XX)

Status key: Not started / In progress / Done.

- [x] Unify PDF/A level typing: consolidate writer and pdfa level enums into a shared type with clear conversions so config/enforcer interoperate without clashes. Status: Done (pdfa.Level is the canonical type; writer aliases to it for config/enforcer interoperability).
- [x] Fix module path references: update design examples/imports to use the actual module path `pdflib/...` to avoid broken code snippets. Status: Done (design examples/imports now reflect the module path).
- [x] Complete filter coverage: add DCT, JPX, CCITT Fax, and JBIG2 decoders to the filter pipeline with limits/timeouts and tests/fixtures. Status: Done (DCT decode implemented; CCITT Fax decodes via golang.org/x/image/ccitt with fixture test; JPX/JBIG2 attempt generic decode and optional external tools (opj_decompress/jbig2dec) before failing with explicit UnsupportedError; pipeline enforces decode timeouts).
- [x] Full JPX/JBIG2 native decoding: evaluate and integrate a maintained Go or cgo-backed decoder (e.g., OpenJPEG or libjbig2dec wrappers) to avoid external tool dependency and handle alpha, palettes, and symbol dictionaries with deterministic limits/tests. Status: Done (JPXDecode prefers the libopenjp2 path, with shared native image bounds enforcing ≤32k dimensions/64MP pixels plus SYCC/CMYK coverage tests; JBIG2 uses libjbig2dec with the same limits and resolver-backed globals, so all native paths now run deterministically without invoking external tools).
- [x] Expand streaming events: emit per-operator/resource/annotation events in streaming.Parser to match the design, with backpressure handling tests. Status: Done (streaming parser now emits ContentOperation/ResourceRef/Annotation/Metadata events with context-aware delivery; test updated to assert per-operator flow).
- [x] Add integration tests: cover image decoding via new filters, PDF/A enforce/validate round-trips, and streaming per-operator consumption/backpressure. Status: Done (pipeline DCT decode test added, PDF/A level/enforcer interoperability test added, and streaming backpressure/per-operator test added).

# Table layout and tagging plan

Status key: Not started / In progress / Done.

- [x] Builder table helper: add a table drawing API (borders, padding, per-column widths) that supports repeating header rows and automatic pagination. Status: Done (table API renders backgrounds/borders/padding, paginates when space runs out, and repeats header rows on new pages).
- [x] Tagging support for tables: emit tagged PDF structure (Table/TR/TH/TD) with MCIDs, parent tree entries, and marked-content wrappers when table helper is used. Status: Done (builder assigns MCIDs per page, builds StructTree with Table/row/cell elements, and writer serializes StructParents/ParentTree with MCR entries).
- [x] Table coverage tests: render tables with borders/padding/header repeat across page breaks and assert tagged structure/MCIDs in output. Status: Done (writer test exercises pagination, header repeat tagging, StructTreeRoot/ParentTree presence, and MCID-marked content).

# Extraction utilities plan

Status key: Not started / In progress / Done.

- [x] Extract text streams: add a semantic-aware text walker that traverses `semantic.Page.Contents` operations (Tj/TJ/'/") to produce UTF-8 page text along with positioning metadata for downstream consumers. Status: Done (`extractor.ExtractText` walks decoded content streams, trims whitespace, and reports page labels; `cmd/extract -text` prints the output).
- [x] Extract image XObjects: surface a helper that iterates each page’s `Resources.XObjects` and inline images, decodes their filters via the existing filter pipeline, and writes them to disk (PNG/JPEG) with sensible filenames. Status: Done (`extractor.ExtractImages` pulls decoded stream bytes and `cmd/extract -images` writes `images/page-XXX-*.bin`).
- [x] Extract annotations: provide a summarizer over `semantic.Page.Annotations` that captures subtype, rect, appearance, URIs, and flags so we can inspect comments/links without re-parsing. Status: Done (`extractor.ExtractAnnotations` normalizes subtype/rect/URI data and feeds JSON via `cmd/extract -annotations`).
- [x] Extract metadata: combine `semantic.Document.Info`, XMP, and trailer-level attributes (Lang/Marked) into a structured JSON/YAML blob for quick auditing. Status: Done (`extractor.ExtractMetadata` aggregates Info, Lang, MarkInfo, permissions, and XMP; `cmd/extract -metadata` emits the JSON blob).
- [x] Extract bookmarks/outlines: flatten `semantic.Document.Outlines` into a deterministic tree (with page indices and XYZ destinations) and emit it as a TOC artifact. Status: Done (`extractor.ExtractBookmarks` walks the outline tree and the CLI reports it under the `bookmarks` section).
- [x] Extract table of contents: merge outlines with `PageLabels` to build a user-facing TOC payload that mirrors viewer panels. Status: Done (`extractor.ExtractTableOfContents` attaches computed page labels; `cmd/extract -toc` surfaces the flattened view).
- [x] Extract font inventory (new need): walk per-page `Resources.Fonts` to report font names, encodings, and whether ToUnicode maps exist, helping audit multi-language PDFs. Status: Done (`extractor.ExtractFonts` deduplicates font dictionaries and lists page usage; surfaced via `cmd/extract -fonts`).
- [x] Extract embedded files (new need): surface `semantic.Document.EmbeddedFiles` as downloadable attachments with relationship metadata to support PDF/A-3 workflows. Status: Done (`extractor.ExtractEmbeddedFiles` walks the Names tree and `cmd/extract -attachments` writes attachments plus JSON summaries).

# Compliance & Quality plan (Phase 4)

Status key: Not started / In progress / Done.

- [x] PDF/A-1b validation: implement `pdfa.Enforcer.Validate` to check for encryption, font embedding, OutputIntents, and forbidden actions (Launch/Sound/Movie). Status: Done (implemented in `pdfa/pdfa.go` with checks for encryption, output intents, font embedding, and forbidden annotations).
- [x] Linearization (Fast Web View): implement 2-pass writing in `writer` to calculate object offsets, reorder objects (Linearization Dict -> Page 1 -> Shared -> Others), and generate Hint Tables when `Linearize: true`. Status: Done (implemented 2-pass writer with object reordering, hint stream generation, and linearization dictionary).

# Error Recovery plan

Status key: Not started / In progress / Done.

- [x] Implement recovery strategies: create `StrictStrategy` (fail fast) and `LenientStrategy` (log & continue) in `recovery/strategies.go`. Status: Done.
- [x] Integrate recovery into scanner/parser: update `scanner` and `parser` to use the configured recovery strategy for common errors (e.g., missing delimiters, bad xrefs). Status: Done (integrated into scanner and object loader; xref repair implemented).
- [x] Add recovery tests: create tests with malformed PDFs to verify that `StrictStrategy` fails and `LenientStrategy` recovers. Status: Done.

# PDF/A Enforcement plan

Status key: Not started / In progress / Done.

- [x] Implement PDF/A enforcement: implement `Enforce` in `pdfa/pdfa.go` to automatically fix violations (e.g., remove encryption, strip forbidden annotations). Status: Done.
- [x] Add enforcement tests: create tests that take a non-compliant PDF and verify it becomes compliant after enforcement. Status: Done.

# Font Subsetting plan

Status key: Not started / In progress / Done.

- [x] Implement GlyphAnalyzer: create `fonts/analyzer.go` to scan content streams and identify used glyphs. Status: Done.
- [x] Implement SubsetPlanner: create `fonts/planner.go` to map original GIDs to a subset. Status: Done (implemented in `fonts/subsetter.go`).
- [x] Implement SubsetGenerator: create `fonts/generator.go` to generate the subsetted font file (initially just a pass-through or simple subset if possible). Status: Done (implemented in `fonts/subsetter.go` and `fonts/tt_subsetter.go` with full binary subsetting; includes fallback for complex scripts like Arabic to preserve shaping).
- [x] Integrate subsetting into writer: update `writer` to use the subsetting pipeline when `SubsetFonts: true`. Status: Done.

# Advanced Font Subsetting plan

Status key: Not started / In progress / Done.

- [x] Capture script-aware runs: extend `fonts.Analyzer` to log per-font UTF-16 runs plus detected script/direction so shaping can replay real text. Status: Done.
- [x] Shape runs via go-text/typesetting: feed recorded runs into `github.com/go-text/typesetting/shaping` with script/lang-specific options to collect exact glyph IDs emitted by the shaper. Status: Done.
- [x] Merge shaped glyphs with closures: union shaped glyph IDs with composite/GSUB closures and drive the planner/subsetter with the expanded set, falling back only when shaping fails. Status: Done.
- [x] Pipeline verification: add regression coverage (unit tests + `examples/fonts` + `examples/extract_text`) ensuring Arabic stays correct while subsetting is active. Status: Done.

# Fuzz Testing

Status key: Not started / In progress / Done.

- [x] Scanner fuzzing: implement fuzz tests for `scanner.Next()` to ensure robustness against malformed inputs. Status: Done (implemented in `scanner/scanner_fuzz_test.go`).
- [x] Parser fuzzing: implement fuzz tests for `DocumentParser.Parse()` to ensure robustness against malformed document structures. Status: Done (implemented in `parser/parser_fuzz_test.go`).
- [x] Filter fuzzing: implement fuzz tests for `Pipeline.Decode()` to ensure robustness of stream decoders. Status: Done (implemented in `filters/filters_fuzz_test.go`).

# Extension System plan

Status key: Not started / In progress / Done.

- [x] Formalize Extension Interfaces: Update `extensions/extensions.go` to define specific interfaces (`Inspector`, `Sanitizer`, `Transformer`, `Validator`) and report structures (`InspectionReport`, `SanitizationReport`, `ValidationReport`). Status: Done.
- [x] Implement Standard Extensions: Create default implementations for Inspector (metadata/count), Sanitizer (remove JS), and Validator (wrapper around `pdfa`). Status: Done.
- [x] Documentation: Update `design.md` to reflect any intentional deviations or update the code to match. Status: Done.

# Optimization plan

Status key: Not started / In progress / Done.

- [x] Optimize `scanner.Token` struct: remove `interface{}` boxing to reduce GC pressure. Status: Done (replaced `Value` interface with concrete fields `Str`, `Bytes`, `Int`, `Float`, `Bool`).
- [x] Implement string interning: intern Name objects to reduce memory usage. Status: Done (added `internPool` to `scanner` to deduplicate Name strings).
- [x] Update consumers: update all packages (`parser`, `ir`, `xref`, `extractor`) to use the new `Token` API. Status: Done (all packages updated and tests passing).
- [x] Verify performance: run benchmarks to quantify memory and CPU improvements. Status: Done (BenchmarkParse50MB: 77ms/op, 387MB/op, 340k allocs/op).

# Future Roadmap (v2.0)

Status key: Not started / In progress / Done.

- [x] Digital Signatures: Implement `Sig` dictionary, byte range calculation, and cryptographic signing (RSA/SHA-256) to support digitally signed PDFs. Status: Done (implemented `writer.Sign` with incremental updates, ByteRange calculation, and `security.RSASigner` with full PKCS#7 detached signature support).
- [x] Form Filling API: Create a high-level API in `builder` to fill AcroForm fields (text, checkbox, radio) and flatten forms. Status: Done (implemented `FormBuilder` interface, `PageBuilder.AddFormField`, and `PDFBuilder.Form` for creating and filling text, checkbox, and choice fields).
- [x] HTML/Markdown to PDF: Implement a layout engine that converts HTML/Markdown input into PDF pages using the `builder` API. Status: Done (implemented layout engine in `layout` package using `github.com/yuin/goldmark` for Markdown and `golang.org/x/net/html` for HTML, supporting headers, paragraphs, lists, and pagination).

# Architecture Refactoring Plan (v2.1)

Status key: Not started / In progress / Done.

## Phase 1: Polymorphic Semantic Model
- [x] Refactor `Annotation` to an interface: Replace the monolithic `Annotation` struct with an interface and specific implementations (e.g., `LinkAnnotation`, `WidgetAnnotation`, `TextAnnotation`). Status: Done.
- [x] Refactor `Action` to an interface: Introduce an `Action` interface to handle interactivity (e.g., `GoToAction`, `URIAction`, `JavaScriptAction`). Status: Done.
- [x] Refactor `ColorSpace` to an interface: Refactor `ColorSpace` to handle complex definitions (e.g., `ICCBasedColorSpace`, `DeviceNColorSpace`). Status: Done.

## Phase 2: Strategy-Based Writer
- [x] Refactor `writer/object_builder.go`: Break down the monolithic `object_builder.go` into smaller, specialized serializers using the Strategy Pattern (e.g., `AnnotationSerializer`, `ColorSpaceSerializer`). Status: Done.

## Phase 3: Incremental Update Tracking
- [x] Add `OriginalRef` and `Dirty` flags: Add `OriginalRef` and `Dirty` flags to all semantic objects to track their origin and modification status for incremental updates. Status: Done.

## Phase 4: PDF 2.0 Compliance
- [x] Add PDF 2.0 fields: Add specific PDF 2.0 fields (e.g., `OutputIntents` on Page, `AssociatedFiles`) to the semantic model. Status: Done.

# Feature Expansion (v2.2)

Status key: Not started / In progress / Done.

## Phase 1: Expanded Annotation Support
- [x] Implement Text Markup Annotations: `Highlight`, `Underline`, `StrikeOut`, `Squiggly`. Status: Done.
- [x] Implement Shape Annotations: `Line`, `Square`, `Circle`. Status: Done.
- [x] Implement Text Annotations: `Text` (Sticky Note), `FreeText` (Typewriter). Status: Done.
- [x] Implement Widget Annotations: `Widget` (Form Fields). Status: Done (integrated into AnnotationSerializer and AcroForm builder).
- [x] Implement Stamp Annotation. Status: Done.
- [x] Implement Ink Annotation. Status: Done.
- [x] Implement FileAttachment Annotation. Status: Done.
- [x] Implement Other Annotations: `Popup`, `Sound`, `Movie`, `Screen`, `PrinterMark`, `TrapNet`, `Watermark`, `3D`, `Redact`, `Projection`. Status: Done.

## Phase 2: Expanded Action Support
- [x] Implement JavaScript Action. Status: Done.
- [x] Implement Named Action. Status: Done.
- [x] Implement Launch Action. Status: Done.
- [x] Implement SubmitForm/ResetForm/ImportData Actions. Status: Done.

## Phase 3: Advanced Graphics
- [x] Implement Blend Modes. Status: Done.
- [x] Implement Transparency Groups. Status: Done.
- [x] Implement Soft Masks. Status: Done.

# Architecture Refactoring Plan (v2.1) - Deepening the Semantic Layer

Status key: Not started / In progress / Done.

## Phase 1: Polymorphic Form Fields & XFA
- [x] Refactor `AcroForm` to support XFA stream.
- [x] Refactor `FormField` to an interface with specific implementations:
    - [x] `TextFormField` (Text fields)
    - [x] `ChoiceFormField` (Combo/List boxes)
    - [x] `ButtonFormField` (Push/Check/Radio buttons)
    - [x] `SignatureFormField` (Digital signatures)
- [x] Update `WidgetAnnotation` to use the new `FormField` interface.
- [x] Update `writer` to serialize the new polymorphic field types.
- [x] Update `builder` and `extractor` to accommodate the changes.

## Phase 2: Advanced Color & Shading
- [x] Implement `SeparationColorSpace` (Spot Colors).
- [x] Implement `DeviceNColorSpace` (Multi-channel).
- [x] Implement `MeshShading` (Type 4-7) for complex gradients.
- [x] Update `writer` to serialize new color spaces and shadings.

## Phase 3: Digital Signature Model
- [x] Implement `Signature` struct in `semantic` to inspect signature details.
- [x] Support `ByteRange`, `SubFilter`, `Cert`, and `Reference` dictionaries (PAdES prep).
- [x] Update `writer` to use the semantic `Signature` model.

## Phase 4: Completing Actions
- [x] Implement `GoToRAction` (Remote GoTo).
- [x] Implement `GoToEAction` (Embedded GoTo).
- [x] Implement `HideAction`.
- [x] Implement `TransAction` (Page Transitions).
- [x] Implement `GoTo3DViewAction`.

# Architecture Refactoring Plan (v2.3) - Zero Compromise Semantics

Status key: Not started / In progress / Done.

## Phase 1: Semantic Functions
- [x] Create `Function` interface in `semantic` (Type 0, 2, 3, 4).
- [x] Implement `SampledFunction` (Type 0).
- [x] Implement `ExponentialFunction` (Type 2).
- [x] Implement `StitchingFunction` (Type 3).
- [x] Implement `PostScriptFunction` (Type 4).
- [x] Update `Shading`, `ColorSpace`, and `ExtGState` to use `Function` instead of `[]byte`.
- [x] Update `writer` to serialize semantic functions.

## Phase 2: Polymorphic Patterns
- [x] Refactor `Pattern` struct to an interface.
- [x] Implement `TilingPattern` (Type 1).
- [x] Implement `ShadingPattern` (Type 2).
- [x] Update `Resources` to use `Pattern` interface.
- [x] Update `writer` to serialize polymorphic patterns.

## Phase 3: Advanced Fonts & Optional Content
- [x] Enhance `Font` model to support Type 3 specific fields (`CharProcs`, `FontMatrix`, `Resources`).
- [x] Implement `OptionalContentGroup` (OCG) and `OptionalContentMembership` (OCMD).
- [x] Update `Resources` to map `Properties` to semantic OCGs.

# Architecture Refactoring Plan (v2.4) - Behavioral & Editing Layers

Status key: Not started / In progress / Done.

## Phase 1: The Scripting Layer (Behavior)
- [x] Create `scripting` package with `Engine` interface.
- [x] Define `PDFDOM` interface to expose `semantic.Document` safely to scripts.
- [x] Implement `JavaScriptAction` execution hook in `extensions`.

## Phase 2: The Content Editor Layer (Editing)
- [x] Create `contentstream/editor` package.
- [x] Implement `SpatialIndex` (QuadTree) for `[]Operation`.
- [x] Implement `Editor` API (`RemoveRect`, `ReplaceText`) using the index.
- [x] Update `contentstream.Processor` to support "Trace" mode for indexing.

## Phase 3: The Validation Layer (Security)
- [x] Create `security/validation` package.
- [x] Implement `ChainBuilder` for X.509 certificate chains.
- [x] Implement `RevocationChecker` (OCSP/CRL).
- [x] Implement `LTVManager` for DSS dictionaries.

## Phase 4: The XFA Engine (Rendering)
- [x] Create `xfa` package.
- [x] Implement `Parser` (XML -> XFA DOM).
- [x] Implement `Layout` (XFA DOM -> `semantic.Page`).

## Phase 5: PDF 2.0 Envelope Support
- [x] Refactor `semantic.Document` to support `Payload` (nested document).
- [x] Update `parser` to detect and handle "Collection" / "EncryptedPayload".

## v2.5 Architecture Refactoring (Zero Compromise Completion)
- [x] **Phase 1: Color Management System (CMS)**
    - [x] Create `cmm` package (ICC, CxF, Transform).
    - [x] Update `ir/semantic` for Spectral Colors.
    - [x] Integrate `cmm` into `pdfa` and `contentstream`.
- [x] **Phase 2: Advanced Tagging & Accessibility**
    - [x] Implement PDF 2.0 Namespaces in `semantic`.
    - [x] Make `contentstream/editor` tag-aware (StructTree repair).
    - [x] Implement `RoleMap` and `ClassMap` logic.
- [x] **Phase 3: Geospatial & Engineering**
    - [x] Create `geo` package (Viewports, Measure).
    - [x] Update `ir/semantic` for GeoPDF support.
- [ ] **Phase 4: Compliance Engines**
    - [ ] Refactor `pdfa` to `compliance`.
    - [ ] Add `pdfx`, `pdfua`, `pdfvt` sub-packages.
- [ ] **Phase 5: Advanced Text**
    - [ ] Implement `ReplaceText` with reshaping in `editor`.
    - [ ] Support Vertical Writing mode.
