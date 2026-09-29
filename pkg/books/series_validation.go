package books

import (
	"errors"

	"github.com/shishobooks/shisho/pkg/seriesnum"
)

var errSeriesNumberEndWithoutStart = errors.New("series number end requires a start")

// validateSeriesInputs normalizes and checks the series groups of a book
// update. A unit without a number is cleared, and an end equal to the start
// collapses to a single number, before seriesnum.ValidateGroup checks the rest.
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
		if input.NumberEnd != nil && *input.NumberEnd == *input.Number {
			input.NumberEnd = nil
		}
		if err := seriesnum.ValidateGroup(*input.Number, input.NumberEnd, input.SeriesNumberUnit); err != nil {
			return err
		}
	}
	return nil
}
