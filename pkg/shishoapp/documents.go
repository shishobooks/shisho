package shishoapp

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// buildEntries turns log entries into the feed's entries, reading each
// record as it is now in tx, the snapshot the entries came from. A record
// the entry doesn't mark deleted always exists in that snapshot, since its
// delete would have moved its entry.
func buildEntries(ctx context.Context, tx bun.IDB, entries []*changefeed.Entry) ([]FeedEntry, error) {
	var bookIDs, libraryIDs []int
	for _, entry := range entries {
		if entry.Deleted {
			continue
		}
		switch entry.Kind {
		case changefeed.KindBook:
			bookIDs = append(bookIDs, entry.RecordID)
		case changefeed.KindLibrary:
			libraryIDs = append(libraryIDs, entry.RecordID)
		}
	}
	books, err := loadBooks(ctx, tx, bookIDs)
	if err != nil {
		return nil, err
	}
	libraries, err := loadLibraries(ctx, tx, libraryIDs)
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
			default:
				// A kind this server doesn't build documents for yet.
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func loadLibraries(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedLibrary, error) {
	out := map[int]*FeedLibrary{}
	if len(ids) == 0 {
		return out, nil
	}
	var libraries []*models.Library
	if err := tx.NewSelect().Model(&libraries).Where("l.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	for _, library := range libraries {
		out[library.ID] = &FeedLibrary{ID: library.ID, Name: library.Name, CoverAspectRatio: library.CoverAspectRatio}
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
		doc.Files = append(doc.Files, FeedFile{
			ID:        file.ID,
			FileType:  file.FileType,
			FileRole:  file.FileRole,
			SizeBytes: file.FilesizeBytes,
		})
	}
	aspect := ""
	if book.Library != nil {
		aspect = book.Library.CoverAspectRatio
	}
	doc.CoverVersion = covers.CacheKey(book.Files, aspect)
	return doc
}
