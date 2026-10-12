# PDF Format

`pkg/pdf` reads PDFs with pdfcpu (metadata) and PDFium (cover rendering and outlines). Downloads (`pkg/filegen/pdf.go`) are the original bytes plus an incremental update built by `pkg/pdf/incremental`. Page numbers are 0-indexed.

- **Get PDFium only through `PdfiumInstance(timeout)`** and `defer instance.Close()`. The shared pool holds one instance on purpose (see `initPdfiumPool`); raise it only after measuring per-instance memory and seeing real contention in the warn logs.
- **Pick the timeout for where the caller runs; never change a shared default.** User-facing requests pass `InteractivePdfiumTimeout`; Scan-time work in this package uses `scanPdfiumTimeout`, whose comment explains why it is long.
- **Never rewrite a PDF through pdfcpu to produce a download.** A rewrite re-optimizes the file, reads all of it before the first byte can be sent, and breaks digital signatures; add what a download needs to the update in `pkg/filegen/pdf.go` instead.
- Tests that hold the PDFium instance or change package state (`scanPdfiumTimeout`, the logger) must not call `t.Parallel()`: the pool, the variable, and the logger output are process-global.
