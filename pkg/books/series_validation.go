package books

import (
	"errors"
	"math"
)

var (
	errSeriesNumberEndWithoutStart = errors.New("series number end requires a start")
	errSeriesNumberNotFinite       = errors.New("series number must be finite")
	errSeriesNumberEndNotFinite    = errors.New("series number end must be finite")
	errSeriesNumberEndBeforeStart  = errors.New("series number end must not be less than the start")
)

func validateSeriesInputs(inputs []SeriesInput) error {
	for i := range inputs {
		input := &inputs[i]
		if input.Number == nil {
			if input.NumberEnd != nil {
				return errSeriesNumberEndWithoutStart
			}
			input.SeriesNumberUnit = nil
			continue
		}
		if !isFiniteSeriesNumber(*input.Number) {
			return errSeriesNumberNotFinite
		}
		if input.NumberEnd == nil {
			continue
		}
		if !isFiniteSeriesNumber(*input.NumberEnd) {
			return errSeriesNumberEndNotFinite
		}
		if *input.NumberEnd < *input.Number {
			return errSeriesNumberEndBeforeStart
		}
		if *input.NumberEnd == *input.Number {
			input.NumberEnd = nil
		}
	}
	return nil
}

func isFiniteSeriesNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
