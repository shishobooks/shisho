package review

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

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
	var fileIDs []int
	err := db.NewSelect().
		Model((*models.File)(nil)).
		Column("id").
		Where("book_id = ?", bookID).
		Scan(ctx, &fileIDs)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, id := range fileIDs {
		if err := RecomputeForFile(ctx, db, id, criteria); err != nil {
			return err
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
