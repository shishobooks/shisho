package reading

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The transitions return these when the Book's current Read-throughs don't
// allow them, which the handlers render as 422 invalid_state.
var (
	ErrAlreadyUnderway = errors.New("a read-through is already underway")
	ErrNothingUnderway = errors.New("no read-through is underway")
	ErrAlreadyPaused   = errors.New("the read-through is already paused")
	ErrNotPaused       = errors.New("the read-through is not paused")
)

// UpdateReadThroughDays returns these for days it rejects, which the
// handlers render as 422 validation_error.
var (
	ErrEndDayWhileUnderway = errors.New("a read-through underway has no end day")
	ErrEndBeforeBegin      = errors.New("the end day is before the begin day")
)

// ErrReadThroughNotFound is returned for a Read-through that doesn't exist
// or belongs to another user.
var ErrReadThroughNotFound = errors.New("read-through not found")

// maxRatingUnits is five stars in quarter-star units.
const maxRatingUnits = 20

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

// StatusExpr is the SQL expression for userID's Reading Status of the Book
// whose id is in bookIDColumn, or NULL when there is none. bookIDColumn is
// interpolated into the SQL, so it must be a column name or an integer
// literal from code, never request input. It is the one
// place the status is derived (ADR 0013), so listings that show, filter, or
// sort on it use this expression:
//
//  1. A Read-through underway: currently_reading, or paused.
//  2. Otherwise, on To Read: to_read.
//  3. Otherwise, the latest ended Read-through: read or did_not_finish.
//     Latest means the latest end day; undated ones rank below dated ones,
//     and ties go to the one recorded last.
func StatusExpr(userID int, bookIDColumn string) string {
	return fmt.Sprintf(`(CASE
		WHEN EXISTS (SELECT 1 FROM read_throughs rts WHERE rts.user_id = %[1]d AND rts.book_id = %[2]s AND rts.outcome = '%[3]s' AND NOT rts.paused) THEN '%[6]s'
		WHEN EXISTS (SELECT 1 FROM read_throughs rts WHERE rts.user_id = %[1]d AND rts.book_id = %[2]s AND rts.outcome = '%[3]s' AND rts.paused) THEN '%[7]s'
		WHEN EXISTS (SELECT 1 FROM user_books ubs WHERE ubs.user_id = %[1]d AND ubs.book_id = %[2]s AND ubs.to_read_position IS NOT NULL) THEN '%[8]s'
		ELSE (
			SELECT CASE rts.outcome WHEN '%[4]s' THEN '%[9]s' WHEN '%[5]s' THEN '%[10]s' END
			FROM read_throughs rts
			WHERE rts.user_id = %[1]d AND rts.book_id = %[2]s AND rts.outcome != '%[3]s'
			ORDER BY rts.end_day IS NULL, rts.end_day DESC, rts.id DESC
			LIMIT 1
		)
	END)`,
		userID, bookIDColumn,
		models.ReadThroughOutcomeUnderway, models.ReadThroughOutcomeFinished, models.ReadThroughOutcomeAbandoned,
		models.ReadingStatusCurrentlyReading, models.ReadingStatusPaused, models.ReadingStatusToRead,
		models.ReadingStatusRead, models.ReadingStatusDidNotFinish,
	)
}

// RatingUnitsExpr is the SQL expression for userID's Rating, in
// quarter-star units, of the Book whose id is in bookIDColumn, or NULL when
// unrated. bookIDColumn is interpolated as in StatusExpr.
func RatingUnitsExpr(userID int, bookIDColumn string) string {
	return fmt.Sprintf(`(SELECT ubr.rating FROM user_books ubr WHERE ubr.user_id = %d AND ubr.book_id = %s)`, userID, bookIDColumn)
}

// LastFinishedDayExpr is the SQL expression for the end day of userID's
// latest finished Read-through of the Book whose id is in bookIDColumn, or
// NULL when no finished Read-through has an end day. bookIDColumn is
// interpolated as in StatusExpr.
func LastFinishedDayExpr(userID int, bookIDColumn string) string {
	return fmt.Sprintf(`(SELECT MAX(rtf.end_day) FROM read_throughs rtf WHERE rtf.user_id = %d AND rtf.book_id = %s AND rtf.outcome = '%s')`,
		userID, bookIDColumn, models.ReadThroughOutcomeFinished)
}

