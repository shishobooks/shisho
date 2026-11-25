package reading

import (
	"math"
	"slices"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// What counts toward ReadingStatsPace and ReadingStatsTotals.WordsRead.
//
// A skip ahead, such as a jump through the contents, counts a stretch of a
// book as read in seconds, and a book left open while away counts time with
// no reading. Rather than guess how fast people read, each user's own timed
// Sessions say what's normal for them: Tukey's outer fences on the log of
// their paces, the quartiles widened by fenceSpreads times the distance
// between them, with each Session weighted by its time. A Session outside
// the fences is left out of Pace. In Words read, a skip's time counts at
// the user's average pace over the Sessions inside the fences, and a slow
// Session counts what it covered, which was real progress.
const (
	fenceSpreads = 3
	// minNormSessions is how many timed Sessions of a kind it takes to
	// judge what's normal. With fewer, every Session counts in full toward
	// Words read, and the kind shows no pace, since a skip would skew it.
	minNormSessions = 5
	// minPaceSeconds is how much timed time a kind needs in the range
	// before its pace means anything.
	minPaceSeconds = 10 * 60
)

// minLogSpread floors the distance between quartiles at 10%, so a user
// whose Sessions nearly all ran at one pace doesn't get fences at that pace.
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

// weighted is a Session's log pace, weighted by its seconds, and what it
// covered.
type weighted struct {
	value, weight, covered float64
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

// paceNorm is what's normal for the user in one kind, in length per second:
// the slowest and fastest normal paces, and the average pace of the Sessions
// between them. The zero value means too few timed Sessions to say.
type paceNorm struct {
	lower, upper, average float64
}

// known reports whether there were enough timed Sessions to say.
func (n paceNorm) known() bool { return n.upper > 0 }

// paceNorms are what's normal for the user in each kind, from sessions,
// which should be all of theirs.
func paceNorms(sessions []statsSession) [paceKinds]paceNorm {
	var points [paceKinds][]weighted
	for _, s := range sessions {
		progress := s.EndPercentage - s.StartPercentage
		kind, length, ok := sessionLength(s)
		if !ok || !isTimed(s) || progress <= 0 || s.Duration <= 0 {
			continue
		}
		seconds := float64(s.Duration)
		covered := progress * length
		points[kind] = append(points[kind], weighted{value: math.Log(covered / seconds), weight: seconds, covered: covered})
	}
	var norms [paceKinds]paceNorm
	for kind := range norms {
		norms[kind] = fenceNorm(points[kind])
	}
	return norms
}

// fenceNorm is Tukey's outer fences on points, one kind's Sessions, and the
// average pace of those inside them; zero with too few to judge.
func fenceNorm(points []weighted) paceNorm {
	if len(points) < minNormSessions {
		return paceNorm{}
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
	lowerQuartile, upperQuartile := weightedQuantile(points, 0.25), weightedQuantile(points, 0.75)
	spread := max(upperQuartile-lowerQuartile, minLogSpread)
	lower, upper := lowerQuartile-fenceSpreads*spread, upperQuartile+fenceSpreads*spread
	var covered, seconds float64
	for _, p := range points {
		if p.value >= lower && p.value <= upper {
			covered += p.covered
			seconds += p.weight
		}
	}
	return paceNorm{lower: math.Exp(lower), upper: math.Exp(upper), average: covered / seconds}
}

// paceTally sums one kind's length covered and the time it took.
type paceTally struct {
	covered, seconds float64
}

// rate is the tally's pace over each span of the given seconds, or nil
// without enough to trust: too little time in the range, or too few Sessions
// overall for a skip to be told apart (norm unknown).
func (t paceTally) rate(norm paceNorm, per float64) *float64 {
	if !norm.known() || t.seconds < minPaceSeconds {
		return nil
	}
	r := t.covered / t.seconds * per
	return &r
}

// paceOf works out the range's pace and words read from the range's
// sessions, judging each timed one against what's normal for the user, and
// counting the share of each Session's time that falls in rng.
func paceOf(sessions []statsSession, norms [paceKinds]paceNorm, rng dayRange, loc *time.Location) (ReadingStatsPace, int) {
	var tallies [paceKinds]paceTally
	wordsRead := 0.0
	for _, s := range sessions {
		progress := s.EndPercentage - s.StartPercentage
		kind, length, ok := sessionLength(s)
		if !ok || progress <= 0 {
			continue
		}
		share := shareInRange(s, rng, loc)
		if share == 0 {
			continue
		}
		covered := progress * length
		seconds := float64(s.Duration)
		timed := isTimed(s)
		norm := norms[kind]
		// A Session with no time that still moved jumped there.
		fast := timed && norm.known() && (seconds == 0 || covered/seconds > norm.upper)
		slow := timed && norm.known() && seconds > 0 && covered/seconds < norm.lower
		credited := covered
		if fast {
			credited = seconds * norm.average
		}
		if kind == paceWords {
			wordsRead += credited * share
		}
		if timed && seconds > 0 && !fast && !slow {
			tallies[kind].covered += covered * share
			tallies[kind].seconds += seconds * share
		}
	}
	return ReadingStatsPace{
		WordsPerMinute: tallies[paceWords].rate(norms[paceWords], 60),
		PagesPerHour:   tallies[pacePages].rate(norms[pacePages], 3600),
		ListeningSpeed: tallies[paceAudio].rate(norms[paceAudio], 1),
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
