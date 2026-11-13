package reading

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// HistoryOptions filters and pages ListHistory.
type HistoryOptions struct {
	Outcome *string
	Removed bool
	Limit   int
	Offset  int
}

// ListHistory returns a page of userID's Read-throughs for all time and how
// many match, newest first: the ones underway, then by end day, then by
// begin day, undated days last, then the latest recorded.
func (svc *Service) ListHistory(ctx context.Context, userID int, opts HistoryOptions) ([]*models.ReadThrough, int, error) {
	readThroughs := []*models.ReadThrough{}
	q := svc.db.NewSelect().
		Model(&readThroughs).
		Where("rt.user_id = ?", userID)
	if opts.Outcome != nil {
		q = q.Where("rt.outcome = ?", *opts.Outcome)
	}
	if opts.Removed {
		q = q.Where("rt.book_id IS NULL")
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}
	err = q.
		OrderExpr("rt.outcome = ? DESC", models.ReadThroughOutcomeUnderway).
		OrderExpr("rt.end_day IS NULL, rt.end_day DESC, rt.begin_day IS NULL, rt.begin_day DESC, rt.created_at DESC, rt.id DESC").
		Limit(int64(opts.Limit)).
		Offset(int64(opts.Offset)).
		Scan(ctx)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return readThroughs, int(total), nil
}

// BuildHistoryEntries describes each of user's readThroughs as Reading
// History shows it. A Book in a library user can't reach shows only its
// title and authors.
func (svc *Service) BuildHistoryEntries(ctx context.Context, user *models.User, readThroughs []*models.ReadThrough) ([]*ReadingHistoryEntry, error) {
	entries := make([]*ReadingHistoryEntry, 0, len(readThroughs))
	var bookIDs []int
	for _, rt := range readThroughs {
		if rt.BookID != nil && !slices.Contains(bookIDs, *rt.BookID) {
			bookIDs = append(bookIDs, *rt.BookID)
		}
	}

	books := map[int]*models.Book{}
	underway := map[int]bool{}
	if len(bookIDs) > 0 {
		var err error
		books, err = svc.loadBooks(ctx, bookIDs)
		if err != nil {
			return nil, err
		}
		var underwayIDs []int
		err = svc.db.NewSelect().
			Model((*models.ReadThrough)(nil)).
			Column("rt.book_id").
			Where("rt.user_id = ? AND rt.outcome = ? AND rt.book_id IN (?)", user.ID, models.ReadThroughOutcomeUnderway, bun.List(bookIDs)).
			Scan(ctx, &underwayIDs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, id := range underwayIDs {
			underway[id] = true
		}
	}
	sessionCounts, err := svc.sessionCounts(ctx, readThroughs)
	if err != nil {
		return nil, err
	}
	authors, err := authorNames(ctx, svc.db, bookIDs)
	if err != nil {
		return nil, err
	}

	for _, rt := range readThroughs {
		entry := &ReadingHistoryEntry{
			ReadThroughEntry: ReadThroughEntry{ReadThrough: rt, Authors: []string{}},
			SessionCount:     sessionCounts[rt.ID],
		}
		book := (*models.Book)(nil)
		if rt.BookID != nil {
			book = books[*rt.BookID]
		}
		if book == nil {
			entry.Removed = true
			if rt.BookTitle != nil {
				entry.Title = *rt.BookTitle
			}
			if rt.BookAuthors != nil {
				entry.Authors = rt.BookAuthors
			}
			entries = append(entries, entry)
			continue
		}
		entry.Title = book.Title
		if names := authors[book.ID]; names != nil {
			entry.Authors = names
		}
		if entry.Book = bookLink(user, book); entry.Book != nil {
			entry.CanReread = !underway[book.ID]
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// loadBooks loads the Books with ids that still exist, with what bookLink
// needs, by id.
func (svc *Service) loadBooks(ctx context.Context, ids []int) (map[int]*models.Book, error) {
	books := map[int]*models.Book{}
	if len(ids) == 0 {
		return books, nil
	}
	var loaded []*models.Book
	err := svc.db.NewSelect().
		Model(&loaded).
		Relation("Library").
		Relation("Files").
		Where("b.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, b := range loaded {
		books[b.ID] = b
	}
	return books, nil
}

// bookLink is what a row needs to link to book and show its cover, or nil
// when user can't reach its library. book comes from loadBooks.
func bookLink(user *models.User, book *models.Book) *ReadingHistoryBook {
	if !user.HasLibraryAccess(book.LibraryID) {
		return nil
	}
	aspectRatio := ""
	if book.Library != nil {
		aspectRatio = book.Library.CoverAspectRatio
	}
	return &ReadingHistoryBook{
		ID:            book.ID,
		LibraryID:     book.LibraryID,
		CoverCacheKey: covers.CacheKey(book.Files, aspectRatio),
	}
}

// sessionCounts counts the Reading Sessions of each of readThroughs, by id.
func (svc *Service) sessionCounts(ctx context.Context, readThroughs []*models.ReadThrough) (map[int]int, error) {
	counts := map[int]int{}
	if len(readThroughs) == 0 {
		return counts, nil
	}
	ids := make([]int, 0, len(readThroughs))
	for _, rt := range readThroughs {
		ids = append(ids, rt.ID)
	}
	var rows []struct {
		ReadThroughID int `bun:"read_through_id"`
		Count         int `bun:"count"`
	}
	err := svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.read_through_id, COUNT(*) AS count").
		Where("rs.read_through_id IN (?)", bun.List(ids)).
		GroupExpr("rs.read_through_id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		counts[row.ReadThroughID] = row.Count
	}
	return counts, nil
}
