# v2.5+ Refactoring & Future-Proofing Plan

## 1. Architectural Review
The current **Three-Tier IR (Raw -> Decoded -> Semantic)** architecture is robust and industry-standard. It successfully decouples parsing, logic, and serialization.
- **Strengths**:
    - **Change Tracking**: `Dirty` flags are already present in `ir/semantic` structs, enabling efficient Incremental Updates.
    - **Modularity**: Clear separation of concerns (Scanner, Parser, Writer, Compliance).
    - **Extensibility**: The `extensions` system allows for validation and transformation pipelines.

- **Weaknesses (Gaps)**:
    - **Memory Scalability**: `semantic.Page` loads `Contents []ContentStream` eagerly. For massive documents (e.g., 50k+ pages), this will exhaust memory.
    - **Layout Duplication**: `layout` (Markdown/HTML) and `xfa` (XML Forms) both require flow layout logic, but currently lack a shared engine.
    - **Missing Modules**: `cmm` (Color Management), `geo` (Geospatial), and `scripting` (JavaScript) are defined in design but unimplemented.

## 2. Refactoring Plan

### Phase 1: Scalability & Lazy Loading (Critical for "Zero Compromise")
**Goal**: Support documents of arbitrary size (10GB+, 100k pages) without OOM.
1.  **Refactor `semantic.Page`**:
    - Replace `Contents []ContentStream` with a `ContentProvider` interface.
    - Implement `LazyContentProvider` that reads streams from disk on-demand.
2.  **Refactor `semantic.Resources`**:
    - Implement `LazyResourceProvider` to load fonts/images only when accessed.
3.  **Stream-Based Processing**:
    - Ensure `filters` and `parser` can operate on `io.Reader` streams without buffering the entire object in memory.

### Phase 2: Unified Layout Engine
**Goal**: Support HTML, Markdown, and XFA with a single consistent rendering model.
1.  **Create `layout/flow`**:
    - Extract a generic **Box Model** engine (Block, Inline, Table, Flex) from `layout`.
    - Implement text shaping and wrapping using `fonts/shaper.go`.
2.  **Update Consumers**:
    - Refactor `layout` (HTML/Markdown) to use `layout/flow`.
    - Implement `xfa` layout using `layout/flow`.

### Phase 3: Advanced Features Implementation
**Goal**: Complete the "Goal Features" list.
1.  **Implement `cmm`**:
    - Add ICC profile parsing and color conversion.
    - Support PDF 2.0 "Spectrally Defined Colors" (CxF).
2.  **Implement `scripting`**:
    - Integrate a JS runtime (e.g., `goja`).
    - Bind `ir/semantic` objects to the JS DOM.
3.  **Implement `geo`**:
    - Add Geospatial coordinate system support to `ir/semantic`.

### Phase 4: Security & Encryption Updates
**Goal**: Modern security compliance.
1.  **PDF 2.0 Encryption**:
    - Implement AES-256 (AESV3) in `security`.
    - Deprecate/Remove RC4 for PDF 2.0 files.
2.  **Granular Security**:
    - Refactor `parser` to support "Unencrypted Wrapper" (different security handlers for different streams).

## 3. Execution Strategy
Since we are in the **Development Phase**, we will prioritize **Architecture Correctness** over backward compatibility.
- **Immediate Action**: Refactor `semantic.Page` for lazy loading (Phase 1). This is the most invasive change and should be done before v1.0.
- **Secondary Action**: Extract `layout/flow` (Phase 2) as we build out XFA support.
