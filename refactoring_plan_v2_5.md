# v2.5 Architecture Refactoring Plan: The "Zero Compromise" Completion

This plan addresses the remaining architectural gaps identified from `goal-features.md` to ensure full support for PDF 2.0, PDF/A-4, PDF/UA, PDF/X, and GeoPDF.

## Phase 1: Color Management System (CMS)
**Goal**: Support high-fidelity color conversion, ICC profiles, and PDF 2.0 Spectral Data (CxF).
**Rationale**: Essential for PDF/A compliance (device-independent color), PDF/X (printing), and accurate rendering.

- [ ] **New Package**: `cmm` (Color Management Module).
    - Interfaces for `Profile`, `Transform`.
    - Support for ICC v4 profiles.
    - Support for PDF 2.0 `BlackPointCompensation`.
- [ ] **Update `ir/semantic`**:
    - Add `SpectrallyDefinedColor` (PDF 2.0).
    - Update `OutputIntent` to use `cmm.Profile`.
- [ ] **Integration**:
    - `pdfa` package uses `cmm` to validate/convert colors.
    - `contentstream` uses `cmm` for color space transformation.

## Phase 2: Advanced Tagging & Accessibility Engine
**Goal**: Full PDF/UA support and maintaining structural integrity during editing.
**Rationale**: "Zero Compromise" editing must not break accessibility.

- [ ] **New Package**: `structure` (refactor out of `semantic` or expand).
    - `Namespace` support (PDF 2.0).
    - `RoleMap` expansion.
    - `ClassMap` support.
- [ ] **Update `contentstream/editor`**:
    - **Tag-Aware Editing**: When `RemoveRect` is called, identify affected `MCID` (Marked Content ID) sequences.
    - **Tree Repair**: Automatically prune or update the `StructTree` when content is deleted.
- [ ] **New Extension**: `AutoTagger`.
    - Heuristics to generate tags for untagged documents (optional, but high value).

## Phase 3: Geospatial & Engineering Support (PDF/E)
**Goal**: Support GeoPDF and 3D engineering features.
**Rationale**: Required for "PDF/E" and specialized workflows.

- [ ] **New Package**: `geo`.
    - `Viewport` definitions.
    - `CoordinateSystem` (Lat/Lon to PDF coords).
    - `Measure` dictionaries.
- [ ] **Update `ir/semantic`**:
    - Add `Measure` dictionary support to `Page`.
    - Add `Viewport` support.

## Phase 4: Compliance Engines (PDF/A-4, PDF/X, PDF/VT)
**Goal**: Specialized validation and enforcement for all ISO standards.
**Rationale**: The current `pdfa` package is a stub.

- [ ] **Refactor `pdfa`**:
    - Rename to `compliance`.
    - Sub-packages: `pdfa`, `pdfx`, `pdfua`, `pdfvt`.
- [ ] **Implement Validators**:
    - `Isartor` test suite logic integration.
    - `VeraPDF` logic integration (where possible in Go).

## Phase 5: Advanced Text & Shaping
**Goal**: Full support for Vertical Writing and Complex Scripts in the Editor.

- [ ] **Update `fonts`**:
    - Integrate `harfbuzz` or pure Go shaper (e.g., `go-text/typesetting`) more deeply.
- [ ] **Update `contentstream/editor`**:
    - Implement `ReplaceText` with full reshaping capabilities.
    - Handle `WritingMode` (Vertical).

## Execution Strategy
1.  **Design**: Update `design.md` to include these new modules.
2.  **Scaffold**: Create package structures (`cmm`, `geo`, `compliance`).
3.  **Integrate**: Wire them into the `ir/semantic` and `parser`.
