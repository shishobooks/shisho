package reading

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Stats returns user's Reading Stats for year, or for all time when year is
// nil. Read-throughs count by the day they ended, as stored; Session time is
// grouped into days in the user's time zone.
func (svc *Service) Stats(ctx context.Context, user *models.User, year *int) (*ReadingStatsResponse, error) {
	loc, err := userLocation(ctx, svc.db, user.ID)
	if err != nil {
		return nil, err
	}
	rng := yearRange(year)

	var readThroughs []*models.ReadThrough
	err = svc.db.NewSelect().
		Model(&readThroughs).
		Where("rt.user_id = ?", user.ID).
		OrderExpr("rt.id").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sessions, err := svc.statsSessions(ctx, user.ID, rng)
	if err != nil {
		return nil, err
	}
	years, err := svc.activityYears(ctx, user.ID, loc, readThroughs)
	if err != nil {
		return nil, err
	}

	resp := &ReadingStatsResponse{
		Year:    year,
		Years:   years,
		Periods: []ReadingStatsPeriod{},
		Kinds:   []ReadingStatsKindCount{},
		Ratings: []ReadingStatsRatingCount{},
		Places:  []ReadingStatsPlace{},
	}

	periods := newPeriodTable(year, years)
	if err := svc.addSessionTime(ctx, resp, user.ID, periods, sessions, rng, loc); err != nil {
		return nil, err
	}

	rereads := rereadIDs(readThroughs)
	var finished []*models.ReadThrough
	var spans []int
	for _, rt := range readThroughs {
		if rt.Outcome == models.ReadThroughOutcomeUnderway || !rng.contains(rt.EndDay) {
			continue
		}
		if rt.Outcome == models.ReadThroughOutcomeAbandoned {
			resp.Totals.DidNotFinish++
			continue
		}
		finished = append(finished, rt)
		resp.Totals.Finished++
		if rereads[rt.ID] {
			resp.Totals.Rereads++
		}
		if days := daysTaken(rt); days != nil {
			spans = append(spans, *days)
		}
		if rt.EndDay != nil {
			periods.addFinish(*rt.EndDay)
		}
	}
	resp.Totals.MedianDaysToFinish = median(spans)
	resp.Periods = periods.rows()
	// The totals are the periods' sums, so the chart adds up to them. Every
	// day in the range falls in a period.
	resp.Totals.ReadingSeconds, resp.Totals.ListeningSeconds = 0, 0
	for _, p := range resp.Periods {
		resp.Totals.ReadingSeconds += p.ReadingSeconds
		resp.Totals.ListeningSeconds += p.ListeningSeconds
	}

	if err := svc.addBreakdowns(ctx, resp, user.ID, finished); err != nil {
		return nil, err
	}
	return resp, nil
}

// statsSession is what Reading Stats needs from one Reading Session.
type statsSession struct {
	StartedAt      time.Time `bun:"started_at"`
	EndedAt        time.Time `bun:"ended_at"`
	Duration       int       `bun:"duration"`
	Kind           string    `bun:"kind"`
	Source         string    `bun:"source"`
	SourceDeviceID *string   `bun:"source_device_id"`
}

// statsSessions loads userID's Sessions that may touch rng in any time zone.
func (svc *Service) statsSessions(ctx context.Context, userID int, rng dayRange) ([]statsSession, error) {
	var sessions []statsSession
	q := svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.started_at, rs.ended_at, rs.duration, rs.kind, rs.source, rs.source_device_id").
		Where("rs.user_id = ?", userID)
	if from, to := rng.instants(); from != nil {
		q = q.Where("rs.ended_at >= ? AND rs.started_at < ?", *from, *to)
	}
	if err := q.Scan(ctx, &sessions); err != nil {
		return nil, errors.WithStack(err)
	}
	return sessions, nil
}

// newYearDays are the UTC days (MM-DD) within a day of New Year, where a
// Session's local year can differ from its UTC year.
var newYearDays = []string{"12-31", "01-01"}

