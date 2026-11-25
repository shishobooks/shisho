package reading

import (
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// Limits on the Sessions that count toward ReadingStatsPace. A Session
// faster than the fastest readers skipped ahead rather than read, and one
// shorter than a minute is too short to time.
const (
	maxWordsPerMinute  = 1500
	maxPagesPerHour    = 1200
	maxListeningSpeed  = 4.0
	minPaceSessionSecs = 60
	// minPaceSeconds is how much timed time a kind needs before its pace
	// means anything.
	minPaceSeconds = 10 * 60
)

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

// paceOf works out the range's pace and words read from sessions, counting
// the share of each Session's time that falls in rng.
func paceOf(sessions []statsSession, rng dayRange, loc *time.Location) (ReadingStatsPace, int) {
	var words, pages, audio paceTally
	wordsRead := 0.0
	for _, s := range sessions {
		progress := s.EndPercentage - s.StartPercentage
		if progress <= 0 || s.FileType == nil {
			continue
		}
		var length, limit float64
		var tally *paceTally
		switch {
		case models.IsReflowableFileType(*s.FileType) && s.WordCount != nil:
			length, limit, tally = float64(*s.WordCount), maxWordsPerMinute/60.0, &words
		case models.IsPageBasedFileType(*s.FileType) && s.PageCount != nil:
			length, limit, tally = float64(*s.PageCount), maxPagesPerHour/3600.0, &pages
		case models.IsAudiobookFileType(*s.FileType) && s.AudioSeconds != nil:
			length, limit, tally = *s.AudioSeconds, maxListeningSpeed, &audio
		default:
			continue
		}
		covered := progress * length
		timedSession := s.Measured || s.Source == models.ReadingSourceKobo
		seconds := float64(s.Duration)
		// A timed Session faster than anyone reads skipped ahead, so none
		// of it counts; an untimed one can't be told apart.
		if timedSession && seconds > 0 && covered/seconds > limit {
			continue
		}
		share := shareInRange(s, rng, loc)
		if share == 0 {
			continue
		}
		if tally == &words {
			wordsRead += covered * share
		}
		if timedSession && seconds >= minPaceSessionSecs {
			tally.covered += covered * share
			tally.seconds += seconds * share
		}
	}
	return ReadingStatsPace{
		WordsPerMinute: words.rate(60),
		PagesPerHour:   pages.rate(3600),
		ListeningSpeed: audio.rate(1),
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
