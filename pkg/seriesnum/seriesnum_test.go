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

func TestValidateGroup(t *testing.T) {
	t.Parallel()

	volume := "volume"
	chapter := "chapter"
	issue := "issue"
	tests := []struct {
		name    string
		start   float64
		end     *float64
		unit    *string
		wantErr error
	}{
		{name: "single", start: 1},
		{name: "range", start: 1, end: float64Ptr(3)},
		{name: "volume unit", start: 1, unit: &volume},
		{name: "chapter unit", start: 1.5, end: float64Ptr(2), unit: &chapter},
		{name: "infinite start", start: math.Inf(1), wantErr: ErrStartNotFinite},
		{name: "nan start", start: math.NaN(), wantErr: ErrStartNotFinite},
		{name: "nan end", start: 1, end: float64Ptr(math.NaN()), wantErr: ErrEndNotFinite},
		{name: "infinite end", start: 1, end: float64Ptr(math.Inf(1)), wantErr: ErrEndNotFinite},
		{name: "equal end", start: 2, end: float64Ptr(2), wantErr: ErrEndNotAfterStart},
		{name: "end before start", start: 3, end: float64Ptr(2), wantErr: ErrEndNotAfterStart},
		{name: "unknown unit", start: 1, unit: &issue, wantErr: ErrUnknownUnit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, ValidateGroup(tt.start, tt.end, tt.unit), tt.wantErr)
		})
	}
}

func TestGroup(t *testing.T) {
	t.Parallel()

	volume := "volume"
	issue := "issue"

	start, end, unit := Group(float64Ptr(1), float64Ptr(3), &volume)
	require.NotNil(t, start)
	require.NotNil(t, end)
	require.NotNil(t, unit)
	assert.InDelta(t, 1, *start, 0.000001)
	assert.InDelta(t, 3, *end, 0.000001)
	assert.Equal(t, "volume", *unit)

	for name, group := range map[string][3]any{
		"missing start": {(*float64)(nil), float64Ptr(2), (*string)(nil)},
		"unit only":     {(*float64)(nil), (*float64)(nil), &volume},
		"bad unit":      {float64Ptr(1), (*float64)(nil), &issue},
		"reversed":      {float64Ptr(3), float64Ptr(1), (*string)(nil)},
	} {
		start, end, unit := Group(group[0].(*float64), group[1].(*float64), group[2].(*string))
		assert.Nil(t, start, name)
		assert.Nil(t, end, name)
		assert.Nil(t, unit, name)
		assert.False(t, ValidGroup(group[0].(*float64), group[1].(*float64), group[2].(*string)), name)
	}
}

func float64Ptr(v float64) *float64 { return &v }