// UnderwayOrderExpr is the SQL ORDER BY for userID's Books underway, the
// Book whose id is in bookIDColumn: the ones being read before the paused
// ones, each most recently begun first. bookIDColumn is interpolated as in
// StatusExpr.
func UnderwayOrderExpr(userID int, bookIDColumn string) string {
	underway := func(column string) string {
		return fmt.Sprintf(`(SELECT rtu.%s FROM read_throughs rtu WHERE rtu.user_id = %d AND rtu.book_id = %s AND rtu.outcome = '%s')`,
			column, userID, bookIDColumn, models.ReadThroughOutcomeUnderway)
	}
	return fmt.Sprintf("%[1]s ASC, %[2]s IS NULL, %[2]s DESC, %[3]s DESC", underway("paused"), underway("begin_day"), underway("id"))
}

// UnderwayFilterExpr is the SQL condition that userID has a Read-through
// underway on the Book whose id is in bookIDColumn, interpolated as in
// StatusExpr.
func UnderwayFilterExpr(userID int, bookIDColumn string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM read_throughs rtu WHERE rtu.user_id = %d AND rtu.book_id = %s AND rtu.outcome = '%s')`,
		userID, bookIDColumn, models.ReadThroughOutcomeUnderway)
}

// Filter keeps the Books whose reader's Reading Status is any of Statuses,
// where models.ReadingStatusFilterNone matches Books with none, and that the
// reader rated within RatingMin and RatingMax stars, both ends included.
// Empty fields don't filter.
type Filter struct {
	Statuses  []string
	RatingMin *float64
	RatingMax *float64
}

// Validate refuses a rating range whose minimum is above its maximum.
func (f Filter) Validate() error {
	if f.RatingMin != nil && f.RatingMax != nil && *f.RatingMin > *f.RatingMax {
		return errcodes.ValidationError("The minimum rating is above the maximum.")
	}
	return nil
}

// Apply adds f, read from readerID's reading data, to q, whose Book id is
// in bookIDColumn, interpolated as in StatusExpr.
func (f Filter) Apply(q *bun.SelectQuery, readerID int, bookIDColumn string) *bun.SelectQuery {
	if len(f.Statuses) > 0 {
		// No status reads as "none", so one IN matches any mix of values
		// and the status is derived once per row.
		q = q.Where(fmt.Sprintf("COALESCE(%s, '%s') IN (?)", StatusExpr(readerID, bookIDColumn), models.ReadingStatusFilterNone), bun.List(f.Statuses))
	}
	rating := RatingUnitsExpr(readerID, bookIDColumn)
	if f.RatingMin != nil {
		q = q.Where(rating+" >= ?", *f.RatingMin*4)
	}
	if f.RatingMax != nil {
		q = q.Where(rating+" <= ?", *f.RatingMax*4)
	}
	return q
}

// AttachReading sets ReadingStatus and Rating on each Book to userID's.
// Only a listing served to userID may call it, since both are private.
func AttachReading(ctx context.Context, db bun.IDB, userID int, books []*models.Book) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]int, 0, len(books))
	for _, b := range books {
		ids = append(ids, b.ID)
	}
	type row struct {
		ID          int     `bun:"id"`
		Status      *string `bun:"status"`
		RatingUnits *int    `bun:"rating"`
	}
	var rows []row
	err := db.NewSelect().
		TableExpr("books AS b").
		Column("b.id").
		ColumnExpr(StatusExpr(userID, "b.id")+" AS status").
		ColumnExpr("ub.rating").
		Join("LEFT JOIN user_books AS ub ON ub.book_id = b.id AND ub.user_id = ?", userID).
		Where("b.id IN (?)", bun.List(ids)).
		Scan(ctx, &rows)
	if err != nil {
		return errors.WithStack(err)
	}
	byID := make(map[int]row, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, b := range books {
		r := byID[b.ID]
		b.ReadingStatus = r.Status
		b.Rating = RatingStars(r.RatingUnits)
	}
	return nil
}

// BookReading is one user's reading record for one Book.
type BookReading struct {
	Status       *string
	RatingUnits  *int
	OnToRead     bool
	ReadThroughs []*models.ReadThrough
}

// BookLibraryID returns the library of bookID, or sql.ErrNoRows when there
// is no such Book.
func (svc *Service) BookLibraryID(ctx context.Context, bookID int) (int, error) {
	var libraryID int
	err := svc.db.NewSelect().
		Model((*models.Book)(nil)).
		Column("b.library_id").
		Where("b.id = ?", bookID).
		Scan(ctx, &libraryID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return libraryID, errors.WithStack(err)
}

// RetrieveBookReading loads userID's reading record for bookID.
func (svc *Service) RetrieveBookReading(ctx context.Context, userID, bookID int) (*BookReading, error) {
	result := &BookReading{ReadThroughs: []*models.ReadThrough{}}

	err := svc.db.NewSelect().
		ColumnExpr(StatusExpr(userID, strconv.Itoa(bookID))+" AS status").
		Scan(ctx, &result.Status)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = svc.db.NewSelect().
		Model(&result.ReadThroughs).
		Where("rt.user_id = ? AND rt.book_id = ?", userID, bookID).
		OrderExpr("rt.outcome = ? DESC, rt.end_day IS NULL, rt.end_day DESC, rt.id DESC", models.ReadThroughOutcomeUnderway).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	userBook := &models.UserBook{}
	err = svc.db.NewSelect().
		Model(userBook).
		Where("ub.user_id = ? AND ub.book_id = ?", userID, bookID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	if err == nil {
		result.RatingUnits = userBook.Rating
		result.OnToRead = userBook.ToReadPosition != nil
	}

	return result, nil
}

// ToReadPlace returns bookID's 1-based place on userID's To Read, counting
// only Books in libraryIDs (nil for every library), as the To Read page
// lists them. Call it only for a Book on To Read.
func (svc *Service) ToReadPlace(ctx context.Context, userID, bookID int, libraryIDs []int) (int, error) {
	q := svc.db.NewSelect().
		TableExpr("user_books AS other").
		Join("JOIN user_books AS this ON this.user_id = other.user_id AND this.book_id = ?", bookID).
		Where("other.user_id = ? AND other.to_read_position IS NOT NULL", userID).
		// The To Read listing orders by position, then book id.
		Where("(other.to_read_position < this.to_read_position OR (other.to_read_position = this.to_read_position AND other.book_id < this.book_id))")
	if libraryIDs != nil {
		q = q.Where("other.book_id IN (SELECT id FROM books WHERE library_id IN (?))", bun.List(libraryIDs))
	}
	ahead, err := q.Count(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return int(ahead) + 1, nil
}

// Start begins a new Read-through on today and takes the Book off To Read.
// Reading a finished or abandoned Book again is the same transition.
func (svc *Service) Start(ctx context.Context, userID, bookID int, today string) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewInsert().
			Model(&models.ReadThrough{
				CreatedAt: now,
				UpdatedAt: now,
				UserID:    userID,
				BookID:    bookID,
				BeginDay:  &today,
				Outcome:   models.ReadThroughOutcomeUnderway,
			}).
			Exec(ctx)
		if database.IsUniqueViolation(err) {
			return ErrAlreadyUnderway
		}
		if err != nil {
			return errors.WithStack(err)
		}

		return leaveToRead(ctx, tx, userID, bookID, now)
	})
}

// leaveToRead takes bookID off userID's To Read, if it's there.
func leaveToRead(ctx context.Context, db bun.IDB, userID, bookID int, now time.Time) error {
	_, err := db.NewUpdate().
		Model((*models.UserBook)(nil)).
		Set("to_read_position = NULL").
		Set("updated_at = ?", now).
		Where("user_id = ? AND book_id = ? AND to_read_position IS NOT NULL", userID, bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// SetPaused pauses or resumes the Read-through underway.
func (svc *Service) SetPaused(ctx context.Context, userID, bookID int, paused bool) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		underway, err := retrieveUnderway(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		if underway == nil {
			return ErrNothingUnderway
		}
		if underway.Paused == paused {
			if paused {
				return ErrAlreadyPaused
			}
			return ErrNotPaused
		}
		underway.Paused = paused
		underway.UpdatedAt = time.Now()
		_, err = tx.NewUpdate().Model(underway).Column("paused", "updated_at").WherePK().Exec(ctx)
		return errors.WithStack(err)
	})
}

// End ends the Read-through underway on today with outcome. Finishing with
// nothing underway records a finished Read-through with no begin day, for a
// Book read before the user tracked it; abandoning needs one underway.
// Either way the Book leaves To Read, so a queued Book marked read shows
// Read; adding it again afterwards plans a reread.
func (svc *Service) End(ctx context.Context, userID, bookID int, outcome, today string) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		underway, err := retrieveUnderway(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		if underway == nil {
			if outcome != models.ReadThroughOutcomeFinished {
				return ErrNothingUnderway
			}
			_, err = tx.NewInsert().
				Model(&models.ReadThrough{
					CreatedAt: now,
					UpdatedAt: now,
					UserID:    userID,
					BookID:    bookID,
					EndDay:    &today,
					Outcome:   outcome,
				}).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			return leaveToRead(ctx, tx, userID, bookID, now)
		}

		underway.Outcome = outcome
		underway.Paused = false
		underway.EndDay = &today
		underway.UpdatedAt = now
		_, err = tx.NewUpdate().
			Model(underway).
			Column("outcome", "paused", "end_day", "updated_at").
			WherePK().
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		return leaveToRead(ctx, tx, userID, bookID, now)
	})
}

func retrieveUnderway(ctx context.Context, db bun.IDB, userID, bookID int) (*models.ReadThrough, error) {
	rt := &models.ReadThrough{}
	err := db.NewSelect().
		Model(rt).
		Where("rt.user_id = ? AND rt.book_id = ? AND rt.outcome = ?", userID, bookID, models.ReadThroughOutcomeUnderway).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rt, nil
}

// RetrieveReadThrough loads one of userID's Read-throughs, returning
// ErrReadThroughNotFound when it doesn't exist or belongs to someone else.
func (svc *Service) RetrieveReadThrough(ctx context.Context, userID, id int) (*models.ReadThrough, error) {
	rt := &models.ReadThrough{}
	err := svc.db.NewSelect().
		Model(rt).
		Where("rt.id = ? AND rt.user_id = ?", id, userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReadThroughNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rt, nil
}

// UpdateReadThroughDays replaces both days of rt. Nil clears a day.
func (svc *Service) UpdateReadThroughDays(ctx context.Context, rt *models.ReadThrough, beginDay, endDay *string) error {
	if endDay != nil && rt.Outcome == models.ReadThroughOutcomeUnderway {
		return ErrEndDayWhileUnderway
	}
	// Days are YYYY-MM-DD, so they compare as strings.
	if beginDay != nil && endDay != nil && *endDay < *beginDay {
		return ErrEndBeforeBegin
	}
	rt.BeginDay = beginDay
	rt.EndDay = endDay
	rt.UpdatedAt = time.Now()
	_, err := svc.db.NewUpdate().
		Model(rt).
		Column("begin_day", "end_day", "updated_at").
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteReadThrough deletes rt: the Read-through no longer counts.
func (svc *Service) DeleteReadThrough(ctx context.Context, rt *models.ReadThrough) error {
	_, err := svc.db.NewDelete().Model(rt).WherePK().Exec(ctx)
	return errors.WithStack(err)
}

// AddToRead puts bookID at the end of userID's To Read. A Book already on
// To Read keeps its place. A Book with a Read-through underway is refused
// with ErrAlreadyUnderway: starting takes a Book off To Read, and its status
// would stay Currently Reading or Paused.
func (svc *Service) AddToRead(ctx context.Context, userID, bookID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		underway, err := retrieveUnderway(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		if underway != nil {
			return ErrAlreadyUnderway
		}
		_, err = appendToRead(ctx, tx, userID, []int{bookID})
		return err
	})
}

// AppendToRead puts bookIDs at the end of userID's To Read in their order,
// skipping Books already on it and Books with a Read-through underway, and
// returns how many it added. The caller checks the user can reach each Book.
func (svc *Service) AppendToRead(ctx context.Context, userID int, bookIDs []int) (int, error) {
	added := 0
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		var err error
		added, err = appendToRead(ctx, tx, userID, bookIDs)
		return err
	})
	return added, err
}

// appendToRead puts bookIDs after the last Book on userID's To Read, in
// their order, skipping any already on it or with a Read-through underway,
// and returns how many it added. It reads the last position once, so run
// it in a transaction; the server's one database connection runs
// transactions one at a time, so a concurrent add can't take the same
// positions.
func appendToRead(ctx context.Context, tx bun.Tx, userID int, bookIDs []int) (int, error) {
	var last int
	err := tx.NewSelect().
		Model((*models.UserBook)(nil)).
		ColumnExpr("COALESCE(MAX(to_read_position), 0)").
		Where("user_id = ?", userID).
		Scan(ctx, &last)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	added := 0
	now := time.Now()
	for _, bookID := range bookIDs {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO user_books (user_id, book_id, created_at, updated_at, to_read_position)
			SELECT ?, ?, ?, ?, ?
			WHERE NOT EXISTS (SELECT 1 FROM read_throughs WHERE user_id = ? AND book_id = ? AND outcome = ?)
			ON CONFLICT (user_id, book_id) DO UPDATE
				SET to_read_position = EXCLUDED.to_read_position, updated_at = EXCLUDED.updated_at
				WHERE user_books.to_read_position IS NULL`,
			userID, bookID, now, now, last+added+1,
			userID, bookID, models.ReadThroughOutcomeUnderway,
		)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, errors.WithStack(err)
		}
		added += int(n)
	}
	return added, nil
}

