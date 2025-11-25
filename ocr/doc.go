package ocr

// Package ocr defines abstraction layers for plugging third-party OCR engines
// (for example, Tesseract or cloud services) into the PDF processing pipeline.
// The interfaces are intentionally small and transport-agnostic so engines can
// be backed by local binaries, native libraries, or remote APIs without leaking
// provider-specific concerns into callers.