// activityYears are the years, newest first, in which userID began or ended
// a Read-through or has Session time in their time zone.
func (svc *Service) activityYears(ctx context.Context, userID int, loc *time.Location, readThroughs []*models.ReadThrough) ([]int, error) {
	seen := map[int]bool{}
	for _, rt := range readThroughs {
		for _, day := range []*string{rt.BeginDay, rt.EndDay} {
			if day == nil {
				continue
			}
			if y, err := strconv.Atoi((*day)[:4]); err == nil {
				seen[y] = true
			}
		}
	}
	// Away from New Year in UTC, a Session's UTC years are its local years
	// in every zone, so SQL can list them from the stored UTC text without
	// loading each Session. Only Sessions starting or ending within a day of
	// New Year need their instants placed in the user's zone.
	var spans []struct {
		StartYear string `bun:"start_year"`
		EndYear   string `bun:"end_year"`
	}
	err := svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("DISTINCT substr(rs.started_at, 1, 4) AS start_year, substr(rs.ended_at, 1, 4) AS end_year").
		Where("rs.user_id = ?", userID).
		Where("substr(rs.started_at, 6, 5) NOT IN (?) AND substr(rs.ended_at, 6, 5) NOT IN (?)", bun.List(newYearDays), bun.List(newYearDays)).
		Scan(ctx, &spans)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, span := range spans {
		start, err1 := strconv.Atoi(span.StartYear)
		end, err2 := strconv.Atoi(span.EndYear)
		if err1 != nil || err2 != nil {
			continue
		}
		for y := start; y <= end; y++ {
			seen[y] = true
		}
	}
	var edges []struct {
		StartedAt time.Time `bun:"started_at"`
		EndedAt   time.Time `bun:"ended_at"`
	}
	err = svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.started_at, rs.ended_at").
		Where("rs.user_id = ?", userID).
		Where("substr(rs.started_at, 6, 5) IN (?) OR substr(rs.ended_at, 6, 5) IN (?)", bun.List(newYearDays), bun.List(newYearDays)).
		Scan(ctx, &edges)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, edge := range edges {
		end := edge.EndedAt.In(loc).Year()
		for y := edge.StartedAt.In(loc).Year(); y <= end; y++ {
			seen[y] = true
		}
	}
	years := make([]int, 0, len(seen))
	for y := range seen {
		years = append(years, y)
	}
	slices.SortFunc(years, func(a, b int) int { return cmp.Compare(b, a) })
	return years, nil
}

// periodTable accumulates the by-month figures of one year, or the by-year
// figures of all time.
type periodTable struct {
	byYear  bool
	periods []*periodTotals
	index   map[string]*periodTotals
}

type periodTotals struct {
	ReadingStatsPeriod
	reading, listening float64
}

