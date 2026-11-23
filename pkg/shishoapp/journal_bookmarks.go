package shishoapp

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// journalBookmark links the id an installation made a Bookmark under to the
// Bookmark; BookmarkID is nil once the Bookmark was deleted, so a create
// sent again doesn't bring it back.
type journalBookmark struct {
	bun.BaseModel `bun:"table:app_journal_bookmarks,alias:ajb"`

	UserID     int       `bun:"user_id,pk"`
	LocalID    string    `bun:"local_id,pk"`
	BookmarkID *int      `bun:"bookmark_id"`
	CreatedAt  time.Time `bun:"created_at,notnull"`
}

const (
	refusedNotAudiobook = "Only audiobooks have Bookmarks."
	refusedBookmarkRef  = "A Bookmark is named by exactly one of bookmark_id or local_id."
)

// applyBookmarkCreate makes a Bookmark at the time the event happened, once
// per LocalID: one made before, even one deleted since, is left as it is.
func (h *handler) applyBookmarkCreate(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalBookmarkCreatePayload, claim claimFunc) (eventOutcome, error) {
	target, err := h.bookmarks.RetrieveFileTarget(ctx, p.FileID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !user.HasLibraryAccess(target.LibraryID) {
		return eventOutcome{refused: refusedFile}, nil
	}
	if err != nil {
		return eventOutcome{}, err
	}
	if !models.IsAudiobookFileType(target.FileType) {
		return eventOutcome{refused: refusedNotAudiobook}, nil
	}
	made, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// The claim writes first, so the transaction holds the write lock
		// before it reads.
		if err := claim(ctx, tx); err != nil {
			return err
		}
		seen, err := tx.NewSelect().Model((*journalBookmark)(nil)).
			Where("user_id = ? AND local_id = ?", user.ID, p.LocalID).
			Exists(ctx)
		if err != nil || seen {
			return errors.WithStack(err)
		}
		bookmark, err := h.bookmarks.CreateIn(ctx, tx, user.ID, p.FileID, *p.Seconds, p.Note, made)
		if err != nil {
			return err
		}
		_, err = tx.NewInsert().Model(&journalBookmark{
			UserID: user.ID, LocalID: p.LocalID, BookmarkID: &bookmark.ID, CreatedAt: time.Now(),
		}).Exec(ctx)
		return errors.WithStack(err)
	})
	return eventOutcome{}, err
}

// applyBookmarkNote replaces a Bookmark's note, unless it changed later
// than the event: the latest instant wins (ADR 0024). Events of one batch
// apply in order, so a note given the same time as the change before it,
// as times from a fast clock are, still applies.
func (h *handler) applyBookmarkNote(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalBookmarkNotePayload, claim claimFunc) (eventOutcome, error) {
	return h.changeBookmark(ctx, user, clock, at, p.JournalBookmarkRef, claim,
		func(ctx context.Context, tx bun.Tx, bookmark *models.Bookmark, changed time.Time) error {
			if bookmark == nil {
				return nil
			}
			if changed.Before(bookmark.UpdatedAt) {
				// The app showed its own note while the edit waited, so the
				// note that won goes to it again.
				return h.feed.MoveOwn(ctx, tx, changefeed.KindBookmark, bookmark.ID, user.ID)
			}
			return h.bookmarks.UpdateNoteIn(ctx, tx, bookmark, p.Note, changed)
		})
}

// applyBookmarkDelete deletes a Bookmark; one already gone stays gone. A
// local id the server hasn't seen is recorded as deleted, since its create
// can come after, from a batch the app gave up on and sent again.
func (h *handler) applyBookmarkDelete(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	ref JournalBookmarkRef, claim claimFunc) (eventOutcome, error) {
	return h.changeBookmark(ctx, user, clock, at, ref, claim,
		func(ctx context.Context, tx bun.Tx, bookmark *models.Bookmark, _ time.Time) error {
			if bookmark != nil {
				return h.bookmarks.DeleteIn(ctx, tx, bookmark)
			}
			if ref.LocalID == nil {
				return nil
			}
			_, err := tx.NewInsert().Model(&journalBookmark{UserID: user.ID, LocalID: *ref.LocalID, CreatedAt: time.Now()}).
				On("CONFLICT DO NOTHING").
				Exec(ctx)
			return errors.WithStack(err)
		})
}

// changeBookmark runs change on the user's Bookmark ref names, in the
// transaction that claims the event, or on nil when it's gone or never was
// the user's: that's taken in, so a delete wins and another user's
// Bookmarks stay hidden.
func (h *handler) changeBookmark(ctx context.Context, user *models.User, clock deviceClock, at time.Time, ref JournalBookmarkRef,
	claim claimFunc, change func(ctx context.Context, tx bun.Tx, bookmark *models.Bookmark, changed time.Time) error) (eventOutcome, error) {
	if (ref.BookmarkID == nil) == (ref.LocalID == nil) {
		return eventOutcome{refused: refusedBookmarkRef}, nil
	}
	changed, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	// Reaching the File is checked before the claim, so a refusal leaves the
	// event unclaimed.
	if current, err := h.findBookmark(ctx, h.db, user.ID, ref); err != nil {
		return eventOutcome{}, err
	} else if current != nil {
		if ok, err := h.readableFile(ctx, user, current.FileID); err != nil || !ok {
			return eventOutcome{refused: refusedFile}, err
		}
	}
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := claim(ctx, tx); err != nil {
			return err
		}
		bookmark, err := h.findBookmark(ctx, tx, user.ID, ref)
		if err != nil {
			return err
		}
		return change(ctx, tx, bookmark, changed)
	})
	return eventOutcome{}, err
}

// findBookmark returns the user's Bookmark ref names, or nil when it's gone
// or was never theirs.
func (h *handler) findBookmark(ctx context.Context, db bun.IDB, userID int, ref JournalBookmarkRef) (*models.Bookmark, error) {
	id := ref.BookmarkID
	if ref.LocalID != nil {
		link := &journalBookmark{}
		err := db.NewSelect().Model(link).Where("user_id = ? AND local_id = ?", userID, *ref.LocalID).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		id = link.BookmarkID
	}
	if id == nil {
		return nil, nil
	}
	bookmark, err := h.bookmarks.RetrieveIn(ctx, db, userID, *id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return bookmark, err
}
