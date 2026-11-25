package shishoapp

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

// The journal's reading events: what the web's status menu, Rating, To
// Read, and Lists do, made in the app at the event's time. The reading and
// lists packages order each against what changed since (ADR 0024, 0028).

const (
	refusedListRef       = "A List is named by exactly one of list_id or local_id."
	refusedList          = "The List doesn't exist, or you can no longer reach it."
	refusedListEdit      = "You can't change the Books in this List."
	refusedListManage    = "You can't rename this List."
	refusedListOwner     = "Only the List's owner can delete it."
	refusedListNameTaken = "You already have a List with this name."
	refusedReadThroughOf = "A Read-through is named by exactly one of read_through_id or book_id."
	refusedNothingToEnd  = "Nothing was underway to mark Did Not Finish."
)

// journalList links the id an installation made a List under to the List;
// ListID is nil once the List was deleted, so a create sent again doesn't
// bring it back.
type journalList struct {
	bun.BaseModel `bun:"table:app_journal_lists,alias:ajl"`

	UserID    int       `bun:"user_id,pk"`
	LocalID   string    `bun:"local_id,pk"`
	ListID    *int      `bun:"list_id"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

// readingEvent decodes a reading event of type kind and returns how to
// apply it, or false when kind isn't one.
func (h *handler) readingEvent(ctx context.Context, c echo.Context, user *models.User, clock deviceClock, ev JournalEvent) (
	apply func(claim claimFunc) (eventOutcome, error), problem *payloadProblem, ok bool) {
	book := func(change func(e reading.LateEvent) error) (func(claim claimFunc) (eventOutcome, error), *payloadProblem) {
		p, problem := decodePayload[JournalBookPayload](c, ev)
		if problem != nil {
			return nil, problem
		}
		return func(claim claimFunc) (eventOutcome, error) {
			return h.lateBookEvent(ctx, user, clock, ev.At, p.BookID, p.Day, claim, change)
		}, nil
	}
	svc := h.reading
	switch ev.Type {
	case JournalEventReadThroughStart:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateStart(ctx, e) })
	case JournalEventReadThroughFinish:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateEnd(ctx, e, models.ReadThroughOutcomeFinished) })
	case JournalEventReadThroughAbandon:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateEnd(ctx, e, models.ReadThroughOutcomeAbandoned) })
	case JournalEventReadThroughPause:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateSetPaused(ctx, e, true) })
	case JournalEventReadThroughResume:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateSetPaused(ctx, e, false) })
	case JournalEventReferenceMark:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateReference(ctx, e, true) })
	case JournalEventReferenceRemove:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateReference(ctx, e, false) })
	case JournalEventRatingClear:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateSetRating(ctx, e, nil) })
	case JournalEventToReadAdd:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateAddToRead(ctx, e) })
	case JournalEventToReadRemove:
		apply, problem = book(func(e reading.LateEvent) error { return svc.LateRemoveFromToRead(ctx, e) })
	case JournalEventRatingSet:
		var p JournalRatingPayload
		if p, problem = decodePayload[JournalRatingPayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.lateBookEvent(ctx, user, clock, ev.At, p.BookID, "", claim, func(e reading.LateEvent) error {
					units := p.Rating
					return svc.LateSetRating(ctx, e, &units)
				})
			}
		}
	case JournalEventToReadMove:
		var p JournalToReadMovePayload
		if p, problem = decodePayload[JournalToReadMovePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.lateBookEvent(ctx, user, clock, ev.At, p.BookID, "", claim, func(e reading.LateEvent) error {
					return svc.LateMoveToRead(ctx, e, p.AfterBookID)
				})
			}
		}
	case JournalEventReadThroughDelete:
		var p JournalReadThroughDeletePayload
		if p, problem = decodePayload[JournalReadThroughDeletePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.applyReadThroughDelete(ctx, user, clock, ev.At, p, claim)
			}
		}
	case JournalEventPositionClear:
		var p JournalFilePayload
		if p, problem = decodePayload[JournalFilePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.applyPositionClear(ctx, user, clock, ev.At, p, claim)
			}
		}
	case JournalEventListCreate:
		var p JournalListCreatePayload
		if p, problem = decodePayload[JournalListCreatePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.applyListCreate(ctx, user, clock, ev.At, p, claim)
			}
		}
	case JournalEventListRename:
		var p JournalListRenamePayload
		if p, problem = decodePayload[JournalListRenamePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.changeList(ctx, user, clock, ev.At, p.JournalListRef, claim, h.lists.CanManage, refusedListManage,
					func(ctx context.Context, tx bun.Tx, listID int, _ time.Time) error {
						err := lists.RenameListIn(ctx, tx, listID, strings.TrimSpace(p.Name), time.Now())
						if errors.Is(err, lists.ErrListNameTaken) {
							return errRefused(refusedListNameTaken)
						}
						return err
					})
			}
		}
	case JournalEventListDelete:
		var p JournalListRef
		if p, problem = decodePayload[JournalListRef](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.changeList(ctx, user, clock, ev.At, p, claim, h.lists.IsOwner, refusedListOwner,
					func(ctx context.Context, tx bun.Tx, listID int, _ time.Time) error {
						_, err := tx.NewDelete().Model((*models.List)(nil)).Where("id = ?", listID).Exec(ctx)
						return errors.WithStack(err)
					})
			}
		}
	case JournalEventListAdd, JournalEventListRemove:
		var p JournalListBookPayload
		if p, problem = decodePayload[JournalListBookPayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				if ok, err := h.reachableBook(ctx, user, p.BookID); err != nil || !ok {
					return eventOutcome{refused: refusedBook}, err
				}
				return h.changeList(ctx, user, clock, ev.At, p.JournalListRef, claim, h.lists.CanEdit, refusedListEdit,
					func(ctx context.Context, tx bun.Tx, listID int, at time.Time) error {
						if ev.Type == JournalEventListAdd {
							return lists.AddBookIn(ctx, tx, listID, p.BookID, user.ID, at, time.Now())
						}
						return lists.RemoveBookIn(ctx, tx, listID, p.BookID, at, time.Now())
					})
			}
		}
	case JournalEventListMove:
		var p JournalListMovePayload
		if p, problem = decodePayload[JournalListMovePayload](c, ev); problem == nil {
			apply = func(claim claimFunc) (eventOutcome, error) {
				return h.changeList(ctx, user, clock, ev.At, p.JournalListRef, claim, h.lists.CanEdit, refusedListEdit,
					func(ctx context.Context, tx bun.Tx, listID int, at time.Time) error {
						return lists.MoveBookIn(ctx, tx, listID, p.BookID, p.AfterBookID, at, time.Now())
					})
			}
		}
	default:
		return nil, nil, false
	}
	return apply, problem, true
}

// refusal is a change refused for the user to read, returned from inside
// a transaction so it rolls back the claim.
type refusal struct{ reason string }

func (r refusal) Error() string { return r.reason }

func errRefused(reason string) error { return refusal{reason: reason} }

// outcomeOf turns a refusal into the event's outcome.
func outcomeOf(err error) (eventOutcome, error) {
	var r refusal
	if errors.As(err, &r) {
		return eventOutcome{refused: r.reason}, nil
	}
	return eventOutcome{}, err
}

// reachableBook reports whether user can reach bookID.
func (h *handler) reachableBook(ctx context.Context, user *models.User, bookID int) (bool, error) {
	libraryID, err := h.reading.BookLibraryID(ctx, bookID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, errors.WithStack(err)
	}
	return user.HasLibraryAccess(libraryID), nil
}

// lateBookEvent applies a change to one of user's Books made at at, once
// it's checked the user can reach the Book.
func (h *handler) lateBookEvent(ctx context.Context, user *models.User, clock deviceClock, at time.Time, bookID int, day string,
	claim claimFunc, change func(e reading.LateEvent) error) (eventOutcome, error) {
	if ok, err := h.reachableBook(ctx, user, bookID); err != nil || !ok {
		return eventOutcome{refused: refusedBook}, err
	}
	occurred, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	err := change(reading.LateEvent{UserID: user.ID, BookID: bookID, At: occurred, DeviceDay: day, Claim: claim})
	if errors.Is(err, reading.ErrLateNothingUnderway) {
		return eventOutcome{refused: refusedNothingToEnd}, nil
	}
	return eventOutcome{}, err
}

// applyReadThroughDelete deletes one of the user's Read-throughs. One that's
// gone, or was never theirs, is taken in and changes nothing: a delete
// wins.
func (h *handler) applyReadThroughDelete(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalReadThroughDeletePayload, claim claimFunc) (eventOutcome, error) {
	if (p.ReadThroughID == nil) == (p.BookID == nil) {
		return eventOutcome{refused: refusedReadThroughOf}, nil
	}
	occurred, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	e := reading.LateEvent{UserID: user.ID, At: occurred, DeviceDay: p.Day, Claim: claim}
	if p.BookID != nil {
		if ok, err := h.reachableBook(ctx, user, *p.BookID); err != nil || !ok {
			return eventOutcome{refused: refusedBook}, err
		}
		e.BookID = *p.BookID
	}
	return eventOutcome{}, h.reading.LateDeleteReadThrough(ctx, e, p.ReadThroughID)
}

// applyPositionClear clears the user's place in a File, unless it was saved
// after the event.
func (h *handler) applyPositionClear(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalFilePayload, claim claimFunc) (eventOutcome, error) {
	if ok, err := h.readableFile(ctx, user, p.FileID); err != nil || !ok {
		return eventOutcome{refused: refusedFile}, err
	}
	occurred, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	err := h.reading.LateClearPosition(ctx, user.ID, p.FileID, occurred, claim)
	if errors.Is(err, reading.ErrFileNotFound) {
		return eventOutcome{refused: refusedFile}, nil
	}
	return eventOutcome{}, err
}

// applyListCreate makes a List the user owns, once per LocalID.
func (h *handler) applyListCreate(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalListCreatePayload, claim claimFunc) (eventOutcome, error) {
	made, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return eventOutcome{refused: "A List needs a name."}, nil
	}
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// The claim writes first, so the transaction holds the write lock
		// before it reads.
		if err := claim(ctx, tx); err != nil {
			return err
		}
		seen, err := tx.NewSelect().Model((*journalList)(nil)).
			Where("user_id = ? AND local_id = ?", user.ID, p.LocalID).
			Exists(ctx)
		if err != nil || seen {
			return errors.WithStack(err)
		}
		list, err := lists.CreateListIn(ctx, tx, lists.CreateListOptions{
			UserID: user.ID, Name: name, Description: p.Description, IsOrdered: p.IsOrdered,
		}, made)
		if errors.Is(err, lists.ErrListNameTaken) {
			return errRefused(refusedListNameTaken)
		}
		if err != nil {
			return err
		}
		_, err = tx.NewInsert().Model(&journalList{UserID: user.ID, LocalID: p.LocalID, ListID: &list.ID, CreatedAt: time.Now()}).Exec(ctx)
		return errors.WithStack(err)
	})
	return outcomeOf(err)
}

// changeList runs change on the List ref names, in the transaction that
// claims the event, once allowed says the user may. A List that's gone, or
// was deleted before its create arrived, is taken in and changes nothing,
// as a delete wins; one the user can't reach is refused.
func (h *handler) changeList(ctx context.Context, user *models.User, clock deviceClock, at time.Time, ref JournalListRef,
	claim claimFunc, allowed func(ctx context.Context, listID, userID int) (bool, error), notAllowed string,
	change func(ctx context.Context, tx bun.Tx, listID int, at time.Time) error) (eventOutcome, error) {
	if (ref.ListID == nil) == (ref.LocalID == nil) {
		return eventOutcome{refused: refusedListRef}, nil
	}
	changed, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	listID, known, err := h.findList(ctx, user.ID, ref)
	if err != nil {
		return eventOutcome{}, err
	}
	if !known {
		// The app sends a List's create before anything else about it, so a
		// local id the server never took in names a List whose create was
		// refused.
		return eventOutcome{refused: refusedList}, nil
	}
	if listID == nil {
		// Deleted: taken in, and changes nothing.
		return eventOutcome{}, errors.WithStack(h.db.RunInTx(ctx, nil, claim))
	}
	exists, err := h.db.NewSelect().Model((*models.List)(nil)).Where("id = ?", *listID).Exists(ctx)
	if err != nil {
		return eventOutcome{}, errors.WithStack(err)
	}
	if !exists {
		return eventOutcome{}, errors.WithStack(h.db.RunInTx(ctx, nil, claim))
	}
	canView, err := h.lists.CanView(ctx, *listID, user.ID)
	if err != nil {
		return eventOutcome{}, errors.WithStack(err)
	}
	if !canView {
		return eventOutcome{refused: refusedList}, nil
	}
	may, err := allowed(ctx, *listID, user.ID)
	if err != nil {
		return eventOutcome{}, errors.WithStack(err)
	}
	if !may {
		return eventOutcome{refused: notAllowed}, nil
	}
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := claim(ctx, tx); err != nil {
			return err
		}
		return change(ctx, tx, *listID, changed)
	})
	return outcomeOf(err)
}

// findList returns the List ref names: its id, nil when it was deleted, and
// whether the server knows the reference at all.
func (h *handler) findList(ctx context.Context, userID int, ref JournalListRef) (*int, bool, error) {
	if ref.ListID != nil {
		return ref.ListID, true, nil
	}
	link := &journalList{}
	err := h.db.NewSelect().Model(link).Where("user_id = ? AND local_id = ?", userID, *ref.LocalID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	return link.ListID, true, nil
}

// statsFor answers the user's Reading Stats for the app.
func statsFor(resp *reading.ReadingStatsResponse, timeZone string) StatsResponse {
	out := StatsResponse{
		Year: resp.Year, Years: resp.Years, TimeZone: timeZone,
		Totals: StatsTotals{
			Finished: resp.Totals.Finished, Rereads: resp.Totals.Rereads, DidNotFinish: resp.Totals.DidNotFinish,
			ReadingSeconds: resp.Totals.ReadingSeconds, ListeningSeconds: resp.Totals.ListeningSeconds,
			WordsRead: resp.Totals.WordsRead, AverageRating: resp.Totals.AverageRating,
		},
		Pace: StatsPace{
			WordsPerMinute: resp.Pace.WordsPerMinute, ComicPagesPerHour: resp.Pace.ComicPagesPerHour,
			PDFPagesPerHour: resp.Pace.PDFPagesPerHour, ListeningSpeed: resp.Pace.ListeningSpeed,
		},
		Periods: []StatsPeriod{}, Genres: counts(resp.Genres), Tags: counts(resp.Tags), Authors: counts(resp.Authors),
		Kinds: []StatsKindCount{}, Ratings: []StatsRatingCount{}, Places: []StatsPlace{},
	}
	if out.Years == nil {
		out.Years = []int{}
	}
	for _, p := range resp.Periods {
		out.Periods = append(out.Periods, StatsPeriod{Year: p.Year, Month: p.Month, Finished: p.Finished,
			ReadingSeconds: p.ReadingSeconds, ListeningSeconds: p.ListeningSeconds})
	}
	for _, k := range resp.Kinds {
		out.Kinds = append(out.Kinds, StatsKindCount{Kind: k.Kind, Books: k.Books})
	}
	for _, r := range resp.Ratings {
		out.Ratings = append(out.Ratings, StatsRatingCount{Rating: r.Rating, Books: r.Books})
	}
	for _, p := range resp.Places {
		out.Places = append(out.Places, StatsPlace{Source: p.Source, DeviceID: p.DeviceID, DeviceName: p.DeviceName,
			ReadingSeconds: p.ReadingSeconds, ListeningSeconds: p.ListeningSeconds})
	}
	return out
}

func counts(in []reading.ReadingStatsCount) []StatsCount {
	out := make([]StatsCount, 0, len(in))
	for _, c := range in {
		out = append(out, StatsCount{Name: c.Name, Books: c.Books})
	}
	return out
}

// stats sends the user's Reading Stats for a year, or all time.
func (h *handler) stats(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var q StatsQuery
	if err := c.Bind(&q); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.reading.Stats(ctx, user, q.Year)
	if err != nil {
		return err
	}
	zone, err := h.reading.TimeZone(ctx, user.ID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, statsFor(resp, zone)))
}
