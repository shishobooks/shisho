package books

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSeriesInputs(t *testing.T) {
	t.Parallel()

	unit := "volume"
	tests := []struct {
		name        string
		input       SeriesInput
		wantErr     bool
		wantEndNil  bool
		wantUnitNil bool
	}{
		{name: "single number", input: SeriesInput{Name: "Series", Number: float64Pointer(1)}, wantEndNil: true},
		{name: "range", input: SeriesInput{Name: "Series", Number: float64Pointer(1), NumberEnd: float64Pointer(3)}},
		{name: "equal normalizes to single", input: SeriesInput{Name: "Series", Number: float64Pointer(2), NumberEnd: float64Pointer(2)}, wantEndNil: true},
		{name: "end before start", input: SeriesInput{Name: "Series", Number: float64Pointer(3), NumberEnd: float64Pointer(2)}, wantErr: true},
		{name: "end without start", input: SeriesInput{Name: "Series", NumberEnd: float64Pointer(2)}, wantErr: true},
		{name: "non finite start", input: SeriesInput{Name: "Series", Number: float64Pointer(math.Inf(1))}, wantErr: true},
		{name: "non finite end", input: SeriesInput{Name: "Series", Number: float64Pointer(1), NumberEnd: float64Pointer(math.NaN())}, wantErr: true},
		{name: "unit without start clears group", input: SeriesInput{Name: "Series", SeriesNumberUnit: &unit}, wantEndNil: true, wantUnitNil: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inputs := []SeriesInput{tt.input}
			err := validateSeriesInputs(inputs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantEndNil {
				assert.Nil(t, inputs[0].NumberEnd)
			}
			if tt.wantUnitNil {
				assert.Nil(t, inputs[0].SeriesNumberUnit)
			}
		})
	}
}

func float64Pointer(value float64) *float64 { return &value }
