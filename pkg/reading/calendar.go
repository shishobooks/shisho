package reading

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// StatsHeatmap returns user's Session time on each day of year, or of the
// last 12 months when year is nil, in their time zone and split at local
// midnight, and the first day they read anything.
func (svc *Service) StatsHeatmap(ctx context.Context, user *models.User, year *int) (*ReadingStatsHeatmapResponse, error) {
	loc, err := userLocation(ctx, svc.db, user.ID)
	if err != nil {
		return nil, err
	}
	y, m, d := svc.now().In(loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	// The last 12 months begin a year back from tomorrow, so a leap day
	// begins on March 1 rather than losing it to February 29 rolling over.
	from := time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).AddDate(-1, 0, 0)
	rng := dayRange{From: from.Format(time.DateOnly), To: today}
	if year != nil {
		rng = yearRange(year)
	}
	resp := &ReadingStatsHeatmapResponse{Today: today, From: rng.From, To: rng.To, Days: []ReadingStatsDay{}}

	sessions, err := svc.statsSessions(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}
	type dayTime struct{ reading, listening float64 }
	days := map[string]*dayTime{}
	for _, s := range sessions {
		for _, part := range splitByDay(s.StartedAt, s.EndedAt, s.Duration, loc) {
			if !rng.contains(&part.Day) {
				continue
			}
			t := days[part.Day]
			if t == nil {
				t = &dayTime{}
				days[part.Day] = t
			}
			if s.Kind == models.ReadingKindListening {
				t.listening += part.Seconds
			} else {
				t.reading += part.Seconds
			}
		}
	}
	for day, t := range days {
		row := ReadingStatsDay{Day: day, ReadingSeconds: roundSeconds(t.reading), ListeningSeconds: roundSeconds(t.listening)}
		if row.ReadingSeconds+row.ListeningSeconds > 0 {
			resp.Days = append(resp.Days, row)
		}
	}
	slices.SortFunc(resp.Days, func(a, b ReadingStatsDay) int { return cmp.Compare(a.Day, b.Day) })

	resp.FirstDay, err = svc.firstReadingDay(ctx, user.ID, loc)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// firstReadingDay is the earliest day userID read in loc, or began or ended
// a Read-through, or nil with neither.
func (svc *Service) firstReadingDay(ctx context.Context, userID int, loc *time.Location) (*string, error) {
	var days struct {
		Begin *string `bun:"begin_day"`
		End   *string `bun:"end_day"`
	}
	err := svc.db.NewSelect().
		TableExpr("read_throughs AS rt").
		ColumnExpr("MIN(rt.begin_day) AS begin_day, MIN(rt.end_day) AS end_day").
		Where("rt.user_id = ?", userID).
		Scan(ctx, &days)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var started []time.Time
	err = svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.started_at").
		Where("rs.user_id = ?", userID).
		OrderExpr("rs.started_at").
		Limit(1).
		Scan(ctx, &started)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var first *string
	candidates := []*string{days.Begin, days.End}
	if len(started) > 0 {
		day := started[0].In(loc).Format(time.DateOnly)
		candidates = append(candidates, &day)
	}
	for _, day := range candidates {
		if day != nil && (first == nil || *day < *first) {
			first = day
		}
	}
	return first, nil
}

// StatsCalendar returns one month of user's Reading Stats calendar: the
// Session time on each Book on each day, split at midnight in the user's
// time zone, and the days Read-throughs finished.
func (svc *Service) StatsCalendar(ctx context.Context, user *models.User, year, month int) (*ReadingStatsCalendarResponse, error) {
	loc, err := userLocation(ctx, svc.db, user.ID)
	if err != nil {
		return nil, err
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	rng := dayRange{From: first.Format(time.DateOnly), To: first.AddDate(0, 1, -1).Format(time.DateOnly)}

	sessions, err := svc.calendarSessions(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}
	finished, err := svc.finishedOn(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}

	type dayBook struct {
		seconds  float64
		finished bool
	}
	days := map[string]map[string]*dayBook{}
	at := func(day, key string) *dayBook {
		if days[day] == nil {
			days[day] = map[string]*dayBook{}
		}
		if days[day][key] == nil {
			days[day][key] = &dayBook{}
		}
		return days[day][key]
	}
	refs := newBookRefs()
	for _, s := range sessions {
		key := refs.addSession(s)
		for _, part := range splitByDay(s.StartedAt, s.EndedAt, s.Duration, loc) {
			if rng.contains(&part.Day) {
				at(part.Day, key).seconds += part.Seconds
			}
		}
	}
	for _, rt := range finished {
		at(*rt.EndDay, refs.addReadThrough(rt)).finished = true
	}

	resolved, err := refs.resolve(ctx, svc, user)
	if err != nil {
		return nil, err
	}
	resp := &ReadingStatsCalendarResponse{
		Year:  year,
		Month: month,
		Books: []ReadingStatsBookRef{},
		Days:  []ReadingStatsCalendarDay{},
	}
	shown := map[string]bool{}
	for day, books := range days {
		row := ReadingStatsCalendarDay{Day: day}
		for key, b := range books {
			seconds := roundSeconds(b.seconds)
			if seconds == 0 && !b.finished {
				// A Session that recorded no time is no reading that day.
				continue
			}
			row.Books = append(row.Books, ReadingStatsCalendarDayBook{Key: key, Seconds: seconds, Finished: b.finished})
			shown[key] = true
		}
		if len(row.Books) == 0 {
			continue
		}
		slices.SortFunc(row.Books, func(a, b ReadingStatsCalendarDayBook) int {
			return compareRefs(resolved[a.Key], resolved[b.Key])
		})
		resp.Days = append(resp.Days, row)
	}
	slices.SortFunc(resp.Days, func(a, b ReadingStatsCalendarDay) int { return cmp.Compare(a.Day, b.Day) })
	for key := range shown {
		resp.Books = append(resp.Books, resolved[key])
	}
	slices.SortFunc(resp.Books, compareRefs)
	return resp, nil
}

// StatsDay returns user's Reading Sessions that touched day in their time
// zone, in the order they started, and the Read-throughs finished that day.
func (svc *Service) StatsDay(ctx context.Context, user *models.User, day string) (*ReadingStatsDayResponse, error) {
	loc, err := userLocation(ctx, svc.db, user.ID)
	if err != nil {
		return nil, err
	}
	rng := dayRange{From: day, To: day}
	sessions, err := svc.calendarSessions(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}
	finished, err := svc.finishedOn(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}

	refs := newBookRefs()
	type onDay struct {
		session calendarSession
		key     string
		seconds float64
	}
	var touched []onDay
	for _, s := range sessions {
		for _, part := range splitByDay(s.StartedAt, s.EndedAt, s.Duration, loc) {
			if part.Day == day {
				touched = append(touched, onDay{session: s, key: refs.addSession(s), seconds: part.Seconds})
				break
			}
		}
	}
	finishedKeys := make([]string, 0, len(finished))
	for _, rt := range finished {
		finishedKeys = append(finishedKeys, refs.addReadThrough(rt))
	}
	resolved, err := refs.resolve(ctx, svc, user)
	if err != nil {
		return nil, err
	}

	ids := make([]*string, 0, len(touched))
	for _, t := range touched {
		ids = append(ids, t.session.SourceDeviceID)
	}
	names, err := deviceNames(ctx, svc.db, user.ID, presentIDs(ids))
	if err != nil {
		return nil, err
	}

	resp := &ReadingStatsDayResponse{
		Day:      day,
		TimeZone: loc.String(),
		Sessions: make([]ReadingStatsDaySession, 0, len(touched)),
		Finished: make([]ReadingStatsBookRef, 0, len(finishedKeys)),
	}
	for _, t := range touched {
		s := t.session
		session := ReadingStatsDaySession{
			ID:              s.ID,
			Book:            resolved[t.key],
			StartedAt:       s.StartedAt,
			EndedAt:         s.EndedAt,
			Seconds:         roundSeconds(t.seconds),
			StartPercentage: s.StartPercentage,
			EndPercentage:   s.EndPercentage,
			Kind:            s.Kind,
			Source:          s.Source,
			Measured:        s.Measured,
		}
		session.DeviceID, session.DeviceName = sessionDevice(names, s.SourceDeviceID)
		resp.Sessions = append(resp.Sessions, session)
	}
	for _, key := range finishedKeys {
		resp.Finished = append(resp.Finished, resolved[key])
	}
	return resp, nil
}

// calendarSession is a Reading Session with what names the Book it was in:
// its Read-through's Book, or the copy kept when that Book was deleted, or,
// with no Read-through, its File's Book.
type calendarSession struct {
	ID                int       `bun:"id"`
	StartedAt         time.Time `bun:"started_at"`
	EndedAt           time.Time `bun:"ended_at"`
	Duration          int       `bun:"duration"`
	Kind              string    `bun:"kind"`
	Source            string    `bun:"source"`
	Measured          bool      `bun:"measured"`
	StartPercentage   float64   `bun:"start_percentage"`
	EndPercentage     float64   `bun:"end_percentage"`
	ReadThroughID     *int      `bun:"read_through_id"`
	ReadThroughBookID *int      `bun:"rt_book_id"`
	ReadThroughTitle  *string   `bun:"rt_book_title"`
	FileBookID        *int      `bun:"file_book_id"`
	SourceDeviceID    *string   `bun:"source_device_id"`
}

// calendarSessions loads userID's Sessions that may touch rng in any time
// zone, in the order they started.
func (svc *Service) calendarSessions(ctx context.Context, userID int, rng dayRange) ([]calendarSession, error) {
	from, to := rng.instants()
	var sessions []calendarSession
	err := svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.id, rs.started_at, rs.ended_at, rs.duration, rs.kind, rs.source, rs.measured").
		ColumnExpr("rs.start_percentage, rs.end_percentage, rs.read_through_id, rs.source_device_id").
		ColumnExpr("rt.book_id AS rt_book_id, rt.book_title AS rt_book_title, f.book_id AS file_book_id").
		Join("LEFT JOIN read_throughs AS rt ON rt.id = rs.read_through_id").
		Join("LEFT JOIN files AS f ON f.id = rs.file_id").
		Where("rs.user_id = ?", userID).
		Where("rs.ended_at >= ? AND rs.started_at < ?", *from, *to).
		OrderExpr("rs.started_at, rs.id").
		Scan(ctx, &sessions)
	return sessions, errors.WithStack(err)
}

// finishedOn loads userID's finished Read-throughs that ended in rng.
func (svc *Service) finishedOn(ctx context.Context, userID int, rng dayRange) ([]*models.ReadThrough, error) {
	var finished []*models.ReadThrough
	err := svc.db.NewSelect().
		Model(&finished).
		Where("rt.user_id = ? AND rt.outcome = ?", userID, models.ReadThroughOutcomeFinished).
		Where("rt.end_day >= ? AND rt.end_day <= ?", rng.From, rng.To).
		OrderExpr("rt.id").
		Scan(ctx)
	return finished, errors.WithStack(err)
}

// bookRefs gathers the Books the calendar names, by key, so their titles
// and links load in one go.
type bookRefs struct {
	bookIDs  []int
	detached map[string]string
	// unnamed are Sessions whose File and Book are gone with no
	// Read-through to say what they were. Nothing ties two of them to one
	// Book, so each stands alone.
	unnamed []string
}

func newBookRefs() *bookRefs {
	return &bookRefs{detached: map[string]string{}}
}

func (r *bookRefs) addBook(id int) string {
	if !slices.Contains(r.bookIDs, id) {
		r.bookIDs = append(r.bookIDs, id)
	}
	return "book-" + strconv.Itoa(id)
}

func (r *bookRefs) addDetached(readThroughID int, title *string) string {
	key := "read-through-" + strconv.Itoa(readThroughID)
	r.detached[key] = ptrString(title)
	return key
}

func (r *bookRefs) addSession(s calendarSession) string {
	switch {
	case s.ReadThroughID != nil && s.ReadThroughBookID != nil:
		return r.addBook(*s.ReadThroughBookID)
	case s.ReadThroughID != nil:
		return r.addDetached(*s.ReadThroughID, s.ReadThroughTitle)
	case s.FileBookID != nil:
		return r.addBook(*s.FileBookID)
	}
	key := "session-" + strconv.Itoa(s.ID)
	r.unnamed = append(r.unnamed, key)
	return key
}

func (r *bookRefs) addReadThrough(rt *models.ReadThrough) string {
	if rt.BookID != nil {
		return r.addBook(*rt.BookID)
	}
	return r.addDetached(rt.ID, rt.BookTitle)
}

// resolve names every key added, linking each Book user can still reach.
func (r *bookRefs) resolve(ctx context.Context, svc *Service, user *models.User) (map[string]ReadingStatsBookRef, error) {
	books, err := svc.loadBooks(ctx, r.bookIDs)
	if err != nil {
		return nil, err
	}
	refs := map[string]ReadingStatsBookRef{}
	for _, key := range r.unnamed {
		refs[key] = ReadingStatsBookRef{Key: key, Removed: true}
	}
	for _, id := range r.bookIDs {
		key := "book-" + strconv.Itoa(id)
		book := books[id]
		if book == nil {
			// Deleted since the Sessions were read.
			refs[key] = ReadingStatsBookRef{Key: key, Removed: true}
			continue
		}
		refs[key] = ReadingStatsBookRef{Key: key, Title: book.Title, Book: bookLink(user, book)}
	}
	for key, title := range r.detached {
		refs[key] = ReadingStatsBookRef{Key: key, Title: title, Removed: true}
	}
	return refs, nil
}

func compareRefs(a, b ReadingStatsBookRef) int {
	return cmp.Or(cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)), cmp.Compare(a.Key, b.Key))
}

