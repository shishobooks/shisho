# PDF Format

`pkg/pdf` reads PDFs with pdfcpu (metadata) and PDFium (cover rendering and outlines); `pkg/filegen/pdf.go` writes downloads. Page numbers are 0-indexed everywhere except pdfcpu's bookmark API.

- **Get PDFium only through `PdfiumInstance(timeout)`** and `defer instance.Close()`. The shared pool holds one instance on purpose (see `initPdfiumPool`); raise it only after measuring per-instance memory and seeing real contention in the warn logs.
- **Pick the timeout for where the caller runs; never change a shared default.** User-facing requests pass `InteractivePdfiumTimeout`; Scan-time work in this package uses `scanPdfiumTimeout`, whose comment explains why it is long.
- **Before reaching for pdfcpu's `api` helpers, read the comments in `pkg/filegen/pdf.go`**: since pdfcpu 0.14, `api.AddPropertiesFile` rejects several info keys and `api.AddKeywordsFile` merges with the source keywords, so neither fits.
- Tests that hold the PDFium instance or change package state (`scanPdfiumTimeout`, the logger) must not call `t.Parallel()`: the pool, the variable, and the logger output are process-global.
