# Repository Guidelines

## Project Structure & Module Organization
- Three-tier IR lives in `ir/`: Raw (`ir/raw`), Decoded (`ir/decoded`), and Semantic (`ir/semantic`), wired by `ir/pipeline.go`.
- Ingestion: `scanner/` tokenizes, `parser/` orchestrates raw parsing, `xref/` resolves offsets, and `security/` handles encryption/decryption.
- Streams: decoders in `filters/`, content operators in `contentstream/`, coordinate helpers in `coords/`, and resource resolution in `resources/`.
- Output: `builder/` offers a high-level authoring API and feeds `writer/` for serialization; PDF/A and compliance features live in `pdfa/`.
- Support: `extensions/` for plugins, `recovery/` for error handling, `observability/` for logging/metrics, with fixtures in `testdata/` and CLIs in `cmd/` (e.g., `cmd/scantest`).

## Build, Test, and Development Commands
- `go test ./...` — full suite across parser, filters, writer, and pipeline.
- `go test ./... -run <Name>` — target a specific test group (e.g., `Pipeline`, `ASCIIHex`).
- `go vet ./...` — static checks for common issues.
- `go fmt ./...` — canonical formatting before sending changes.
- `go run ./cmd/scantest` — quick sanity pass when touching scanner/xref behavior.

## Coding Style & Naming Conventions
- Go 1.25-compatible module; prefer standard library plus local packages.
- Follow idiomatic Go: short functions, early returns, explicit error handling, and doc comments on exported symbols.
- Constructors use `NewX`/`NewXFromY`; interfaces name capabilities (`Handler`, `Resolver`). File names reflect the primary type (`decoder_impl.go`, `xref.go`), with tests in `_test.go`.
- Pass `context.Context` to potentially blocking or heavy operations (I/O, decompression).

## Testing Guidelines
- Use table-driven tests; keep generated PDFs or fixtures in `testdata/`. For small repros, build in-memory documents (see `writer/writer_impl_test.go`).
- Name tests `TestFeature_Subfeature`; add subtests for malformed PDFs, filter edges, backpressure, and incremental/xref scenarios.
- When adding filters or parser features, include end-to-end pipeline coverage (`ir/pipeline_test.go`) to lock decoded output.

## Commit & Pull Request Guidelines
- Commit subjects are imperative and concise (“Add ASCII85 decoder padding”); group logically related changes.
- Describe intent, approach, and verification in the body, referencing commands run (`go test ./...`, focused cases). Link issues and attach minimal repro PDFs when relevant.
- Before review: ensure `go fmt ./...`, `go vet ./...`, and targeted tests for affected areas pass; document new public APIs or semantics.

## Security & Configuration Tips
- Do not commit real secrets or proprietary PDFs; prefer synthetic/redacted fixtures. Centralize encryption assumptions in `security/`.
- Validate lengths/offsets and bound reads in scanners, filters, and content streams to avoid resource exhaustion; keep defensive defaults and document any graceful degradation paths.
