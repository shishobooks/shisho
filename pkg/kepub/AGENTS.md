# KePub Format

`pkg/kepub` converts EPUBs into Kobo EPUBs (`.kepub.epub`). `pkg/fixedlayout` writes the plain fixed-layout EPUB that both the CBZ to KePub and CBZ to EPUB Conversions deliver. `pkg/filegen` runs both as Conversion steps for downloads.

- **Conversion is idempotent.** Converting an existing KePub must not double-wrap spans, and text is preserved exactly: only wrapping is added.
- **Every content output path goes through `XHTMLProcessor`** (`PreProcess` before `html.Parse`, `PostProcess` after rendering). Go's HTML5 parser drops the XML declaration and leaves void elements unclosed, so skipping either step produces invalid XHTML.
- **Every archive writer puts the `mimetype` entry first and uncompressed (`zip.Store`)**, as the EPUB spec requires, and writes it only once.
