package mp4

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConvertChaptersToParsed(t *testing.T) {
	chapters := []Chapter{
		{Title: "Chapter 1", Start: 0, End: 5 * time.Minute},
		{Title: "Chapter 2", Start: 5 * time.Minute, End: 10 * time.Minute},
	}

	parsed := convertChaptersToParsed(chapters)

	assert.Len(t, parsed, 2)
	assert.Equal(t, "Chapter 1", parsed[0].Title)
	assert.NotNil(t, parsed[0].StartTimestampMs)
	assert.Equal(t, int64(0), *parsed[0].StartTimestampMs)
	assert.Nil(t, parsed[0].StartPage)
	assert.Nil(t, parsed[0].Href)
	assert.Empty(t, parsed[0].Children)

	assert.Equal(t, "Chapter 2", parsed[1].Title)
	assert.Equal(t, int64(300000), *parsed[1].StartTimestampMs) // 5 minutes in ms
}
