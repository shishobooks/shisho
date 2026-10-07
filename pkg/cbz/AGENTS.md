# CBZ Format

`pkg/cbz` parses CBZ archives and their `ComicInfo.xml`; `pkg/filegen/cbz.go` (`CBZGenerator`) writes them; CBZ-to-KePub conversion is `pkg/kepub`.

- **Page numbers are 0-indexed positions in a byte-order sort of image entry names**, so `page10` comes before `page2`. Cover page, chapter start pages, and served pages all depend on that order, so any new code that turns a page number into an archive entry must sort the same way (`getSortedImageFiles`). The UI shows pages 1-indexed.
- **The series unit (volume or chapter) comes only from filenames and sidecars.** ComicInfo `<Number>` cannot encode it, so never pair ComicInfo endpoints with a filename-derived unit: start, end, and unit are one group from one source.