// RemoveFromToRead takes bookID off userID's To Read, if it's there.
func (svc *Service) RemoveFromToRead(ctx context.Context, userID, bookID int) error {
	return leaveToRead(ctx, svc.db, userID, bookID, time.Now())
}

// ReorderToRead puts bookIDs in this order on userID's To Read. The Books
// it names move among the places they held, so Books it leaves out (in a
// library the user can no longer reach) keep theirs; ids not on To Read are
// ignored. Positions are renumbered from 1.
func (svc *Service) ReorderToRead(ctx context.Context, userID int, bookIDs []int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		var current []*models.UserBook
		err := tx.NewSelect().
			Model(&current).
			Where("ub.user_id = ? AND ub.to_read_position IS NOT NULL", userID).
			Order("ub.to_read_position ASC", "ub.book_id ASC").
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		queued := make(map[int]bool, len(current))
		for _, ub := range current {
			queued[ub.BookID] = true
		}
		named := make([]int, 0, len(bookIDs))
		seen := make(map[int]bool, len(bookIDs))
		for _, id := range bookIDs {
			if queued[id] && !seen[id] {
				named = append(named, id)
				seen[id] = true
			}
		}

		order := make([]int, len(current))
		next := 0
		for i, ub := range current {
			if seen[ub.BookID] {
				order[i] = named[next]
				next++
			} else {
				order[i] = ub.BookID
			}
		}

		now := time.Now()
		for i, bookID := range order {
			position := i + 1
			if current[i].BookID == bookID && *current[i].ToReadPosition == position {
				continue
			}
			_, err := tx.NewUpdate().
				Model((*models.UserBook)(nil)).
				Set("to_read_position = ?", position).
				Set("updated_at = ?", now).
				Where("user_id = ? AND book_id = ?", userID, bookID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}

// RatingStars converts quarter-star units to stars, keeping nil as nil.
func RatingStars(units *int) *float64 {
	if units == nil {
		return nil
	}
	stars := float64(*units) / 4
	return &stars
}

// RatingUnits converts stars to quarter-star units, reporting false for a
// value that isn't a quarter step from 0.25 to 5.
func RatingUnits(stars float64) (int, bool) {
	units := stars * 4
	rounded := math.Round(units)
	if math.Abs(units-rounded) > 1e-9 || rounded < 1 || rounded > maxRatingUnits {
		return 0, false
	}
	return int(rounded), true
}

// SetRating sets userID's Rating of bookID in quarter-star units, or clears
// it when units is nil.
func (svc *Service) SetRating(ctx context.Context, userID, bookID int, units *int) error {
	now := time.Now()
	_, err := svc.db.NewInsert().
		Model(&models.UserBook{
			UserID:    userID,
			BookID:    bookID,
			CreatedAt: now,
			UpdatedAt: now,
			Rating:    units,
		}).
		On("CONFLICT (user_id, book_id) DO UPDATE").
		Set("rating = EXCLUDED.rating").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return errors.WithStack(err)
}

// MergeBooks moves every user's reading data from sourceBookID to
// targetBookID, for a merge that is about to delete the source. Run it in
// the transaction that deletes the source, before the delete cascades the
// source's rows away.
//
//   - The target keeps its Rating and To Read position, taking the source's
//     where it has none.
//   - The source's Read-throughs move to the target. When both have one
//     underway, they collapse into the target's, which takes the earlier
//     begin day and stays paused only if both were.
//   - A user with a Read-through underway on the target afterwards has it
//     off To Read.
func MergeBooks(ctx context.Context, db bun.IDB, sourceBookID, targetBookID int) error {
	if sourceBookID == targetBookID {
		return nil
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE user_books AS t
			SET rating = COALESCE(t.rating, s.rating),
				to_read_position = COALESCE(t.to_read_position, s.to_read_position)
			FROM user_books AS s
			WHERE s.book_id = ? AND t.book_id = ? AND s.user_id = t.user_id`,
			[]any{sourceBookID, targetBookID}},
		{`DELETE FROM user_books
			WHERE book_id = ? AND user_id IN (SELECT user_id FROM user_books WHERE book_id = ?)`,
			[]any{sourceBookID, targetBookID}},
		{`UPDATE user_books SET book_id = ? WHERE book_id = ?`,
			[]any{targetBookID, sourceBookID}},
		// A NULL begin day is unknown, so the earlier of two is the earlier
		// known one.
		{`UPDATE read_throughs AS t
			SET begin_day = CASE
					WHEN t.begin_day IS NULL THEN s.begin_day
					WHEN s.begin_day IS NULL THEN t.begin_day
					ELSE MIN(t.begin_day, s.begin_day)
				END,
				paused = t.paused AND s.paused
			FROM read_throughs AS s
			WHERE s.book_id = ? AND t.book_id = ? AND s.user_id = t.user_id
				AND s.outcome = ? AND t.outcome = ?`,
			[]any{sourceBookID, targetBookID, models.ReadThroughOutcomeUnderway, models.ReadThroughOutcomeUnderway}},
		{`DELETE FROM read_throughs
			WHERE book_id = ? AND outcome = ?
				AND user_id IN (SELECT user_id FROM read_throughs WHERE book_id = ? AND outcome = ?)`,
			[]any{sourceBookID, models.ReadThroughOutcomeUnderway, targetBookID, models.ReadThroughOutcomeUnderway}},
		{`UPDATE read_throughs SET book_id = ? WHERE book_id = ?`,
			[]any{targetBookID, sourceBookID}},
		// A Read-through underway takes the Book off To Read, as starting
		// one does.
		{`UPDATE user_books SET to_read_position = NULL
			WHERE book_id = ? AND to_read_position IS NOT NULL
				AND user_id IN (SELECT user_id FROM read_throughs WHERE book_id = ? AND outcome = ?)`,
			[]any{targetBookID, targetBookID, models.ReadThroughOutcomeUnderway}},
	}
	for _, s := range statements {
		if _, err := db.ExecContext(ctx, s.query, s.args...); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