func newPeriodTable(year *int, years []int) *periodTable {
	t := &periodTable{byYear: year == nil, index: map[string]*periodTotals{}}
	add := func(y int, m *int, key string) {
		p := &periodTotals{ReadingStatsPeriod: ReadingStatsPeriod{Year: y, Month: m}}
		t.periods = append(t.periods, p)
		t.index[key] = p
	}
	if year != nil {
		for m := 1; m <= 12; m++ {
			month := m
			add(*year, &month, time.Date(*year, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01"))
		}
		return t
	}
	if len(years) == 0 {
		return t
	}
	for y := years[len(years)-1]; y <= years[0]; y++ {
		add(y, nil, strconv.Itoa(y))
	}
	return t
}

// at is the period day falls in, or nil when it's outside the table.
func (t *periodTable) at(day string) *periodTotals {
	if t.byYear {
		return t.index[day[:4]]
	}
	return t.index[day[:7]]
}

func (t *periodTable) addFinish(day string) {
	if p := t.at(day); p != nil {
		p.Finished++
	}
}

func (t *periodTable) rows() []ReadingStatsPeriod {
	rows := make([]ReadingStatsPeriod, 0, len(t.periods))
	for _, p := range t.periods {
		row := p.ReadingStatsPeriod
		row.ReadingSeconds = roundSeconds(p.reading)
		row.ListeningSeconds = roundSeconds(p.listening)
		rows = append(rows, row)
	}
	return rows
}

// addSessionTime adds the Session time that fell on days in rng to the
// periods, and to Places, which splits it by where it was read: the Shisho
// reader, each of the user's Devices, or, for a Device since deleted, the
// kind of reader it was.
func (svc *Service) addSessionTime(ctx context.Context, resp *ReadingStatsResponse, userID int, periods *periodTable, sessions []statsSession, rng dayRange, loc *time.Location) error {
	type place struct {
		source             string
		deviceID           string
		reading, listening float64
	}
	places := map[[2]string]*place{}
	var deviceIDs []string
	for _, s := range sessions {
		key := [2]string{s.Source, ""}
		if s.Source != models.ReadingSourceWebReader && s.SourceDeviceID != nil {
			key[1] = *s.SourceDeviceID
		}
		listening := s.Kind == models.ReadingKindListening
		for _, part := range splitByDay(s.StartedAt, s.EndedAt, s.Duration, loc) {
			if !rng.contains(&part.Day) {
				continue
			}
			p := places[key]
			if p == nil {
				p = &place{source: key[0], deviceID: key[1]}
				places[key] = p
				if key[1] != "" {
					deviceIDs = append(deviceIDs, key[1])
				}
			}
			period := periods.at(part.Day)
			if listening {
				p.listening += part.Seconds
				if period != nil {
					period.listening += part.Seconds
				}
			} else {
				p.reading += part.Seconds
				if period != nil {
					period.reading += part.Seconds
				}
			}
		}
	}

	names := map[string]string{}
	if len(deviceIDs) > 0 {
		var devices []struct {
			ID   string `bun:"id"`
			Name string `bun:"name"`
		}
		err := svc.db.NewSelect().
			TableExpr("devices AS d").
			ColumnExpr("d.id, d.name").
			Where("d.user_id = ? AND d.id IN (?)", userID, bun.List(deviceIDs)).
			Scan(ctx, &devices)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, d := range devices {
			names[d.ID] = d.Name
		}
	}

	// A Device's Sessions keep its id only while it exists, so its name is
	// found; any that isn't joins its reader's unnamed row.
	merged := map[[2]string]*place{}
	var order []*place
	for _, p := range places {
		key := [2]string{p.source, ""}
		if _, ok := names[p.deviceID]; ok {
			key[1] = p.deviceID
		}
		m := merged[key]
		if m == nil {
			m = &place{source: key[0], deviceID: key[1]}
			merged[key] = m
			order = append(order, m)
		}
		m.reading += p.reading
		m.listening += p.listening
	}
	for _, p := range order {
		if roundSeconds(p.reading)+roundSeconds(p.listening) == 0 {
			// A Session that recorded no time is no reading there.
			continue
		}
		row := ReadingStatsPlace{
			Source:           p.source,
			ReadingSeconds:   roundSeconds(p.reading),
			ListeningSeconds: roundSeconds(p.listening),
		}
		if name, ok := names[p.deviceID]; ok {
			row.DeviceName = &name
		}
		resp.Places = append(resp.Places, row)
	}
	slices.SortFunc(resp.Places, func(a, b ReadingStatsPlace) int {
		return cmp.Or(
			cmp.Compare(b.ReadingSeconds+b.ListeningSeconds, a.ReadingSeconds+a.ListeningSeconds),
			cmp.Compare(a.Source, b.Source),
			cmp.Compare(ptrString(a.DeviceName), ptrString(b.DeviceName)),
		)
	})
	return nil
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// rereadIDs are the finished Read-throughs of a Book the user had already
// finished: every one after the first, in the order they were read. A
// detached Read-through keeps no link to the other reads of its deleted
// Book, so it is never counted as a reread.
func rereadIDs(readThroughs []*models.ReadThrough) map[int]bool {
	byBook := map[int][]*models.ReadThrough{}
	for _, rt := range readThroughs {
		if rt.BookID == nil || rt.Outcome != models.ReadThroughOutcomeFinished {
			continue
		}
		byBook[*rt.BookID] = append(byBook[*rt.BookID], rt)
	}
	rereads := map[int]bool{}
	for _, rts := range byBook {
		slices.SortStableFunc(rts, func(a, b *models.ReadThrough) int {
			return cmp.Or(cmp.Compare(readOrderDay(a), readOrderDay(b)), cmp.Compare(a.ID, b.ID))
		})
		for _, rt := range rts[1:] {
			rereads[rt.ID] = true
		}
	}
	return rereads
}

// readOrderDay is the day that places a Read-through among the others: its
// end day, else its begin day, else when it was recorded.
func readOrderDay(rt *models.ReadThrough) string {
	if rt.EndDay != nil {
		return *rt.EndDay
	}
	if rt.BeginDay != nil {
		return *rt.BeginDay
	}
	return rt.CreatedAt.UTC().Format(time.DateOnly)
}

// daysTaken counts rt's begin day to its end day, both included, or nil
// without both.
func daysTaken(rt *models.ReadThrough) *int {
	if rt.BeginDay == nil || rt.EndDay == nil {
		return nil
	}
	begin, err1 := time.Parse(time.DateOnly, *rt.BeginDay)
	end, err2 := time.Parse(time.DateOnly, *rt.EndDay)
	if err1 != nil || err2 != nil || end.Before(begin) {
		return nil
	}
	days := int(end.Sub(begin).Hours()/24) + 1
	return &days
}

func median(values []int) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	m := float64(sorted[mid])
	if len(sorted)%2 == 0 {
		m = float64(sorted[mid-1]+sorted[mid]) / 2
	}
	return &m
}

func roundSeconds(seconds float64) int {
	return int(math.Round(seconds))
}

// StatsFinishedOptions picks the page of finished Read-throughs
// StatsFinished returns; see ListReadingStatsFinishedQuery for the filters.
// It mirrors that query field for field, so a handler converts one to it.
type StatsFinishedOptions struct {
	Year   *int
	Genre  *string
	Tag    *string
	Author *string
	Kind   *string
	Rating *float64
	Limit  int
	Offset int
}

// StatsFinished returns a page of user's finished Read-throughs that ended
// in the year, or for all time, and match the filters, latest first, and
// how many there are.
func (svc *Service) StatsFinished(ctx context.Context, user *models.User, opts StatsFinishedOptions) (*ListReadingStatsFinishedResponse, error) {
	resp := &ListReadingStatsFinishedResponse{Items: []ReadingStatsFinishedBook{}}
	var finished []*models.ReadThrough
	q := svc.db.NewSelect().
		Model(&finished).
		Where("rt.user_id = ? AND rt.outcome = ?", user.ID, models.ReadThroughOutcomeFinished)
	if rng := yearRange(opts.Year); !rng.all() {
		q = q.Where("rt.end_day >= ? AND rt.end_day <= ?", rng.From, rng.To)
	}
	// Names match as the breakdowns group them, ignoring case.
	if opts.Genre != nil {
		q = q.Where(`EXISTS (SELECT 1 FROM book_genres AS fbg JOIN genres AS fg ON fg.id = fbg.genre_id
			WHERE fbg.book_id = rt.book_id AND fg.name = ? COLLATE NOCASE)`, *opts.Genre)
	}
	if opts.Tag != nil {
		q = q.Where(`EXISTS (SELECT 1 FROM book_tags AS fbt JOIN tags AS ft ON ft.id = fbt.tag_id
			WHERE fbt.book_id = rt.book_id AND ft.name = ? COLLATE NOCASE)`, *opts.Tag)
	}
	if opts.Author != nil {
		q = q.Where(`EXISTS (SELECT 1 FROM authors AS fa JOIN persons AS fp ON fp.id = fa.person_id
			WHERE fa.book_id = rt.book_id AND fp.name = ? COLLATE NOCASE)`, *opts.Author)
	}
	if opts.Rating != nil {
		q = q.Where(`EXISTS (SELECT 1 FROM user_books AS fub
			WHERE fub.user_id = rt.user_id AND fub.book_id = rt.book_id AND fub.rating = ?)`,
			ratingUnitsOrZero(*opts.Rating))
	}
	if opts.Kind != nil {
		// A kind comes from Sessions and Files, not a column, so work out
		// which of the matches so far were read as it, then page those.
		var candidates []*models.ReadThrough
		if err := q.Clone().Model(&candidates).Column("rt.id", "rt.book_id").Scan(ctx); err != nil {
			return nil, errors.WithStack(err)
		}
		kinds, err := svc.readThroughKinds(ctx, candidates)
		if err != nil {
			return nil, err
		}
		ids := []int{}
		for _, rt := range candidates {
			if kinds[rt.ID][*opts.Kind] {
				ids = append(ids, rt.ID)
			}
		}
		if len(ids) == 0 {
			return resp, nil
		}
		q = q.Where("rt.id IN (?)", bun.List(ids))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp.Total = int(total)
	err = q.
		OrderExpr("rt.end_day IS NULL, rt.end_day DESC, rt.begin_day IS NULL, rt.begin_day DESC, rt.id DESC").
		Limit(int64(opts.Limit)).
		Offset(int64(opts.Offset)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(finished) == 0 {
		return resp, nil
	}

	entries, err := svc.BuildHistoryEntries(ctx, user, finished)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(finished))
	for _, rt := range finished {
		ids = append(ids, rt.ID)
	}
	var totals []struct {
		ReadThroughID int `bun:"read_through_id"`
		Seconds       int `bun:"seconds"`
	}
	err = svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("rs.read_through_id, SUM(rs.duration) AS seconds").
		Where("rs.read_through_id IN (?)", bun.List(ids)).
		GroupExpr("rs.read_through_id").
		Scan(ctx, &totals)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	seconds := make(map[int]int, len(totals))
	for _, row := range totals {
		seconds[row.ReadThroughID] = row.Seconds
	}
	ratings, err := svc.currentRatings(ctx, user.ID, finished)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		row := ReadingStatsFinishedBook{
			ReadThroughEntry: entry.ReadThroughEntry,
			DaysTaken:        daysTaken(entry.ReadThrough),
		}
		if s, ok := seconds[entry.ReadThrough.ID]; ok {
			row.Seconds = &s
		}
		if bookID := entry.ReadThrough.BookID; bookID != nil {
			if stars, ok := ratings[*bookID]; ok {
				row.Rating = &stars
			}
		}
		resp.Items = append(resp.Items, row)
	}
	return resp, nil
}

// addBreakdowns counts the distinct Books behind the finished Read-throughs
// by Genre, Tag, author, kind, and Rating. A detached Read-through has no
// Book to describe, so it's left out.
func (svc *Service) addBreakdowns(ctx context.Context, resp *ReadingStatsResponse, userID int, finished []*models.ReadThrough) error {
	var bookIDs []int
	for _, rt := range finished {
		if rt.BookID != nil && !slices.Contains(bookIDs, *rt.BookID) {
			bookIDs = append(bookIDs, *rt.BookID)
		}
	}
	resp.Genres = newBreakdown(nil)
	resp.Tags = newBreakdown(nil)
	resp.Authors = newBreakdown(nil)
	if len(bookIDs) == 0 {
		return nil
	}

	genres, err := svc.bookNames(ctx, "book_genres", "genres", "genre_id", bookIDs)
	if err != nil {
		return err
	}
	resp.Genres = newBreakdown(genres)
	tags, err := svc.bookNames(ctx, "book_tags", "tags", "tag_id", bookIDs)
	if err != nil {
		return err
	}
	resp.Tags = newBreakdown(tags)
	authors, err := authorNames(ctx, svc.db, bookIDs)
	if err != nil {
		return err
	}
	var authorRows []bookName
	for bookID, names := range authors {
		for _, name := range names {
			authorRows = append(authorRows, bookName{BookID: bookID, Name: name})
		}
	}
	resp.Authors = newBreakdown(authorRows)

	if resp.Kinds, err = svc.kindCounts(ctx, finished); err != nil {
		return err
	}
	resp.Ratings, resp.Totals.AverageRating, err = svc.ratingCounts(ctx, userID, bookIDs)
	return err
}

type bookName struct {
	BookID int    `bun:"book_id"`
	Name   string `bun:"name"`
}

// bookNames lists the names linked to bookIDs through a join table, such as
// each Book's Genres.
func (svc *Service) bookNames(ctx context.Context, joinTable, nameTable, nameColumn string, bookIDs []int) ([]bookName, error) {
	var rows []bookName
	err := svc.db.NewSelect().
		TableExpr("? AS j", bun.Ident(joinTable)).
		ColumnExpr("j.book_id, n.name").
		Join("JOIN ? AS n ON n.id = j.?", bun.Ident(nameTable), bun.Ident(nameColumn)).
		Where("j.book_id IN (?)", bun.List(bookIDs)).
		Scan(ctx, &rows)
	return rows, errors.WithStack(err)
}

// newBreakdown counts the distinct Books under each name, treating names
// that differ only in case as one, since each library keeps its own.
func newBreakdown(rows []bookName) []ReadingStatsCount {
	type group struct {
		name  string
		books map[int]bool
	}
	groups := map[string]*group{}
	for _, row := range rows {
		key := nocaseKey(row.Name)
		g := groups[key]
		if g == nil {
			g = &group{name: row.Name, books: map[int]bool{}}
			groups[key] = g
		}
		g.books[row.BookID] = true
	}
	sorted := make([]*group, 0, len(groups))
	for _, g := range groups {
		sorted = append(sorted, g)
	}
	slices.SortFunc(sorted, func(a, b *group) int {
		return cmp.Or(
			cmp.Compare(len(b.books), len(a.books)),
			cmp.Compare(nocaseKey(a.name), nocaseKey(b.name)),
			strings.Compare(a.name, b.name),
		)
	})

	counts := make([]ReadingStatsCount, 0, len(sorted))
	for _, g := range sorted {
		counts = append(counts, ReadingStatsCount{Name: g.name, Books: len(g.books)})
	}
	return counts
}

// nocaseKey folds name as SQLite's NOCASE collation does, ASCII letters
// only, so a breakdown groups names exactly as filtering the finished
// Read-throughs by one matches them.
func nocaseKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, name)
}

