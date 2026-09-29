// Package seriesnum parses and formats single series numbers and contiguous ranges.
package seriesnum

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
)

// Errors returned by ValidateGroup. The book update handler shows them to the
// user as the reason a series number was rejected.
var (
	ErrStartNotFinite   = errors.New("series number must be finite")
	ErrEndNotFinite     = errors.New("series number end must be finite")
	ErrEndNotAfterStart = errors.New("series number end must not be less than the start")
	ErrUnknownUnit      = errors.New("series number unit must be volume or chapter")
)

var rangePattern = regexp.MustCompile(`^([+-]?(?:\d+(?:\.\d*)?|\.\d+))(?:\s*[-–—]\s*([+-]?(?:\d+(?:\.\d*)?|\.\d+)))?$`)

// ParseRange parses a single series number or a strictly increasing contiguous
// range separated by a hyphen, en dash, or em dash.
func ParseRange(s string) (start float64, end *float64, ok bool) {
	matches := rangePattern.FindStringSubmatch(strings.TrimSpace(s))
	if matches == nil {
		return 0, nil, false
	}

	start, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || !isFinite(start) {
		return 0, nil, false
	}
	if matches[2] == "" {
		return start, nil, true
	}

	endValue, err := strconv.ParseFloat(matches[2], 64)
	if err != nil || !isFinite(endValue) || endValue <= start {
		return 0, nil, false
	}
	return start, &endValue, true
}

// FormatRange formats whole endpoints as integers and other endpoints as
// decimals, joining a range with a hyphen.
func FormatRange(start float64, end *float64) string {
	formatted := formatNumber(start)
	if end != nil {
		formatted += "-" + formatNumber(*end)
	}
	return formatted
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// ValidateGroup checks a series number group (start, end, and unit) that has a
// start. The start must be finite, the end must be nil or a finite value
// greater than the start, and the unit must be nil, volume, or chapter.
func ValidateGroup(start float64, end *float64, unit *string) error {
	if !isFinite(start) {
		return ErrStartNotFinite
	}
	if end != nil {
		if !isFinite(*end) {
			return ErrEndNotFinite
		}
		if *end <= start {
			return ErrEndNotAfterStart
		}
	}
	if unit != nil && *unit != models.SeriesNumberUnitVolume && *unit != models.SeriesNumberUnitChapter {
		return ErrUnknownUnit
	}
	return nil
}

// ValidGroup reports whether the group has a start and ValidateGroup accepts
// it. Plugins, sidecars, and parsed file metadata use it to drop a malformed
// group whole rather than keep part of it.
func ValidGroup(start, end *float64, unit *string) bool {
	return start != nil && ValidateGroup(*start, end, unit) == nil
}

// Group returns the group unchanged when ValidGroup accepts it and three nils
// otherwise.
func Group(start, end *float64, unit *string) (*float64, *float64, *string) {
	if !ValidGroup(start, end, unit) {
		return nil, nil, nil
	}
	return start, end, unit
}
