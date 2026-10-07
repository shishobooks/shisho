# KePub Format

`pkg/kepub` converts EPUBs (`(*Converter).ConvertEPUB`, `converter.go`) and CBZs (`(*Converter).ConvertCBZWithMetadata`, `cbz.go`, a method, not a package function) into Kobo EPUBs (`.kepub.epub`). `pkg/filegen` wraps them (`GetKepubGenerator`, `SupportsKepub`, `kepub_cbz.go`); the download cache keys KePub output separately from the original through the fingerprint's `format`. The injected `kobo.js` is the `koboJS` const in `converter.go`.

Both converters write the `mimetype` entry first and uncompressed (`zip.Store`), as the EPUB spec requires; the EPUB path then skips the source's copy so it is not written twice.

## EPUB conversion

- **Content files are found by string-parsing the OPF manifest**, not encoding/xml, to avoid namespace trouble: `application/xhtml+xml` and `text/html` items, minus NCX. `kobo.js` is added at the EPUB root only when absent, and each content file references it by a computed relative path.
- **Conversion is idempotent**: converting an existing KePub must not double-wrap spans. Text is preserved exactly; only wrapping is added.
- `TransformOPF` adds `cover-image` to the cover item's `properties`, found through `<meta name="cover">` (either attribute order) or an existing `cover-image` item. The idempotence check reads the `properties` tokens, not the whole tag, so an id or href containing `cover-image` does not count.

## Span wrapping (`content.go`)

- Span ids are `kobo.{paragraph}.{sentence}` from a fresh `SpanCounter` per file, starting at 1. Block elements (`p`, `ol`, `ul`, `table`, `h1` to `h6`) mark a deferred paragraph boundary (`markParagraphBoundary`); images increment immediately (`incrementParagraph`). The sentence counter resets with each paragraph.
- Text splits after `.!?:` followed by whitespace (quotes after the punctuation stay with the sentence) and on newlines; whitespace runs become their own spans, and a non-breaking space is wrapped.
- Never wrap inside `script`, `style`, `pre`, `code`, `svg`, or `math`, nor whitespace-only or empty nodes.
- The body is wrapped in `div#book-columns > div#book-inner` (kobo.js paginates on them) and a `kobostylehacks` style is injected.

## XHTML round-trip (`xhtml.go`)

Go's `html.Parse` treats input as HTML5, dropping the XML declaration and emitting void elements unclosed. The converter strips the declaration before parsing and restores it afterwards, and rewrites void elements (`area`, `base`, `br`, `col`, `embed`, `hr`, `img`, `input`, `link`, `meta`, `param`, `source`, `track`, `wbr`), empty `script`, and empty anchors as self-closing XHTML. Any new output path must go through the same post-processing so the result stays valid XHTML.

## CBZ conversion (`cbz.go`)

- Pages are image entries (`.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`, hidden files skipped) in `naturalLess` order (`page2` before `page10`). `pkg/cbz` and `pkg/cbzpages` use plain byte order, so for names like those the page indexes differ; see `pkg/cbz/AGENTS.md`.
- Output is a fixed-layout EPUB (`rendition:layout` `pre-paginated`, `rendition:spread` `landscape`) with NCX, `nav.xhtml`, and one KCC-style XHTML page per image whose viewport matches the image size, alternating left and right spreads.
- **`ProcessImageForEreader` is shared with `pkg/filegen/cbz.go`.** It shrinks images larger than the Kobo Libra Color screen (1264x1680) keeping aspect ratio, converts PNG to JPEG (quality 85), and quantizes grayscale pages to Kobo's 16-level palette (`isGrayscaleImage`, `quantizeToKoboPalette`). Smaller images and anything that fails to decode pass through unchanged. Images are processed by a worker pool of `min(CPU count, image count)` that honors context cancellation.
- Authors are deduplicated by `name|role`, and metadata values are XML-escaped.
