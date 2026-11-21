package books

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The Replace* methods make a Book's or File's relationship rows match the
// wanted list, updating rows in place: a row whose key is still wanted keeps
// its id and is written only when its other values differ, new keys are
// inserted, and the rest deleted. A Resync that reads the same values writes
// nothing, so the change log doesn't move the Book (ADR 0023). Each key is
// the table's unique index without its parent column, so updates never
// collide with it.
//
// They run without a transaction, as the delete and insert they replace
// did: a transaction that reads before it writes can't take the write lock
// once another connection has written, and the scan already holds each
// Book's lock while it writes the Book's relationships.

// relation describes how to sync one join table's rows.
type relation[T any] struct {
	// key identifies a row under its parent.
	key func(*T) string
	// same reports whether stored already holds wanted's values.
	same func(stored, wanted *T) bool
	// adopt copies wanted's values onto stored, keeping its id.
	adopt func(stored, wanted *T)
	// columns are what an update writes.
	columns []string
}

// syncRelation writes the difference between stored and wanted. A wanted
// row whose key repeats an earlier one is dropped, as the unique index
// would refuse it.
func syncRelation[T any](ctx context.Context, db bun.IDB, stored, wanted []*T, r relation[T]) error {
	byKey := make(map[string]*T, len(stored))
	for _, row := range stored {
		byKey[r.key(row)] = row
	}
	keep := map[string]bool{}
	var updates, inserts []*T
	for _, row := range wanted {
		key := r.key(row)
		if keep[key] {
			continue
		}
		keep[key] = true
		existing, ok := byKey[key]
		switch {
		case !ok:
			inserts = append(inserts, row)
		case !r.same(existing, row):
			r.adopt(existing, row)
			updates = append(updates, existing)
		}
	}
	var deletes []*T
	for _, row := range stored {
		if !keep[r.key(row)] {
			deletes = append(deletes, row)
		}
	}

	if len(deletes) > 0 {
		if _, err := db.NewDelete().Model(&deletes).WherePK().Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	for _, row := range updates {
		if _, err := db.NewUpdate().Model(row).Column(r.columns...).WherePK().Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	if len(inserts) > 0 {
		if _, err := db.NewInsert().Model(&inserts).Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ptrKey[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}

// ReplaceAuthors makes bookID's authors match authors, in place.
func (svc *Service) ReplaceAuthors(ctx context.Context, bookID int, authors []*models.Author) error {
	var stored []*models.Author
	if err := svc.db.NewSelect().Model(&stored).Where("book_id = ?", bookID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	return syncRelation(ctx, svc.db, stored, authors, relation[models.Author]{
		key:     func(a *models.Author) string { return fmt.Sprintf("%d/%s", a.PersonID, ptrKey(a.Role)) },
		same:    func(s, w *models.Author) bool { return s.SortOrder == w.SortOrder },
		adopt:   func(s, w *models.Author) { s.SortOrder = w.SortOrder },
		columns: []string{"sort_order"},
	})
}

// ReplaceNarrators makes fileID's narrators match narrators, in place.
func (svc *Service) ReplaceNarrators(ctx context.Context, fileID int, narrators []*models.Narrator) error {
	var stored []*models.Narrator
	if err := svc.db.NewSelect().Model(&stored).Where("file_id = ?", fileID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	return syncRelation(ctx, svc.db, stored, narrators, relation[models.Narrator]{
		key:     func(n *models.Narrator) string { return strconv.Itoa(n.PersonID) },
		same:    func(s, w *models.Narrator) bool { return s.SortOrder == w.SortOrder },
		adopt:   func(s, w *models.Narrator) { s.SortOrder = w.SortOrder },
		columns: []string{"sort_order"},
	})
}

// ReplaceBookSeries makes bookID's Series memberships match bookSeries, in
// place.
func (svc *Service) ReplaceBookSeries(ctx context.Context, bookID int, bookSeries []*models.BookSeries) error {
	var stored []*models.BookSeries
	if err := svc.db.NewSelect().Model(&stored).Where("book_id = ?", bookID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	return syncRelation(ctx, svc.db, stored, bookSeries, relation[models.BookSeries]{
		key: func(bs *models.BookSeries) string { return strconv.Itoa(bs.SeriesID) },
		same: func(s, w *models.BookSeries) bool {
			return s.SortOrder == w.SortOrder && samePtr(s.SeriesNumber, w.SeriesNumber) &&
				samePtr(s.SeriesNumberEnd, w.SeriesNumberEnd) && samePtr(s.SeriesNumberUnit, w.SeriesNumberUnit)
		},
		adopt: func(s, w *models.BookSeries) {
			s.SortOrder = w.SortOrder
			s.SeriesNumber = w.SeriesNumber
			s.SeriesNumberEnd = w.SeriesNumberEnd
			s.SeriesNumberUnit = w.SeriesNumberUnit
		},
		columns: []string{"sort_order", "series_number", "series_number_end", "series_number_unit"},
	})
}

// ReplaceBookGenres makes bookID's genres match bookGenres, in place.
func (svc *Service) ReplaceBookGenres(ctx context.Context, bookID int, bookGenres []*models.BookGenre) error {
	var stored []*models.BookGenre
	if err := svc.db.NewSelect().Model(&stored).Where("book_id = ?", bookID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	return syncRelation(ctx, svc.db, stored, bookGenres, relation[models.BookGenre]{
		key:  func(bg *models.BookGenre) string { return strconv.Itoa(bg.GenreID) },
		same: func(_, _ *models.BookGenre) bool { return true },
	})
}

// ReplaceBookTags makes bookID's tags match bookTags, in place.
func (svc *Service) ReplaceBookTags(ctx context.Context, bookID int, bookTags []*models.BookTag) error {
	var stored []*models.BookTag
	if err := svc.db.NewSelect().Model(&stored).Where("book_id = ?", bookID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	return syncRelation(ctx, svc.db, stored, bookTags, relation[models.BookTag]{
		key:  func(bt *models.BookTag) string { return strconv.Itoa(bt.TagID) },
		same: func(_, _ *models.BookTag) bool { return true },
	})
}
