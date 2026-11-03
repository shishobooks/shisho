package review

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// BookReviewRecomputer refreshes files.reviewed for every file of each book,
// loading the review criteria once. *books.Service satisfies it. pkg/books
// imports the genres, tags, people, and publishers packages, so their handlers
// take this interface instead of *books.Service to avoid an import cycle.
type BookReviewRecomputer interface {
	RecomputeReviewedForBooks(ctx context.Context, bookIDs []int)
}

// withFileRelations and withBookRelations load what completeness reads. A
// relation left out reads as an empty field, which marks files not reviewed.
func withFileRelations(q *bun.SelectQuery) *bun.SelectQuery {
	return q.Relation("Narrators").Relation("Identifiers").Relation("Chapters")
}

func withBookRelations(q *bun.SelectQuery) *bun.SelectQuery {
	return q.Relation("Authors").Relation("BookSeries").Relation("BookGenres").Relation("BookTags")
}

// RecomputeForFile reloads the file (with relations needed by completeness),
// computes its `reviewed` value, and persists. Override-set rows short-circuit.
// Supplements get reviewed=NULL.
func RecomputeForFile(ctx context.Context, db bun.IDB, fileID int, criteria Criteria) error {
	file := &models.File{}
	err := db.NewSelect().
		Model(file).
		Where("f.id = ?", fileID).
		Apply(withFileRelations).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if file.FileRole == models.FileRoleSupplement {
		_, err := db.NewUpdate().
			Model((*models.File)(nil)).
			Set("reviewed = NULL").
			Where("id = ?", fileID).
			Exec(ctx)
		return errors.WithStack(err)
	}

	book := &models.Book{}
	err = db.NewSelect().
		Model(book).
		Where("b.id = ?", file.BookID).
		Apply(withBookRelations).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	reviewed := computeReviewedValue(book, file, criteria)
	_, err = db.NewUpdate().
		Model((*models.File)(nil)).
		Set("reviewed = ?", reviewed).
		Where("id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// RecomputeBatchSize bounds how long a batch holds the database's only
// connection, so other requests get a turn between batches during a large
// recompute.
const RecomputeBatchSize = 500

// RecomputeForBooks recomputes reviewed for every file of each book and writes
// only the values that changed. It works in batches, each in its own
// transaction so a concurrent edit can't land between a batch's read and write.
// Don't wrap a large call in a transaction: that holds the connection for the
// whole run. When a batch fails, its books are retried one at a time, so a bad
// row costs only its own book; the returned error counts them and names the
// first.
func RecomputeForBooks(ctx context.Context, db bun.IDB, bookIDs []int, criteria Criteria) error {
	var firstErr error
	var firstBookID, failed int
	for batch := range slices.Chunk(bookIDs, RecomputeBatchSize) {
		err := recomputeInTx(ctx, db, batch, criteria)
		if err == nil {
			continue
		}
		// A canceled context fails every book the same way.
		if ctx.Err() != nil {
			return err
		}
		for _, bookID := range batch {
			if err := recomputeInTx(ctx, db, []int{bookID}, criteria); err != nil {
				failed++
				if firstErr == nil {
					firstErr, firstBookID = err, bookID
				}
			}
		}
	}
	if firstErr != nil {
		return errors.Wrapf(firstErr, "%d of %d books failed, first book %d", failed, len(bookIDs), firstBookID)
	}
	return nil
}

func recomputeInTx(ctx context.Context, db bun.IDB, bookIDs []int, criteria Criteria) error {
	return db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		return recomputeBatch(ctx, tx, bookIDs, criteria)
	})
}

func recomputeBatch(ctx context.Context, tx bun.Tx, bookIDs []int, criteria Criteria) error {
	var files []*models.File
	err := tx.NewSelect().
		Model(&files).
		Where("f.book_id IN (?)", bun.List(bookIDs)).
		Apply(withFileRelations).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(files) == 0 {
		return nil
	}

	var books []*models.Book
	err = tx.NewSelect().
		Model(&books).
		Where("b.id IN (?)", bun.List(bookIDs)).
		Apply(withBookRelations).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	booksByID := make(map[int]*models.Book, len(books))
	for _, book := range books {
		booksByID[book.ID] = book
	}

	var setTrue, setFalse, setNull []int
	for _, file := range files {
		var reviewed *bool
		if file.FileRole != models.FileRoleSupplement {
			book, ok := booksByID[file.BookID]
			if !ok {
				return errors.Errorf("file %d points at missing book %d", file.ID, file.BookID)
			}
			reviewed = computeReviewedValue(book, file, criteria)
		}
		switch {
		case reviewed == nil && file.Reviewed == nil:
		case reviewed != nil && file.Reviewed != nil && *reviewed == *file.Reviewed:
		case reviewed == nil:
			setNull = append(setNull, file.ID)
		case *reviewed:
			setTrue = append(setTrue, file.ID)
		default:
			setFalse = append(setFalse, file.ID)
		}
	}

	for _, write := range []struct {
		value   *bool
		fileIDs []int
	}{{pointerutil.Bool(true), setTrue}, {pointerutil.Bool(false), setFalse}, {nil, setNull}} {
		if len(write.fileIDs) == 0 {
			continue
		}
		_, err := tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("reviewed = ?", write.value).
			Where("id IN (?)", bun.List(write.fileIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// SetOverride writes an explicit override and the timestamp, then writes the
// effective `reviewed` value. Pass override=nil to clear (then completeness
// drives reviewed).
func SetOverride(ctx context.Context, db bun.IDB, fileID int, override *string, criteria Criteria) error {
	if override != nil && *override != models.ReviewOverrideReviewed && *override != models.ReviewOverrideUnreviewed {
		return errors.Errorf("invalid override value: %q", *override)
	}
	now := time.Now().UTC()
	q := db.NewUpdate().Model((*models.File)(nil)).Where("id = ?", fileID)
	if override == nil {
		q = q.Set("review_override = NULL").Set("review_overridden_at = NULL")
	} else {
		q = q.Set("review_override = ?", *override).Set("review_overridden_at = ?", now)
	}
	if _, err := q.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}
	return RecomputeForFile(ctx, db, fileID, criteria)
}

// computeReviewedValue returns the effective reviewed value for a (book, main-file)
// pair given the override and completeness. Returns nil for supplements (caller
// is responsible for that branch).
func computeReviewedValue(book *models.Book, file *models.File, criteria Criteria) *bool {
	if file.ReviewOverride != nil {
		switch *file.ReviewOverride {
		case models.ReviewOverrideReviewed:
			t := true
			return &t
		case models.ReviewOverrideUnreviewed:
			f := false
			return &f
		}
	}
	v := IsComplete(book, file, criteria)
	return &v
}
