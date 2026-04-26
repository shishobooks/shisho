package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeriesNumberUnitConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "volume", SeriesNumberUnitVolume)
	assert.Equal(t, "chapter", SeriesNumberUnitChapter)
}
