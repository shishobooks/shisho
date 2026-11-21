package shishoapp

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

// buildEntries turns log entries into the feed's entries for userID,
// reading each record as it is now in tx, the snapshot the entries came
// from. A record the entry doesn't mark deleted always exists in that
// snapshot, since its delete would have moved its entry.
func buildEntries(ctx context.Context, tx bun.IDB, userID int, entries []*changefeed.Entry) ([]FeedEntry, error) {
	var bookIDs, libraryIDs, positionFileIDs, readThroughIDs []int
	for _, entry := range entries {
		if entry.Deleted {
			continue
		}
		switch entry.Kind {
		case changefeed.KindBook:
			bookIDs = append(bookIDs, entry.RecordID)
		case changefeed.KindLibrary:
			libraryIDs = append(libraryIDs, entry.RecordID)
		case changefeed.KindReadingPosition:
			positionFileIDs = append(positionFileIDs, entry.RecordID)
		case changefeed.KindReadThrough:
			readThroughIDs = append(readThroughIDs, entry.RecordID)
		}
	}
	books, err := loadBooks(ctx, tx, bookIDs)
	if err != nil {
		return nil, err
	}
	libraries, err := loadLibraries(ctx, tx, userID, libraryIDs)
	if err != nil {
		return nil, err
	}
	positions, err := loadPositions(ctx, tx, userID, positionFileIDs)
	if err != nil {
		return nil, err
	}
	readThroughs, err := loadReadThroughs(ctx, tx, userID, readThroughIDs)
	if err != nil {
		return nil, err
	}

	out := make([]FeedEntry, 0, len(entries))
	for _, entry := range entries {
		item := FeedEntry{Kind: entry.Kind, ID: entry.RecordID, Deleted: entry.Deleted}
		if entry.LibraryID != nil {
			item.LibraryID = *entry.LibraryID
		}
		if !entry.Deleted {
			switch entry.Kind {
			case changefeed.KindBook:
				book, ok := books[entry.RecordID]
				if !ok {
					continue
				}
				item.Book = book
			case changefeed.KindLibrary:
				library, ok := libraries[entry.RecordID]
				if !ok {
					continue
				}
				item.Library = library
			case changefeed.KindReadingPosition:
				position, ok := positions[entry.RecordID]
				if !ok {
					continue
				}
				item.ReadingPosition = position
			case changefeed.KindReadThrough:
				readThrough, ok := readThroughs[entry.RecordID]
				if !ok {
					continue
				}
				item.ReadThrough = readThrough
			default:
				// A kind this server doesn't build documents for yet.
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func loadLibraries(ctx context.Context, tx bun.IDB, userID int, ids []int) (map[int]*FeedLibrary, error) {
	out := map[int]*FeedLibrary{}
	if len(ids) == 0 {
		return out, nil
	}
	var libraries []*models.Library
	if err := tx.NewSelect().Model(&libraries).Where("l.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	var settings []*models.UserLibrarySettings
	err := tx.NewSelect().Model(&settings).
		Where("uls.user_id = ?", userID).
		Where("uls.library_id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sorts := map[int]string{}
	for _, setting := range settings {
		// A spec the web app wouldn't apply is no sort at all.
		if setting.SortSpec == nil {
			continue
		}
		if _, err := sortspec.Parse(*setting.SortSpec); err == nil {
			sorts[setting.LibraryID] = *setting.SortSpec
		}
	}
	for _, library := range libraries {
		out[library.ID] = &FeedLibrary{
			ID: library.ID, Name: library.Name, CoverAspectRatio: library.CoverAspectRatio,
			SortSpec: sorts[library.ID],
		}
	}
	return out, nil
}

func loadBooks(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedBook, error) {
	out := map[int]*FeedBook{}
	if len(ids) == 0 {
		return out, nil
	}
	var books []*models.Book
	err := tx.NewSelect().Model(&books).
		Relation("Library").
		Relation("Authors", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("a.sort_order ASC")
		}).
		Relation("Authors.Person").
		Relation("BookSeries", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("bs.sort_order ASC")
		}).
		Relation("BookSeries.Series").
		Relation("Files", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("f.id ASC")
		}).
		Relation("Files.Narrators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
		Relation("Files.Chapters", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("ch.sort_order ASC", "ch.id ASC")
		}).
		Where("b.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, book := range books {
		out[book.ID] = bookDocument(book)
	}
	return out, nil
}

// bookDocument is book as the app keeps it. The triggers move a Book's
// entry when anything read here changes.
func bookDocument(book *models.Book) *FeedBook {
	doc := &FeedBook{
		ID:        book.ID,
		LibraryID: book.LibraryID,
		Title:     book.Title,
		Subtitle:  book.Subtitle,
		SortTitle: book.SortTitle,
		CreatedAt: book.CreatedAt,
		Authors:   []FeedAuthor{},
		Series:    []FeedSeriesMembership{},
		Files:     []FeedFile{},
	}
	for _, author := range book.Authors {
		if author.Person == nil {
			continue
		}
		doc.Authors = append(doc.Authors, FeedAuthor{
			PersonID: author.PersonID,
			Name:     author.Person.Name,
			SortName: author.Person.SortName,
			Role:     author.Role,
		})
	}
	for _, membership := range book.BookSeries {
		if membership.Series == nil {
			continue
		}
		doc.Series = append(doc.Series, FeedSeriesMembership{
			SeriesID:  membership.SeriesID,
			Name:      membership.Series.Name,
			SortName:  membership.Series.SortName,
			Number:    membership.SeriesNumber,
			NumberEnd: membership.SeriesNumberEnd,
			Unit:      membership.SeriesNumberUnit,
		})
	}
	for _, file := range book.Files {
		narrators := []FeedNarrator{}
		for _, narrator := range file.Narrators {
			if narrator.Person == nil {
				continue
			}
			narrators = append(narrators, FeedNarrator{
				PersonID: narrator.PersonID, Name: narrator.Person.Name, SortName: narrator.Person.SortName,
			})
		}
		doc.Files = append(doc.Files, FeedFile{
			CoverVersion:             fileCoverVersion(file),
			Narrators:                narrators,
			Chapters:                 chapterDocuments(file.Chapters),
			ID:                       file.ID,
			FileType:                 file.FileType,
			FileRole:                 file.FileRole,
			SizeBytes:                file.FilesizeBytes,
			DisplayName:              file.ResolveDisplayName(),
			ReleaseDate:              file.ReleaseDate,
			PageCount:                file.PageCount,
			AudiobookDurationSeconds: file.AudiobookDurationSeconds,
		})
	}
	aspect := ""
	if book.Library != nil {
		aspect = book.Library.CoverAspectRatio
	}
	doc.CoverVersion = covers.CacheKey(book.Files, aspect)
	return doc
}

// chapterDocuments orders chapters as a table of contents: each parent's
// children, by their sort order, right after it.
func chapterDocuments(chapters []*models.Chapter) []FeedChapter {
	children := map[int][]*models.Chapter{}
	var roots []*models.Chapter
	byID := map[int]bool{}
	for _, chapter := range chapters {
		byID[chapter.ID] = true
	}
	for _, chapter := range chapters {
		if chapter.ParentID != nil && byID[*chapter.ParentID] {
			children[*chapter.ParentID] = append(children[*chapter.ParentID], chapter)
		} else {
			roots = append(roots, chapter)
		}
	}
	out := make([]FeedChapter, 0, len(chapters))
	var add func(list []*models.Chapter)
	add = func(list []*models.Chapter) {
		for _, chapter := range list {
			out = append(out, FeedChapter{
				ID: chapter.ID, ParentID: chapter.ParentID, Title: chapter.Title,
				StartTimestampMs: chapter.StartTimestampMs, StartPage: chapter.StartPage, Href: chapter.Href,
			})
			add(children[chapter.ID])
		}
	}
	add(roots)
	return out
}

// loadPositions reads userID's Reading Positions in fileIDs, by File.
func loadPositions(ctx context.Context, tx bun.IDB, userID int, fileIDs []int) (map[int]*FeedReadingPosition, error) {
	out := map[int]*FeedReadingPosition{}
	if len(fileIDs) == 0 {
		return out, nil
	}
	var positions []*models.ReadingPosition
	err := tx.NewSelect().Model(&positions).
		Where("rp.user_id = ?", userID).
		Where("rp.file_id IN (?)", bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, position := range positions {
		out[position.FileID] = &FeedReadingPosition{
			FileID: position.FileID, CFI: position.CFI, Page: position.Page, Seconds: position.Seconds,
			Percentage: position.Percentage, OccurredAt: position.OccurredAt.UTC(), Source: position.Source,
			DeviceID: position.SourceDeviceID,
		}
	}
	return out, nil
}

// loadReadThroughs reads userID's Read-throughs among ids.
func loadReadThroughs(ctx context.Context, tx bun.IDB, userID int, ids []int) (map[int]*FeedReadThrough, error) {
	out := map[int]*FeedReadThrough{}
	if len(ids) == 0 {
		return out, nil
	}
	var readThroughs []*models.ReadThrough
	err := tx.NewSelect().Model(&readThroughs).
		Where("rt.user_id = ?", userID).
		Where("rt.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, rt := range readThroughs {
		out[rt.ID] = &FeedReadThrough{
			ID: rt.ID, BookID: rt.BookID, BeginDay: rt.BeginDay, EndDay: rt.EndDay, Outcome: rt.Outcome, Paused: rt.Paused,
		}
	}
	return out, nil
}

// fileCoverVersion keys file's own cover as covers.CacheKey keys a Book's,
// or is empty when the File has no cover.
func fileCoverVersion(file *models.File) string {
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return ""
	}
	return fmt.Sprintf("%d-%d", file.ID, file.UpdatedAt.Unix())
}
