# CBZ Format

`pkg/cbz` parses CBZ archives and their `ComicInfo.xml`; `pkg/filegen/cbz.go` (`CBZGenerator`) writes them; CBZ-to-KePub conversion is `pkg/kepub`.

- **Page numbers are 0-indexed positions in `PageImages`.** Cover page, chapter start pages, and served pages all depend on that list, so code in any package that turns a page number into an archive entry calls it instead of filtering or sorting entries itself. The UI shows pages 1-indexed.
- **The series unit (volume or chapter) comes only from filenames and sidecars.** ComicInfo `<Number>` cannot encode it, so never pair ComicInfo endpoints with a filename-derived unit: start, end, and unit are one group from one source.
