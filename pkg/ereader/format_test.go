package ereader

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestFormatWordCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		count int
		want  string
	}{
		{1, "1 word"},
		{850, "850 words"},
		{999, "999 words"},
		{1000, "1K words"},
		{1500, "1.5K words"},
		{9960, "10K words"},
		{29327, "29K words"},
		{139549, "140K words"},
		{999999, "1M words"},
		{1234567, "1.2M words"},
		{2500000000, "2.5B words"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatWordCount(tt.count), "%d", tt.count)
	}
}

func TestFormatLength(t *testing.T) {
	t.Parallel()

	duration := 24180.0
	short := 30.0
	pages := 1200
	page := 1
	words := 95312
	tests := []struct {
		name string
		file models.File
		want string
	}{
		{"an audiobook's duration", models.File{AudiobookDurationSeconds: &duration}, "6h 43m"},
		{"under a minute", models.File{AudiobookDurationSeconds: &short}, "<1m"},
		{"pages", models.File{PageCount: &pages}, "1,200 pages"},
		{"one page", models.File{PageCount: &page}, "1 page"},
		{"words", models.File{WordCount: &words}, "95K words"},
		{"nothing to say", models.File{}, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatLength(&tt.file), tt.name)
	}
}