// statsKindOrder is the order kinds with the same count are listed in.
var statsKindOrder = []string{ReadingStatsKindEbook, ReadingStatsKindComic, ReadingStatsKindAudiobook, ReadingStatsKindUnknown}

// statsKind is the kind of reading done in a File of fileType.
func statsKind(fileType string) string {
	switch {
	case models.IsAudiobookFileType(fileType):
		return ReadingStatsKindAudiobook
	case fileType == models.FileTypeCBZ:
		return ReadingStatsKindComic
	default:
		return ReadingStatsKindEbook
	}
}

// kindCounts counts the distinct Books read as each kind. A Book read as
// two kinds counts under each.
func (svc *Service) kindCounts(ctx context.Context, finished []*models.ReadThrough) ([]ReadingStatsKindCount, error) {
	kinds, err := svc.readThroughKinds(ctx, finished)
	if err != nil {
		return nil, err
	}
	books := map[string]map[int]bool{}
	for _, rt := range finished {
		if rt.BookID == nil {
			continue
		}
		for kind := range kinds[rt.ID] {
			if books[kind] == nil {
				books[kind] = map[int]bool{}
			}
			books[kind][*rt.BookID] = true
		}
	}
	counts := []ReadingStatsKindCount{}
	for _, kind := range statsKindOrder {
		if n := len(books[kind]); n > 0 {
			counts = append(counts, ReadingStatsKindCount{Kind: kind, Books: n})
		}
	}
	slices.SortStableFunc(counts, func(a, b ReadingStatsKindCount) int { return cmp.Compare(b.Books, a.Books) })
	return counts, nil
}

