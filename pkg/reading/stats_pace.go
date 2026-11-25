package reading

import (
	"math"
	"slices"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// What counts toward ReadingStatsPace and ReadingStatsTotals.WordsRead.
//
// A skip ahead, such as a jump through the contents, would count a stretch
// of a book as read in a few seconds. Rather than guess how fast people
// read, each user's own timed Sessions say what's normal for them: the limit
// for a kind is Tukey's outer fence on the log of their paces, the upper
// quartile plus fenceSpreads times the distance between quartiles, with
// each Session weighted by its time. Anchoring at the upper quartile lets a
// reader whose books differ in pace keep the faster ones. A Session faster
// than the limit is credited as if read at the limit for its time.
const (
	fenceSpreads = 3
	// minLimitSessions is how many timed Sessions of a kind it takes to
	// judge what's normal; with fewer, every Session counts in full.
	minLimitSessions = 5
	// minPaceSeconds is how much timed time a kind needs in the range
	// before its pace means anything.
	minPaceSeconds = 10 * 60
)

// minLogSpread floors the distance between quartiles at 10%, so a user
// whose Sessions nearly all ran at one pace doesn't get a limit at that pace.
var minLogSpread = math.Log(1.1)

// paceKind is the kind of length a Session covers.
type paceKind int

const (
	paceWords paceKind = iota
	pacePages
	paceAudio
	paceKinds
)

// sessionLength is the kind and length of s's File: its words, pages, or
// seconds of audio. ok is false for a File of unknown length, or gone.
func sessionLength(s statsSession) (kind paceKind, length float64, ok bool) {
	if s.FileType == nil {
		return 0, 0, false
	}
	switch {
	case models.IsReflowableFileType(*s.FileType) && s.WordCount != nil && *s.WordCount > 0:
		return paceWords, float64(*s.WordCount), true
	case models.IsPageBasedFileType(*s.FileType) && s.PageCount != nil && *s.PageCount > 0:
		return pacePages, float64(*s.PageCount), true
	case models.IsAudiobookFileType(*s.FileType) && s.AudioSeconds != nil && *s.AudioSeconds > 0:
		return paceAudio, *s.AudioSeconds, true
	}
	return 0, 0, false
}

// isTimed reports whether s's time is what it took: measured, or a Kobo's
// own count of reading minutes. KOReader without the plugin guesses its time
// from when it synced.
func isTimed(s statsSession) bool {
	return s.Measured || s.Source == models.ReadingSourceKobo
}

// weighted is a value and how much it counts.
type weighted struct {
	value, weight float64
}

// weightedQuantile is the value at fraction q of points' total weight,
// points sorted by value.
func weightedQuantile(points []weighted, q float64) float64 {
	total := 0.0
	for _, p := range points {
		total += p.weight
	}
	cum := 0.0
	for _, p := range points {
		cum += p.weight
		if cum >= total*q {
			return p.value
		}
	}
	return points[len(points)-1].value
}

// paceLimits are the fastest pace that's normal for the user in each kind,
// in length per second, from sessions, which should be all of theirs. Zero
// where too few timed Sessions say.
func paceLimits(sessions []statsSession) [paceKinds]float64 {
	var points [paceKinds][]weighted
	for _, s := range sessions {
		progress := s.EndPercentage - s.StartPercentage
		kind, length, ok := sessionLength(s)
		if !ok || !isTimed(s) || progress <= 0 || s.Duration <= 0 {
			continue
		}
		seconds := float64(s.Duration)
		points[kind] = append(points[kind], weighted{value: math.Log(progress * length / seconds), weight: seconds})
	}
	var limits [paceKinds]float64
	for kind := range limits {
		limits[kind] = fenceLimit(points[kind])
	}
	return limits
}

// fenceLimit is Tukey's outer fence on points, the logs of one kind's paces,
// back as a pace; zero with too few to judge.
func fenceLimit(points []weighted) float64 {
	if len(points) < minLimitSessions {
		return 0
	}
	slices.SortFunc(points, func(a, b weighted) int {
		switch {
		case a.value < b.value:
			return -1
		case a.value > b.value:
			return 1
		}
		return 0
	})
	upper := weightedQuantile(points, 0.75)
	spread := max(upper-weightedQuantile(points, 0.25), minLogSpread)
	return math.Exp(upper + fenceSpreads*spread)
}

// paceTally sums one kind's length covered and the time it took.
type paceTally struct {
	covered, seconds float64
}

func (t paceTally) rate(per float64) *float64 {
	if t.seconds < minPaceSeconds {
		return nil
	}
	r := t.covered / t.seconds * per
	return &r
}

// paceOf works out the range's pace and words read from the range's
// sessions, crediting each timed one up to the user's limits, and counting
// the share of each Session's time that falls in rng.
func paceOf(sessions []statsSession, limits [paceKinds]float64, rng dayRange, loc *time.Location) (ReadingStatsPace, int) {
	var tallies [paceKinds]paceTally
	wordsRead := 0.0
	for _, s := range sessions {
		progress := s.EndPercentage - s.StartPercentage
		kind, length, ok := sessionLength(s)
		if !ok || progress <= 0 {
			continue
		}
		covered := progress * length
		timed := isTimed(s)
		seconds := float64(s.Duration)
		if limit := limits[kind]; timed && limit > 0 {
			covered = min(covered, seconds*limit)
		}
		share := shareInRange(s, rng, loc)
		if share == 0 {
			continue
		}
		if kind == paceWords {
			wordsRead += covered * share
		}
		if timed && seconds > 0 {
			tallies[kind].covered += covered * share
			tallies[kind].seconds += seconds * share
		}
	}
	return ReadingStatsPace{
		WordsPerMinute: tallies[paceWords].rate(60),
		PagesPerHour:   tallies[pacePages].rate(3600),
		ListeningSpeed: tallies[paceAudio].rate(1),
	}, roundSeconds(wordsRead)
}

// shareInRange is the fraction of s's time that falls on days in rng, as
// reading time is split across days.
func shareInRange(s statsSession, rng dayRange, loc *time.Location) float64 {
	parts := splitByDay(s.StartedAt, s.EndedAt, s.Duration, loc)
	var in, total float64
	for _, part := range parts {
		total += part.Seconds
		if rng.contains(&part.Day) {
			in += part.Seconds
		}
	}
	if total == 0 {
		// No time to split: the Session counts where it ended.
		end := s.EndedAt.In(loc).Format(time.DateOnly)
		if rng.contains(&end) {
			return 1
		}
		return 0
	}
	return in / total
}
