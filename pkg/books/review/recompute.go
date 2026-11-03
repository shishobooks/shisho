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

// RecomputeForFile reloads the file (with relations needed by completeness),
// computes its `reviewed` value, and persists. Override-set rows short-circuit.
// Supplements get reviewed=NULL.
func RecomputeForFile(ctx context.Context, db bun.IDB, fileID int, criteria Criteria) error {
	file := &models.File{}
	err := db.NewSelect().
		Model(file).
		Where("f.id = ?", fileID).
		Relation("Narrators").
		Relation("Identifiers").
		Relation("Chapters").
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
		Relation("Authors").
		Relation("BookSeries").
		Relation("BookGenres").
		Relation("BookTags").
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

// RecomputeForBook recomputes reviewed for every file belonging to the book.
func RecomputeForBook(ctx context.Context, db bun.IDB, bookID int, criteria Criteria) error {
	return RecomputeForBooks(ctx, db, []int{bookID}, criteria)
}

// RecomputeBatchSize bounds how long a batch holds the database's only
// connection, so other requests get a turn between batches during a large
// recompute.
const RecomputeBatchSize = 500

// RecomputeForBooks recomputes reviewed for every file of each book and writes
// only the values that changed. It works in batches, each in its own
// transaction so a concurrent edit can't land between a batch's read and write.
// Don't wrap a large call in a transaction: that holds the connection for the
// whole run. A failed batch doesn't stop the rest; the first error is returned.
func RecomputeForBooks(ctx context.Context, db bun.IDB, bookIDs []int, criteria Criteria) error {
	var firstErr error
	for batch := range slices.Chunk(bookIDs, RecomputeBatchSize) {
		err := db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
			return recomputeBatch(ctx, tx, batch, criteria)
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func recomputeBatch(ctx context.Context, tx bun.Tx, bookIDs []int, criteria Criteria) error {
	var files []*models.File
	err := tx.NewSelect().
		Model(&files).
		Where("f.book_id IN (?)", bun.List(bookIDs)).
		Relation("Narrators").
		Relation("Identifiers").
		Relation("Chapters").
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
		Relation("Authors").
		Relation("BookSeries").
		Relation("BookGenres").
		Relation("BookTags").
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
			reviewed = computeReviewedValue(booksByID[file.BookID], file, criteria)
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