// ratingUnitsOrZero is stars in quarter-star units, or 0, which matches no
// Rating, for a value the handler should already have refused.
func ratingUnitsOrZero(stars float64) int {
	units, _ := RatingUnits(stars)
	return units
}

// currentRatings are userID's Ratings, in stars, of the Books of
// readThroughs, by Book id. Unrated Books are left out.
func (svc *Service) currentRatings(ctx context.Context, userID int, readThroughs []*models.ReadThrough) (map[int]float64, error) {
	ratings := map[int]float64{}
	var bookIDs []int
	for _, rt := range readThroughs {
		if rt.BookID != nil {
			bookIDs = append(bookIDs, *rt.BookID)
		}
	}
	if len(bookIDs) == 0 {
		return ratings, nil
	}
	var rows []struct {
		BookID int `bun:"book_id"`
		Rating int `bun:"rating"`
	}
	err := svc.db.NewSelect().
		Model((*models.UserBook)(nil)).
		Column("ub.book_id", "ub.rating").
		Where("ub.user_id = ? AND ub.book_id IN (?) AND ub.rating IS NOT NULL", userID, bun.List(bookIDs)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		ratings[row.BookID] = *RatingStars(&row.Rating)
	}
	return ratings, nil
}

// readThroughKinds are the kinds each of the finished Read-throughs that
// still has its Book was read as, by Read-through id. They come from the
// Files its Sessions were in; one with no Session that says falls back to
// its Book's main Files, and "unknown" when those don't say either.
func (svc *Service) readThroughKinds(ctx context.Context, finished []*models.ReadThrough) (map[int]map[string]bool, error) {
	var ids, bookIDs []int
	seenBooks := map[int]bool{}
	for _, rt := range finished {
		if rt.BookID != nil {
			ids = append(ids, rt.ID)
			if !seenBooks[*rt.BookID] {
				seenBooks[*rt.BookID] = true
				bookIDs = append(bookIDs, *rt.BookID)
			}
		}
	}
	result := map[int]map[string]bool{}
	if len(ids) == 0 {
		return result, nil
	}
	var sessionRows []struct {
		ReadThroughID int     `bun:"read_through_id"`
		Kind          string  `bun:"kind"`
		FileType      *string `bun:"file_type"`
	}
	err := svc.db.NewSelect().
		TableExpr("reading_sessions AS rs").
		ColumnExpr("DISTINCT rs.read_through_id, rs.kind, f.file_type").
		Join("LEFT JOIN files AS f ON f.id = rs.file_id").
		Where("rs.read_through_id IN (?)", bun.List(ids)).
		Scan(ctx, &sessionRows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sessionKinds := map[int]map[string]bool{}
	for _, row := range sessionRows {
		kind := ""
		switch {
		case row.FileType != nil:
			kind = statsKind(*row.FileType)
		case row.Kind == models.ReadingKindListening:
			kind = ReadingStatsKindAudiobook
		}
		if kind == "" {
			continue
		}
		if sessionKinds[row.ReadThroughID] == nil {
			sessionKinds[row.ReadThroughID] = map[string]bool{}
		}
		sessionKinds[row.ReadThroughID][kind] = true
	}

	var fileRows []struct {
		BookID   int    `bun:"book_id"`
		FileType string `bun:"file_type"`
	}
	err = svc.db.NewSelect().
		TableExpr("files AS f").
		ColumnExpr("f.book_id, f.file_type").
		Where("f.book_id IN (?) AND f.file_role = ?", bun.List(bookIDs), models.FileRoleMain).
		Scan(ctx, &fileRows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bookKinds := map[int]map[string]bool{}
	for _, row := range fileRows {
		if bookKinds[row.BookID] == nil {
			bookKinds[row.BookID] = map[string]bool{}
		}
		bookKinds[row.BookID][statsKind(row.FileType)] = true
	}

	for _, rt := range finished {
		if rt.BookID == nil {
			continue
		}
		kinds := sessionKinds[rt.ID]
		if len(kinds) == 0 {
			kinds = bookKinds[*rt.BookID]
		}
		if len(kinds) == 0 {
			kinds = map[string]bool{ReadingStatsKindUnknown: true}
		}
		result[rt.ID] = kinds
	}
	return result, nil
}

// ratingCounts counts userID's current Ratings of bookIDs, each Book once,
// lowest first, and their mean in stars.
func (svc *Service) ratingCounts(ctx context.Context, userID int, bookIDs []int) ([]ReadingStatsRatingCount, *float64, error) {
	var units []int
	err := svc.db.NewSelect().
		Model((*models.UserBook)(nil)).
		Column("ub.rating").
		Where("ub.user_id = ? AND ub.book_id IN (?) AND ub.rating IS NOT NULL", userID, bun.List(bookIDs)).
		Scan(ctx, &units)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	counts := []ReadingStatsRatingCount{}
	if len(units) == 0 {
		return counts, nil, nil
	}
	byUnits := map[int]int{}
	sum := 0
	for _, u := range units {
		byUnits[u]++
		sum += u
	}
	for u := 1; u <= maxRatingUnits; u++ {
		if n := byUnits[u]; n > 0 {
			counts = append(counts, ReadingStatsRatingCount{Rating: *RatingStars(&u), Books: n})
		}
	}
	average := float64(sum) / float64(len(units)) / 4
	return counts, &average, nil
}