// ReadThroughSessions lists userID's Reading Sessions of readThroughID, in
// the order they started. The caller checks the Read-through is theirs.
func (svc *Service) ReadThroughSessions(ctx context.Context, userID, readThroughID int) (*ReadThroughSessionsResponse, error) {
	loc, err := userLocation(ctx, svc.db, userID)
	if err != nil {
		return nil, err
	}
	var sessions []*models.ReadingSession
	err = svc.db.NewSelect().
		Model(&sessions).
		Where("rs.user_id = ? AND rs.read_through_id = ?", userID, readThroughID).
		OrderExpr("rs.started_at, rs.id").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ids := make([]*string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.SourceDeviceID)
	}
	names, err := deviceNames(ctx, svc.db, userID, presentIDs(ids))
	if err != nil {
		return nil, err
	}
	resp := &ReadThroughSessionsResponse{TimeZone: loc.String(), Items: make([]ReadThroughSession, 0, len(sessions))}
	for _, s := range sessions {
		item := ReadThroughSession{
			ID:              s.ID,
			StartedAt:       s.StartedAt,
			EndedAt:         s.EndedAt,
			Seconds:         s.Duration,
			StartPercentage: s.StartPercentage,
			EndPercentage:   s.EndPercentage,
			Kind:            s.Kind,
			Source:          s.Source,
			Measured:        s.Measured,
		}
		item.DeviceID, item.DeviceName = sessionDevice(names, s.SourceDeviceID)
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}
