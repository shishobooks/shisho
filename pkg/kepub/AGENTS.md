# KePub Format

`pkg/kepub` converts EPUBs into Kobo EPUBs (`.kepub.epub`), and strips a KePub back to plain EPUB (`Strip`). `pkg/fixedlayout` writes the plain fixed-layout EPUB that both the CBZ to KePub and CBZ to EPUB Conversions deliver. `pkg/filegen` runs them as Conversion steps for downloads.

- **Conversion is idempotent.** Converting an existing KePub must not double-wrap spans, and text is preserved exactly: only wrapping is added.
- **Content rewrites read `html.NewTokenizer` tokens and copy every token and attribute they don't change as it was**, never `html.Parse` and `html.Render`: Go's HTML5 parser drops the XML declaration, restructures XHTML that isn't valid HTML (a div in a p, a self-closed `<a/>`), and re-renders the rest. Read the tokens through `eachToken`, never a tokenizer of your own: it turns on `AllowCDATA`, without which a CDATA section holding a `>` ends there and the rest reads as markup, and calls `NextIsNotRawText` after a self-closing tag, without which a self-closed `<script/>` or `<title/>` swallows what follows as raw text.
- **Every archive writer puts the `mimetype` entry first and uncompressed (`zip.Store`)**, as the EPUB spec requires, and writes it only once.
