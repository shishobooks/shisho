package reading

import (
	"time"
)

// daySeconds is the part of a Reading Session's time that fell on one day,
// a calendar day (YYYY-MM-DD) in the user's time zone.
type daySeconds struct {
	Day     string
	Seconds float64
}

// splitByDay divides duration seconds of a Session that ran from start to
// end across the days of loc it touched, in proportion to how much of the
// span fell on each. Each day ends at its real local midnight, so a day
// that daylight saving makes 23 or 25 hours long gets its true share. A
// Session with no span lands whole on the day it started.
func splitByDay(start, end time.Time, duration int, loc *time.Location) []daySeconds {
	start, end = start.In(loc), end.In(loc)
	total := float64(duration)
	if !end.After(start) {
		return []daySeconds{{Day: start.Format(time.DateOnly), Seconds: total}}
	}
	span := end.Sub(start).Seconds()
	var out []daySeconds
	for cur := start; cur.Before(end); {
		y, m, d := cur.Date()
		next := time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		if !next.After(cur) {
			// A zone that skips midnight can normalize it backwards; step
			// past it rather than loop.
			next = cur.Add(time.Hour)
		}
		if next.After(end) {
			next = end
		}
		out = append(out, daySeconds{
			Day:     cur.Format(time.DateOnly),
			Seconds: total * next.Sub(cur).Seconds() / span,
		})
		cur = next
	}
	return out
}

// dayRange is a span of calendar days, both ends included, or every day
// when it is unbounded (all time).
type dayRange struct {
	From, To string
}

// yearRange is the days of year, or every day when year is nil.
func yearRange(year *int) dayRange {
	if year == nil {
		return dayRange{}
	}
	from := time.Date(*year, time.January, 1, 0, 0, 0, 0, time.UTC)
	return dayRange{
		From: from.Format(time.DateOnly),
		To:   from.AddDate(1, 0, -1).Format(time.DateOnly),
	}
}

func (r dayRange) all() bool { return r.From == "" }

// contains reports whether day falls in r. An unset day is in only the
// unbounded range.
func (r dayRange) contains(day *string) bool {
	if r.all() {
		return true
	}
	return day != nil && *day >= r.From && *day <= r.To
}

// instants are the UTC instants that bound loading the Sessions that may
// touch r in any time zone: a day's margin on each side, since no zone is a
// day from UTC. Callers trim to r after grouping by day. Both are nil for
// the unbounded range.
func (r dayRange) instants() (*time.Time, *time.Time) {
	if r.all() {
		return nil, nil
	}
	from, _ := time.Parse(time.DateOnly, r.From)
	to, _ := time.Parse(time.DateOnly, r.To)
	from = from.AddDate(0, 0, -1)
	to = to.AddDate(0, 0, 2)
	return &from, &to
}
