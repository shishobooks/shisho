package ereader

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
)

// formatLength is how long a File is, as the web app's facts line says it:
// an audiobook's duration, else its pages, else its words. Empty when the
// File has none of them.
func formatLength(f *models.File) string {
	switch {
	case f.AudiobookDurationSeconds != nil && *f.AudiobookDurationSeconds > 0:
		return formatDuration(*f.AudiobookDurationSeconds)
	case f.PageCount != nil && *f.PageCount > 0:
		return formatCount(*f.PageCount, "page")
	case f.WordCount != nil && *f.WordCount > 0:
		return formatWordCount(*f.WordCount)
	}
	return ""
}

// formatDuration is the web app's formatDuration: "6h 43m", "12m", or "<1m".
func formatDuration(seconds float64) string {
	if seconds < 60 {
		return "<1m"
	}
	total := int(seconds)
	hours, minutes := total/3600, total%3600/60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// formatWordCount is the web app's formatWordCount: exact under a thousand,
// then rounded to thousands, millions, or billions, to two significant
// digits below ten of a unit ("1.5K words", "29K words", "1.2M words").
func formatWordCount(count int) string {
	if count < 1000 {
		return formatCount(count, "word")
	}
	units := []struct {
		size float64
		// suffix is the unit's, and up the next unit's, which a count
		// that rounds to a thousand of this one becomes.
		suffix, up string
	}{{1e9, "B", ""}, {1e6, "M", "B"}, {1e3, "K", "M"}}
	for _, u := range units {
		if float64(count) < u.size {
			continue
		}
		scaled := float64(count) / u.size
		rounded := math.Round(scaled)
		if scaled < 10 {
			rounded = math.Round(scaled*10) / 10
		}
		// 999,999 rounds to 1000K, which is 1M.
		if rounded >= 1000 && u.up != "" {
			return "1" + u.up + " words"
		}
		return strconv.FormatFloat(rounded, 'f', -1, 64) + u.suffix + " words"
	}
	return formatCount(count, "word")
}

// formatCount is a count and its noun, thousands grouped: "1 page",
// "1,200 pages".
func formatCount(count int, singular string) string {
	noun := singular + "s"
	if count == 1 {
		noun = singular
	}
	digits := strconv.Itoa(count)
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return grouped.String() + " " + noun
}
