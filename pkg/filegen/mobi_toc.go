package filegen

import (
	"context"
	"io"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
)

// mobiTOCEdits turns the file's stored chapters into edits to the tables
// of contents in a MOBI or AZW3 file, the way rewriteTOCs does for an EPUB.
// A scan stores the chapters of one book's index, the first mobi.ReadTOC
// returns; the stored chapters are aligned to it, and the edits are carried
// to the other book of a combo file by title, since the two books' locators
// differ. It returns nil when there is nothing to write.
func mobiTOCEdits(ctx context.Context, src io.ReaderAt, size int64, stored []*models.Chapter) map[mobi.Book]*mobi.TOCEdit {
	chapters := chapterTree(stored)
	if len(chapters) == 0 {
		return nil
	}
	log := logger.FromContext(ctx)
	tocs, err := mobi.ReadTOC(src, size)
	if err != nil {
		log.Warn("failed to read MOBI table of contents, leaving it as written", logger.Data{
			"category": "mobi_toc_write",
			"error":    err.Error(),
		})
		return nil
	}
	if len(tocs) == 0 {
		return nil
	}

	primary, indexes := mobiTOCEntries(tocs[0].Entries)
	changes, ok := alignTOC(primary, chapters)
	if !ok {
		log.Warn("MOBI chapters do not match its table of contents, leaving it as written", logger.Data{
			"category": "mobi_toc_write",
		})
		return nil
	}
	if changes.empty() {
		return nil
	}

	edits := map[mobi.Book]*mobi.TOCEdit{tocs[0].Book: mobiTOCEdit(changes, indexes)}
	for _, other := range tocs[1:] {
		entries, otherIndexes := mobiTOCEntries(other.Entries)
		carried := carryChanges(primary, changes, &tocDoc{entries: entries})
		edits[other.Book] = mobiTOCEdit(carried, otherIndexes)
	}
	return edits
}

// mobiTOCEntries turns index entries into tocEntries, with no package
// path, so carryChanges pairs them by title, and maps each back to its
// index.
func mobiTOCEntries(entries []*mobi.TOCEntry) ([]*tocEntry, map[*tocEntry]int) {
	indexes := map[*tocEntry]int{}
	var convert func([]*mobi.TOCEntry) []*tocEntry
	convert = func(entries []*mobi.TOCEntry) []*tocEntry {
		out := make([]*tocEntry, 0, len(entries))
		for _, e := range entries {
			te := &tocEntry{title: e.Title, href: e.Locator, children: convert(e.Children)}
			indexes[te] = e.Index
			out = append(out, te)
		}
		return out
	}
	return convert(entries), indexes
}

// logSkippedMOBITOC is the mobi.Metadata.OnTOCSkipped of a generated
// download.
func logSkippedMOBITOC(ctx context.Context) func(*mobi.TOCSkipError) {
	return func(e *mobi.TOCSkipError) {
		book := "MOBI6"
		if e.Book == mobi.BookKF8 {
			book = "KF8"
		}
		logger.FromContext(ctx).Warn("failed to write MOBI chapter edits, leaving the table of contents as written", logger.Data{
			"category": "mobi_toc_write",
			"book":     book,
			"error":    e.Error(),
		})
	}
}

func mobiTOCEdit(changes tocChanges, indexes map[*tocEntry]int) *mobi.TOCEdit {
	edit := &mobi.TOCEdit{Renamed: map[int]string{}, Deleted: map[int]bool{}}
	for e, title := range changes.renamed {
		edit.Renamed[indexes[e]] = title
	}
	for e := range changes.deleted {
		edit.Deleted[indexes[e]] = true
	}
	return edit
}
