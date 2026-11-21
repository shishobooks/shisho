package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

var (
	// ErrAlreadyReference is returned for marking Reference a Book that
	// already is, and ErrNotReference for removing it from one that isn't,
	// which the handlers render as 422 invalid_state.
	ErrAlreadyReference = errors.New("the book is already reference")
	ErrNotReference     = errors.New("the book is not reference")
	// ErrUndoOnReference is returned for undoing an automatic finish of a
	// Book marked Reference since: Reference is the later choice.
	ErrUndoOnReference = errors.New("the book is reference")
)

// MarkReference marks bookID Reference for userID, for a Book they dip into
// rather than read through (ADR 0028). From then on only the user's explicit
// choices change its status. A Read-through underway or paused becomes plain
// Reading Sessions: its sessions and position update history are kept,
// detached as a lookup's are, and it's deleted, leaving the Reading Positions
// where they are. The Book leaves To Read. Earlier ended Read-throughs stay
// as history, and no automatic finish of one can be undone any more, even
// after Remove from Reference.
func (svc *Service) MarkReference(ctx context.Context, userID, bookID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		reference, err := isReference(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		if reference {
			return ErrAlreadyReference
		}
		return markReference(ctx, tx, userID, bookID, svc.now())
	})
}

// markReference is MarkReference inside the caller's transaction, for a Book
// that isn't Reference yet.
func markReference(ctx context.Context, tx bun.Tx, userID, bookID int, now time.Time) error {
	underway, err := retrieveUnderway(ctx, tx, userID, bookID)
	if err != nil {
		return err
	}
	if underway != nil {
		if err := dissolveReadThrough(ctx, tx, underway.ID); err != nil {
			return err
		}
	}
	if err := leaveToRead(ctx, tx, userID, bookID, now); err != nil {
		return err
	}
	// Reference is the later choice, so an automatic finish before it stands,
	// as KeepFinished makes one stand.
	_, err = tx.NewUpdate().
		Model((*models.ReadingPositionUpdate)(nil)).
		Set("auto_finished = FALSE").
		Where("auto_finished").
		Where("read_through_id IN (SELECT id FROM read_throughs WHERE user_id = ? AND book_id = ?)", userID, bookID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return chooseStatus(ctx, tx, userID, bookID, true, now)
}

// dissolveReadThrough deletes Read-through rtID but keeps what was recorded
// during it, its Reading Sessions and position update history, on no
// Read-through, so its time still counts. Unlike deleting a Read-through, it
// leaves the Reading Positions and To Read alone.
func dissolveReadThrough(ctx context.Context, tx bun.Tx, rtID int) error {
	for _, model := range []any{(*models.ReadingSession)(nil), (*models.ReadingPositionUpdate)(nil)} {
		_, err := tx.NewUpdate().
			Model(model).
			Set("read_through_id = NULL").
			Where("read_through_id = ?", rtID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	_, err := tx.NewDelete().Model((*models.ReadThrough)(nil)).Where("id = ?", rtID).Exec(ctx)
	return errors.WithStack(err)
}

// RemoveReference clears userID's Reference mark on bookID and records
// nothing else, so the status falls back to what the Read-throughs say.
func (svc *Service) RemoveReference(ctx context.Context, userID, bookID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		reference, err := isReference(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		if !reference {
			return ErrNotReference
		}
		return chooseStatus(ctx, tx, userID, bookID, false, svc.now())
	})
}

// BulkReference marks each of bookIDs Reference for userID in one
// transaction, as MarkReference does for one, skipping Books already
// Reference. The caller checks the user can reach each Book.
func (svc *Service) BulkReference(ctx context.Context, userID int, bookIDs []int) (BulkReadingResponse, error) {
	return svc.bulkReference(ctx, userID, bookIDs, true)
}

// BulkRemoveReference clears the Reference mark on each of bookIDs for
// userID in one transaction, skipping Books that aren't Reference. The
// caller checks the user can reach each Book.
func (svc *Service) BulkRemoveReference(ctx context.Context, userID int, bookIDs []int) (BulkReadingResponse, error) {
	return svc.bulkReference(ctx, userID, bookIDs, false)
}

func (svc *Service) bulkReference(ctx context.Context, userID int, bookIDs []int, mark bool) (BulkReadingResponse, error) {
	var result BulkReadingResponse
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		result = BulkReadingResponse{}
		now := svc.now()
		for _, bookID := range distinct(bookIDs) {
			reference, err := isReference(ctx, tx, userID, bookID)
			if err != nil {
				return err
			}
			if reference == mark {
				result.Skipped++
				continue
			}
			if mark {
				err = markReference(ctx, tx, userID, bookID, now)
			} else {
				err = chooseStatus(ctx, tx, userID, bookID, false, now)
			}
			if err != nil {
				return err
			}
			result.Changed++
		}
		return nil
	})
	return result, err
}

// chooseStatus records an explicit status choice userID made for bookID at
// now: whether the Book is Reference after it, and the instant, which every
// explicit choice stamps whether or not it concerns Reference.
func chooseStatus(ctx context.Context, db bun.IDB, userID, bookID int, reference bool, now time.Time) error {
	_, err := db.NewInsert().
		Model(&models.UserBook{
			UserID:         userID,
			BookID:         bookID,
			CreatedAt:      now,
			UpdatedAt:      now,
			Reference:      reference,
			StatusChosenAt: &now,
		}).
		On("CONFLICT (user_id, book_id) DO UPDATE").
		Set("reference = EXCLUDED.reference").
		Set("status_chosen_at = EXCLUDED.status_chosen_at").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return errors.WithStack(err)
}

// isReference reports whether userID marked bookID Reference.
func isReference(ctx context.Context, db bun.IDB, userID, bookID int) (bool, error) {
	exists, err := db.NewSelect().
		Model((*models.UserBook)(nil)).
		Where("ub.user_id = ? AND ub.book_id = ? AND ub.reference", userID, bookID).
		Exists(ctx)
	return exists, errors.WithStack(err)
}
