# Architecture Review & Refactoring Plan (v2.4)

## 1. Review of Current Architecture (v2.3)

The current architecture, following the v2.3 refactoring (Polymorphic Semantic Model), is **highly robust** and well-positioned to meet the "Zero Compromise" goals.

### Strengths
*   **Three-Tier IR (Raw -> Decoded -> Semantic):** Provides the necessary abstraction layers to handle both low-level repair and high-level manipulation.
*   **Polymorphism:** The recent introduction of interfaces for `Annotation`, `Action`, `ColorSpace`, `Pattern`, `Shading`, and `Function` allows for full standard compliance without monolithic structures.
*   **Streaming & Extension System:** Solid foundation for processing large files and adding custom logic.
*   **PDF 2.0 Readiness:** The semantic model already includes PDF 2.0 fields (OutputIntents, AssociatedFiles).

### Gaps & Future Challenges
While the *data structures* are ready, the *behavioral* and *editing* capabilities required for some advanced features are missing or under-architected:

1.  **Complex Content Editing (Redaction):**
    *   *Goal:* "Physically delete underlying content" (Redaction).
    *   *Gap:* The current `ContentStream` is a linear list of operations. Deleting content spatially (e.g., "remove everything in this Rect") requires complex geometric analysis (QuadTree/Spatial Indexing) which is currently missing.

2.  **Active Behavior (JavaScript & Forms):**
    *   *Goal:* "Support Acrobat JavaScript API" and "Form Calculation".
    *   *Gap:* The architecture defines *data* for Actions/Forms but has no *execution environment*. We need a Scripting Layer.

3.  **Dynamic XFA Rendering:**
    *   *Goal:* "Dynamic XFA Rendering".
    *   *Gap:* XFA is XML-based. Rendering it requires a completely separate layout engine that converts XML -> PDF Pages. The current `layout` package is for HTML/Markdown, not XFA.

4.  **Advanced Security (LTV/PAdES):**
    *   *Goal:* "Long Term Validation (LTV)".
    *   *Gap:* Validation requires a complex chain-of-trust engine (OCSP/CRL client, Certificate Store). The current `security` package is focused on Encryption/Signing, not Validation.

5.  **PDF 2.0 Unencrypted Wrapper:**
    *   *Goal:* Support "Unencrypted Wrapper Document".
    *   *Gap:* This fundamentally changes the file structure (a PDF inside a PDF). The `parser` needs to be aware of this "Envelope" pattern.

---

## 2. Refactoring Plan (v2.4)

To accommodate these future features, we propose the following architectural refinements. Since we are in the development phase, we can introduce these changes now.

### Phase 1: The Scripting Layer (Behavior)
**Objective:** Enable Form Calculations and Action Execution.

*   **New Package:** `scripting`
*   **Interface:** `Engine` (Execute script, access Document Object Model).
*   **Integration:** Add `ScriptEngine` field to `semantic.Document` (optional).
*   **Refactoring:**
    *   Define a "PDF DOM" interface that exposes `semantic.Document` to the script engine in a safe way.
    *   Implement a bridge for a Go JS runtime (e.g., `goja`).

### Phase 2: The Content Editor Layer (Editing)
**Objective:** Enable Redaction and Advanced Content Modification.

*   **New Package:** `contentstream/editor`
*   **Components:**
    *   `SpatialIndex`: A QuadTree or R-Tree implementation that indexes `Operation`s by their bounding box on the page.
    *   `Editor`: A high-level API (`RemoveRect(r Rectangle)`, `ReplaceText(old, new string)`).
*   **Refactoring:**
    *   Update `contentstream.Processor` to support "Trace" mode (calculating BBox for every op without rendering).

### Phase 3: The Validation Layer (Security)
**Objective:** Enable PAdES LTV and Digital Signature Verification.

*   **New Package:** `security/validation`
*   **Components:**
    *   `ChainBuilder`: Builds and verifies X.509 certificate chains.
    *   `RevocationChecker`: Handles OCSP/CRL.
    *   `LTVManager`: Manages DSS (Document Security Store) dictionaries.
*   **Refactoring:**
    *   Split `security` into `security/encryption`, `security/signing`, `security/core`.

### Phase 4: The XFA Engine (Rendering)
**Objective:** Support Dynamic XFA (Legacy Government Forms).

*   **New Package:** `xfa`
*   **Components:**
    *   `Parser`: XML -> XFA DOM.
    *   `Layout`: XFA DOM -> `semantic.Page` (Virtual Pages).
*   **Integration:**
    *   Add `RenderXFA(ctx context.Context) error` to `semantic.Document` which populates `Pages` from `AcroForm.XFA`.

### Phase 5: PDF 2.0 Envelope Support
**Objective:** Support Unencrypted Wrapper Documents.

*   **Refactoring:**
    *   Modify `parser.Document` to support a `Payload` field.
    *   Update `loader` to detect the "Collection" dictionary and "EncryptedPayload" key, allowing the parser to transparently "enter" the payload if requested.

## 3. Immediate Action Items

1.  **Create `contentstream/editor` skeleton:** This is the prerequisite for the "Redact" feature.
2.  **Define `scripting` interface:** This prepares the ground for interactive forms.
