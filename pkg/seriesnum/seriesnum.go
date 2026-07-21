// Package seriesnum parses and formats single series numbers and contiguous ranges.
package seriesnum

import (
	"math"
	"regexp"
	"strconv"
	"strings"
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
