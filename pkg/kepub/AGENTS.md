# KePub Format

`pkg/kepub` converts EPUBs and CBZs into Kobo EPUBs (`.kepub.epub`); `pkg/filegen` wraps the converters for downloads.

- **Conversion is idempotent.** Converting an existing KePub must not double-wrap spans, and text is preserved exactly: only wrapping is added.
- **Every content output path goes through `XHTMLProcessor`** (`PreProcess` before `html.Parse`, `PostProcess` after rendering). Go's HTML5 parser drops the XML declaration and leaves void elements unclosed, so skipping either step produces invalid XHTML.
- **Every archive writer puts the `mimetype` entry first and uncompressed (`zip.Store`)**, as the EPUB spec requires, and writes it only once.
