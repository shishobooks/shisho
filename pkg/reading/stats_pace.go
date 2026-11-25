package reading

import (
	"cmp"
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

// paceKind is the kind of length a Session covers. Comics and PDFs are both
// pages, but read at such different paces that each is judged on its own.
type paceKind int

const (
	paceWords paceKind = iota
	paceComicPages
	pacePDFPages
	paceAudio
	paceKinds
)

// paceSample is what one Session covered of its File and the time it took.
type paceSample struct {
	kind    paceKind
	covered float64
	seconds float64
	timed   bool
}

// sampleOf is s's sample, and false for a Session that didn't move forward
// or whose File's length is unknown, or gone.
func sampleOf(s statsSession) (paceSample, bool) {
	progress := s.EndPercentage - s.StartPercentage
	if progress <= 0 || s.FileType == nil {
		return paceSample{}, false
	}
	sample := paceSample{
		seconds: float64(max(s.Duration, 0)),
		// Measured time, or a Kobo's own count of reading minutes.
		// KOReader without the plugin guesses its time from when it synced.
		timed: s.Measured || s.Source == models.ReadingSourceKobo,
	}
	var length float64
	switch fileType := *s.FileType; {
	case models.IsReflowableFileType(fileType) && s.WordCount != nil && *s.WordCount > 0:
		sample.kind, length = paceWords, float64(*s.WordCount)
	case models.IsPageBasedFileType(fileType) && s.PageCount != nil && *s.PageCount > 0:
		sample.kind, length = paceComicPages, float64(*s.PageCount)
		if fileType == models.FileTypePDF {
			sample.kind = pacePDFPages
		}
		// Shisho's own page readers, on the web and in the app, save their
		// place as the 0-based page over the last page, so their percentage
		// spans one page fewer than the File has.
		if s.Source == models.ReadingSourceWebReader || s.Source == models.ReadingSourceShishoApp {
			length = max(length-1, 1)
		}
	case models.IsAudiobookFileType(fileType) && s.AudioSeconds != nil && *s.AudioSeconds > 0:
		sample.kind, length = paceAudio, *s.AudioSeconds
	default:
		return paceSample{}, false
	}
	sample.covered = progress * length
	return sample, true
}

// pacePoint is one timed Session's log pace, weighted by its seconds.
type pacePoint struct {
	logPace, seconds, covered float64
}

// weightedQuantile is the log pace at fraction q of points' total time,
// points sorted by pace.
func weightedQuantile(points []pacePoint, q float64) float64 {
	total := 0.0
	for _, p := range points {
		total += p.seconds
	}
	cum := 0.0
	for _, p := range points {
		cum += p.seconds
		if cum >= total*q {
			return p.logPace
		}
	}
	return points[len(points)-1].logPace
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
	var points [paceKinds][]pacePoint
	for _, s := range sessions {
		sample, ok := sampleOf(s)
		if !ok || !sample.timed || sample.seconds <= 0 {
			continue
		}
		points[sample.kind] = append(points[sample.kind], pacePoint{
			logPace: math.Log(sample.covered / sample.seconds),
			seconds: sample.seconds,
			covered: sample.covered,
		})
	}
	var norms [paceKinds]paceNorm
	for kind := range norms {
		norms[kind] = fenceNorm(points[kind])
	}
	return norms
}

// fenceNorm is Tukey's outer fences on points, one kind's Sessions, and the
// average pace of those inside them; zero with too few to judge.
func fenceNorm(points []pacePoint) paceNorm {
	if len(points) < minNormSessions {
		return paceNorm{}
	}
	slices.SortFunc(points, func(a, b pacePoint) int { return cmp.Compare(a.logPace, b.logPace) })
	lowerQuartile, upperQuartile := weightedQuantile(points, 0.25), weightedQuantile(points, 0.75)
	spread := max(upperQuartile-lowerQuartile, minLogSpread)
	lower, upper := lowerQuartile-fenceSpreads*spread, upperQuartile+fenceSpreads*spread
	var covered, seconds float64
	for _, p := range points {
		if p.logPace >= lower && p.logPace <= upper {
			covered += p.covered
			seconds += p.seconds
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
// overall for a skip to be told apart (norm unknown). Splitting time by day
// can leave a whole ten minutes a hair short, which still counts.
func (t paceTally) rate(norm paceNorm, per float64) *float64 {
	if !norm.known() || t.seconds < minPaceSeconds-1e-6 {
		return nil
	}
	r := t.covered / t.seconds * per
	return &r
}

// paceOf works out the range's pace and words read from sessions, judging
// each against what's normal for the user, and counting the share of each
// Session's time that falls in rng.
func paceOf(sessions []statsSession, norms [paceKinds]paceNorm, rng dayRange, loc *time.Location) (ReadingStatsPace, int) {
	var tallies [paceKinds]paceTally
	wordsRead := 0.0
	for _, s := range sessions {
		sample, ok := sampleOf(s)
		if !ok || !rng.mayTouch(s.StartedAt, s.EndedAt) {
			continue
		}
		share := shareInRange(s, rng, loc)
		if share == 0 {
			continue
		}
		norm := norms[sample.kind]
		// A timed Session with no time that still moved jumped there. An
		// untimed one isn't judged: KOReader without the plugin times it
		// from a pull Shisho stamps to a push the device stamps, so a device
		// clock running behind could make real reading look like a skip.
		judged := norm.known() && sample.timed
		fast := judged && (sample.seconds == 0 || sample.covered/sample.seconds > norm.upper)
		slow := judged && sample.seconds > 0 && sample.covered/sample.seconds < norm.lower
		credited := sample.covered
		if fast {
			credited = sample.seconds * norm.average
		}
		if sample.kind == paceWords {
			wordsRead += credited * share
		}
		if sample.timed && sample.seconds > 0 && !fast && !slow {
			tallies[sample.kind].covered += sample.covered * share
			tallies[sample.kind].seconds += sample.seconds * share
		}
	}
	return ReadingStatsPace{
		WordsPerMinute:    tallies[paceWords].rate(norms[paceWords], 60),
		ComicPagesPerHour: tallies[paceComicPages].rate(norms[paceComicPages], 3600),
		PDFPagesPerHour:   tallies[pacePDFPages].rate(norms[pacePDFPages], 3600),
		ListeningSpeed:    tallies[paceAudio].rate(norms[paceAudio], 1),
	}, int(math.Round(wordsRead))
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
