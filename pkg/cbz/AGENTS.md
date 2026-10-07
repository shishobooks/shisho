# CBZ Format

`pkg/cbz` parses CBZ archives and their `ComicInfo.xml` (`cbz.go`) and detects chapters (`chapters.go`); `pkg/filegen/cbz.go` (`CBZGenerator`) writes them. The ComicInfo-element-to-field mapping lives in those files; author role constants are `AuthorRole*` in `pkg/models/person.go`. CBZ-to-KePub conversion is `pkg/kepub`.

## Pages are 0-indexed byte-order image names

Page N is the Nth `.jpg`/`.jpeg`/`.png`/`.gif`/`.webp` entry after a plain byte-order sort of entry names (`getSortedImageFiles` in `pkg/cbz/cbz.go` and again in `pkg/cbzpages/cache.go`), so `page10` sorts before `page2`. Cover page, chapter start pages, and served pages all depend on this order, so every sort must stay identical. `pkg/kepub/cbz.go` orders pages with `naturalLess` instead. Stored page numbers are 0-indexed, and the UI shows them 1-indexed.

## Parsing

- **Creators** come from eight role elements (`Writer`, `Penciller`, `Inker`, `Colorist`, `Letterer`, `CoverArtist`, `Editor`, `Translator`), each comma-separated; every name keeps the role of the element it came from.
- **Publisher** prefers `<Imprint>` over `<Publisher>`. The imprint is folded into Publisher when parsing, and the generator never writes `Imprint`.
- **Cover** is the `<Page Type="FrontCover">` index, else `InnerCover`, else page 0; out-of-range indexes are ignored.
- **Page count** counts image entries, not `<PageCount>`.
- **Chapters**: folder-based first (each immediate parent folder becomes a chapter titled with the folder name, used only with 2+ folders), else the filename pattern `(?i)ch(?:apter)?[\s_-]*(\d+)` grouping consecutive pages as "Chapter N", else none.

## Series number and unit

ComicInfo `<Number>` carries a single number or an increasing range (`1-3`, decimals allowed) but cannot encode volume versus chapter. Filenames can: `v01`, `vol. 1-3`, `#1-3`, bare trailing numbers (volume) and `c05-08`, `ch. 5`, `chapter 5` (chapter), with hyphen, en dash, or em dash ranges. Ambiguous forms (`#001`, bare numbers) default to volume so rescans of existing files keep their meaning.

- `pkg/fileutils/naming.go` owns the pipeline: `NormalizeSeriesNumberInTitle` normalizes the title (`Title v003`, `Title c005-008`) and returns the unit, `ExtractSeriesFromTitle` splits it back, and `formatSeriesNumber` / `IsOrganizedName` format `v{N}` / `c{N}` suffixes, which also name organized folders (`Title c042/Title.cbz`).
- **Start, end, and unit are one atomic group from one source.** Never combine ComicInfo endpoints with a filename-derived unit; the unit survives only through a source that can represent it (filename, sidecar).
- Whole numbers print as `1`, decimals as `1.5`, ranges as `1.5-3.5` in ComicInfo and `v001.5-003.5` in organized names.

## Generation

- **Only modeled ComicInfo elements survive.** The generator unmarshals into `cbzComicInfo` (in `pkg/filegen/cbz.go`) and marshals it back, with no catch-all, so an element the struct does not declare is dropped. Add the element to the struct to preserve it.
- The source's genres and tags stay when the book has none. Authors with no role go to `<Writer>`. Series is the first `BookSeries` by sort order.
- `updateCoverPage` clears every existing `FrontCover` type and marks the new cover, creating `<Pages>` when missing.
- Images are processed in parallel with `kepub.ProcessImageForEreader` (the same resize, PNG-to-JPEG, and manga grayscale steps as KePub conversion); undecodable images and non-image entries pass through unchanged.
