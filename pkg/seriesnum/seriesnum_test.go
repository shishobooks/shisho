package seriesnum

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantStart float64
		wantEnd   *float64
		wantOK    bool
	}{
		{name: "integer", input: "1", wantStart: 1, wantOK: true},
		{name: "decimal", input: "1.5", wantStart: 1.5, wantOK: true},
		{name: "hyphen", input: "1-3", wantStart: 1, wantEnd: float64Ptr(3), wantOK: true},
		{name: "hyphen spaces", input: "1 - 3", wantStart: 1, wantEnd: float64Ptr(3), wantOK: true},
		{name: "en dash", input: "1–3", wantStart: 1, wantEnd: float64Ptr(3), wantOK: true},
		{name: "em dash spaces", input: "1 — 3", wantStart: 1, wantEnd: float64Ptr(3), wantOK: true},
		{name: "decimal range", input: "1.5-2.5", wantStart: 1.5, wantEnd: float64Ptr(2.5), wantOK: true},
		{name: "surrounding spaces", input: " 1-3 ", wantStart: 1, wantEnd: float64Ptr(3), wantOK: true},
		{name: "non contiguous", input: "1,2,3", wantOK: false},
		{name: "equal", input: "1-1", wantOK: false},
		{name: "reversed", input: "3-1", wantOK: false},
		{name: "missing end", input: "1-", wantOK: false},
		{name: "infinity", input: "Inf", wantOK: false},
		{name: "range infinity", input: "1-Inf", wantOK: false},
		{name: "nan", input: "NaN", wantOK: false},
		{name: "empty", input: "", wantOK: false},
		{name: "text", input: "first", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, end, ok := ParseRange(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.InDelta(t, tt.wantStart, start, 0.000001)
			if tt.wantEnd == nil {
				assert.Nil(t, end)
			} else {
				require.NotNil(t, end)
				assert.InDelta(t, *tt.wantEnd, *end, 0.000001)
			}
		})
	}
}

func TestFormatRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start float64
		end   *float64
		want  string
	}{
		{name: "integer", start: 1, want: "1"},
		{name: "decimal", start: 1.5, want: "1.5"},
		{name: "integer range", start: 1, end: float64Ptr(3), want: "1-3"},
		{name: "decimal range", start: 1.25, end: float64Ptr(3.5), want: "1.25-3.5"},
		{name: "large exact integer", start: 1000, end: float64Ptr(1002), want: "1000-1002"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FormatRange(tt.start, tt.end))
		})
	}
}

func TestParseRangeRejectsNonFiniteNumericValues(t *testing.T) {
	t.Parallel()

	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		assert.False(t, isFinite(value))
	}
}

func float64Ptr(v float64) *float64 { return &v }
