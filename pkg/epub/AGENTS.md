# EPUB Format

`pkg/epub` parses EPUBs; `pkg/filegen/epub.go` (`EPUBGenerator`) writes them. The KePub, OPDS, eReader, Kobo, and Share Link downloads are all built on the generated EPUB, so a generator bug ships everywhere.

## Writing OPF

- **Round-trip fidelity.** `modifyOPF` unmarshals the whole OPF into `opfPackage`, edits it, and marshals it back, so anything the structs do not carry is silently dropped. Every OPF struct carries an ``Attrs opfAttrs `xml:",any,attr"` `` catch-all; give any new OPF struct the same field instead of modeling attributes one by one.
- **Any new code path that marshals `opfPackage` must call `writeRefinements`**, which turns parse-only role, file-as, and scheme fields into the form the package's EPUB version allows. Skipped, they print without the `opf:` prefix.
- New OPF handling extends `pkg/filegen/epub_opf_fidelity_test.go` and, when it differs between EPUB 2 and 3, `epub_opf_version_test.go`.
