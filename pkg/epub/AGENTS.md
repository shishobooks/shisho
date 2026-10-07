# EPUB Format

`pkg/epub` parses EPUBs (`opf.go` for metadata and the `Parse` entry point, `nav.go` for chapters); `pkg/filegen/epub.go` (`EPUBGenerator`) writes them. The OPF-element-to-field mapping lives in `opf.go`. The KePub, OPDS, eReader, Kobo, and Share Link downloads are all built on the generated EPUB, so a generator bug ships everywhere.

## Parsing

- **Title** prefers the `dc:title` with `id="title-main"` or a `title-type="main"` refinement.
- **Authors** are creators with role `aut`, from the `role` attribute in any namespace or the creator's `role` refinement; when only one creator exists it counts regardless of role.
- **Publisher** is overridden by `ibooks:imprint` or `<meta name="imprint">` when present.
- **URL** is `<meta name="shisho:url">` (what the generator writes), then the first `http(s)` value of `dc:relation`, then `dc:source`.
- **Identifiers** take their type from `scheme` (any namespace), then the `identifier-type` refinement, then detection from the value; unknown types are skipped.
- **Genres are `dc:subject`; tags are the comma-separated `calibre:tags` meta.** They are stored separately and never mixed.
- **Chapters** come from the EPUB 3 nav document (manifest `properties="nav"`), else the NCX named by the spine `toc` attribute, else none.

## Cover lookup (parser and writer share it)

The cover is the manifest item named by the last `<meta name="cover" content="ID"/>`, else the item whose `properties` include `cover-image`, else an item with id `cover`, `cover-image`, or `coverimage` (case-insensitive). The writer (`findCoverImage`) also requires an `image/*` media type at every step, so a meta naming an XHTML cover page is never overwritten with image bytes. Hrefs resolve against the OPF directory after percent-decoding, so `../Images/cover.jpg` works.

When Shisho has a cover and the package has none, `addCoverImage` adds `cover.<ext>` next to the OPF (suffixed if taken) with an unused manifest id, marked `properties="cover-image"` in EPUB 3 or with `<meta name="cover">` in EPUB 2. A `<meta name="cover">` that pointed at nothing usable is repointed, not duplicated.

## Generation

**Round-trip fidelity.** `modifyOPF` unmarshals the whole OPF into `opfPackage`, edits it, and marshals it back, so anything the structs do not carry is silently dropped.

- Every OPF struct (package, spine, manifest item, itemref, title, creator, identifier, meta) carries an ``Attrs opfAttrs `xml:",any,attr"` `` catch-all, which preserves `properties`, `linear`, `page-progression-direction`, `prefix`, `xml:lang`, and the rest. Give any new OPF struct the same field instead of modeling attributes one by one.
- `opfAttrs.UnmarshalXMLAttr` drops `xmlns` and `xmlns:*`, because encoding/xml cannot re-emit namespace declarations from `,any,attr` (it prints duplicates or invented `_xmlns:` prefixes). The struct tags declare namespaces.
- **The `dc:identifier` whose `id` matches `package@unique-identifier` is always kept**, or the package points at a missing id. Its value follows the file: a file identifier with the same normalized value is not written twice (`urn:isbn:978...` equals `978...`), one of the same kind (ISBN-10 and ISBN-13 are one kind) replaces a stale value, and with no identifier of that kind the source value stays.
- A retitled `dc:title` drops its `xml:lang`, `dir`, and `opf:file-as`, which described the old text. When the file has a language, an existing package `xml:lang` is set to it.
- Extend `pkg/filegen/epub_opf_fidelity_test.go` when adding OPF handling.

**Version-dependent attributes.** `isEPUB3` reads `package@version`: 3.x and later is EPUB 3, missing or 2.x is EPUB 2.

- `opfCreator.Role`/`FileAs` and `opfID.Scheme` match `role`, `file-as`, and `scheme` in any namespace and are parse-only. `writeRefinements` moves them out and clears them before marshal, because as plain fields they would print without the `opf:` prefix. **Any new code path that marshals `opfPackage` must call `writeRefinements`.**
- EPUB 2 writes `opf:role`, `opf:file-as`, `opf:scheme` in the OPF namespace (`opfAttrs.setOPF`) and removes other spellings from `Attrs` so nothing prints twice.
- EPUB 3 forbids them on `dc:` elements: the writer ensures an id and writes `<meta refines="#id">` with `property="role" scheme="marc:relators"`, `file-as`, or `identifier-type`. An identical existing refinement is kept with its attributes; a changed role or file-as takes the new value and drops stale attributes; an existing `identifier-type` always wins (sources often use ONIX codes for the same type).
- A role refinement counts only with no scheme or `scheme="marc:relators"`; one in another vocabulary is no role, so a same-named book author reuses that element instead of being written twice. Non-author creators and all `dc:contributor` elements are kept, and a source author matching a book author keeps its element (id, attributes, `alternate-script` refinements). Dropping an author or identifier also drops the metas refining its id.
- An unchanged `dc:title` keeps its attributes; in EPUB 3 its `opf:file-as` moves into a `file-as` refinement (an existing one wins).
- `pkg/filegen/epub_opf_version_test.go` covers both versions and validates with `xmllint --noout` and epubcheck when installed.

**Other writer behavior.** Authors are written as `aut` in sort order with file-as from the person's sort name. Series is written in both Calibre (`calibre:series`, `calibre:series_index`) and EPUB 3 (`belongs-to-collection` with `collection-type` and `group-position`) form; whole numbers print as `1`, decimals as `1.5`. The source's genres stay when the book has none.
