# PDF Parser & Creator Library — Architecture

## 1. Executive Summary

This document specifies the architecture, interfaces, and extensibility model for a **production-grade PDF parser and creator library** in Go.

The library is designed to support:

* **Streaming parsing and writing** for large PDFs with configurable backpressure
* **Three-tier IR (Intermediate Representation)** with explicit transformation boundaries
* **Full fidelity read/write** including vector graphics, text, images, forms, annotations, and metadata
* **Incremental updates** with append-only or full rewrite modes
* **Font subsetting and embedding** with complete pipeline specification
* **PDF/A compliance** (1/2/3 variants) with validation and auto-correction
* **Extensible plugin system** with defined execution phases and ordering
* **Robust security and error recovery**

The library provides both **high-level convenience APIs** for typical PDF tasks and **low-level control** for applications requiring fine-grained operations.

---

## 2. Goals

**Primary Goals**

1. Parse PDFs of any complexity with configurable memory limits
2. Provide three explicit IR levels (Raw, Decoded, Semantic) with clear transformation boundaries
3. Enable deterministic PDF generation with embedded font subsetting
4. Support incremental updates and append-only modifications
5. Ensure PDF/A compliance with validation and automatic fixes
6. Provide extensibility through well-defined plugin phases
7. Handle malformed PDFs with configurable error recovery
8. Support concurrent operations where safe

**Non-Goals**

* Full OCR or text recognition (may be delegated to plugins)
* PDF rendering engine (layout calculation for display)
* Built-in cloud storage integration (external I/O adapters)

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     High-Level Builder API                   │
│                   (Convenience, Fluent Interface)            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Semantic IR (Level 3)                       │
│         Pages, Fonts, Images, Annotations, Metadata         │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Decoded IR (Level 2)                        │
│            Decompressed Streams, Decrypted Objects          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Raw IR (Level 1)                          │
│       Dictionaries, Arrays, Streams, Names, Numbers          │
└────────────────────────┬────────────────────────────────────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
          ▼              ▼              ▼
    ┌─────────┐    ┌─────────┐   ┌──────────┐
    │ Scanner │    │  XRef   │   │ Security │
    │Tokenizer│    │Resolver │   │ Handler  │
    └─────────┘    └─────────┘   └──────────┘
          │              │              │
          └──────────────┴──────────────┘
                         │
                         ▼
                   Input Stream

───────────────────────────────────────────────────────────────

                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Extension Hub                             │
│          Inspect → Sanitize → Transform → Validate           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                 Serialization Engine                         │
│       Full Writer, Incremental Writer, Linearization        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
                   Output Stream
```

**Cross-Cutting Concerns (injected into all layers):**
- Error Recovery Strategy
- Context & Cancellation
- Security Limits

---

## 4. Module Architecture

### 4.1 Core Modules

| Module           | Responsibility                                                    |
| ---------------- | ----------------------------------------------------------------- |
| `scanner`        | Tokenizes raw PDF bytes, handles PDF syntax                       |
| `xref`           | Resolves cross-reference tables and streams                       |
| `security`       | Encryption/decryption, permissions, password handling             |
| `parser`         | Coordinates scanning, xref, security to build Raw IR              |
| `ir/raw`         | Raw PDF objects (Level 1): dictionaries, arrays, streams          |
| `ir/decoded`     | Decoded objects (Level 2): decompressed, decrypted                |
| `ir/semantic`    | Semantic objects (Level 3): pages, fonts, annotations             |
| `filters`        | Stream decoders (Flate, DCT, JPX, etc.) with pipeline composition |
| `contentstream`  | Content stream parsing, graphics state, text positioning          |
| `resources`      | Resource resolution with inheritance and scoping                  |
| `fonts`          | Font subsetting, embedding, ToUnicode generation                  |
| `coords`         | Coordinate transformations, user space, device space              |
| `writer`         | PDF serialization: full, incremental, linearized                  |
| `pdfa`           | PDF/A validation, XMP generation, ICC profiles, compliance fixes  |
| `extensions`     | Plugin system with phased execution model                         |
| `recovery`       | Error recovery strategies for malformed PDFs                      |
| `builder`        | High-level fluent API for PDF construction                        |
| `layout`         | Layout engine for converting structured content (Markdown/HTML) to PDF |
| `scripting`      | JavaScript execution environment and PDF DOM implementation       |
| `contentstream/editor` | Spatial indexing (QuadTree) and content redaction/editing |
| `security/validation` | Digital signature validation, LTV, and revocation checking |
| `xfa`            | XML Forms Architecture parsing and layout engine                  |
| `cmm`            | Color Management Module (ICC, CxF)                                |
| `geo`            | Geospatial PDF support                                            |
| `compliance`     | Unified compliance engine (PDF/A, PDF/X, PDF/UA)                  |

### 4.2 Module Dependencies

```
builder
  └─→ ir/semantic
       └─→ ir/decoded
            └─→ ir/raw
                 └─→ scanner, xref, security

layout
  └─→ builder

extensions
  └─→ ir/semantic (operates on semantic IR)

writer
  └─→ ir/semantic
       └─→ ir/decoded
            └─→ ir/raw

fonts
  └─→ ir/semantic (pages, text)

contentstream
  └─→ ir/decoded (stream bytes)
       └─→ coords (transformations)

filters
  └─→ ir/raw (stream dictionaries)

recovery
  └─→ (injected into all layers)
```

---

## 5. Three-Tier IR Architecture

### 5.1 Level 1: Raw IR

**Purpose:** Direct representation of PDF primitive objects as per PDF spec.

### 5.2 Level 2: Decoded IR

**Purpose:** Objects after stream decoding and decryption.

### 5.3 Level 3: Semantic IR

**Purpose:** High-level PDF structures with business logic.

### 5.4 IR Transformation Pipeline

---

## 6. Core Component Specifications

### 6.1 Scanner & Parser
### 6.2 XRef Resolution
### 6.3 Object Loader
### 6.4 Filter Pipeline
### 6.5 Security Handler

---

## 7. Content Stream Architecture

---

## 8. Resource Resolution Architecture

---

## 9. Coordinate System Architecture

---

## 10. Font Subsetting Architecture

---

## 11. Streaming Architecture

---

## 12. Extension System Architecture

---

## 13. Writer Architecture

---

## 14. PDF/A Compliance Architecture

---

## 15. Error Recovery Architecture

---

## 16. Advanced Features Architecture (v2.4+)

### 16.1 Scripting Engine
### 16.2 Content Editor & Spatial Indexing
### 16.3 Digital Signature Validation (LTV)
### 16.4 XFA Support
### 16.5 Color Management (CMM)
### 16.6 Geospatial Support
### 16.7 Compliance Engine

---

## 17. High-Level Builder API

---

## 18. Concurrency Model

### 18.1 Thread Safety
### 18.2 Parallel Processing Opportunities

---

## 19. Security Architecture

### 19.1 Security Limits
### 19.2 Input Validation

---

## 20. Layout Engine

### 20.1 Engine Architecture
### 20.2 Supported Features

---

## 21. Testing Strategy

### 20.1 Test Corpus
### 20.2 Test Categories

---

## 22. Performance Targets

---

## 23. Roadmap

---

## 24. Dependencies

---

## 25. API Stability

---

## 26. References

---

## 27. Appendix: Example Workflows

### Example 1: Parse and Extract Text
### Example 2: Create Simple PDF
### Example 3: Font Subsetting
### Example 4: PDF/A Conversion

---

**End of Design Document v2.0**
