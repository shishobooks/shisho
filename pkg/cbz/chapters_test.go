package cbz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChaptersFromFolders(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "chapters from immediate parent directories",
			files: []string{
				"Series Title/Chapter 1/page001.jpg",
				"Series Title/Chapter 1/page002.jpg",
				"Series Title/Chapter 2/page003.jpg",
				"Series Title/Chapter 2/page004.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "all files in same directory - no chapters",
			files: []string{
				"page001.jpg",
				"page002.jpg",
				"page003.jpg",
			},
			expected: nil,
		},
		{
			name: "single chapter folder",
			files: []string{
				"Chapter 1/page001.jpg",
				"Chapter 1/page002.jpg",
			},
			expected: nil, // Single folder = no chapters
		},
		{
			name: "deeply nested - uses immediate parent only",
			files: []string{
				"Volume 1/Arc 1/Chapter 1/page001.jpg",
				"Volume 1/Arc 1/Chapter 2/page002.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := detectChaptersFromFolders(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}

type expectedChapter struct {
	title     string
	startPage int
}

func TestDetectChapters(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "prefers folders over filenames",
			files: []string{
				"Chapter 1/ch01_page001.jpg",
				"Chapter 1/ch01_page002.jpg",
				"Chapter 2/ch02_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "falls back to filenames when single folder",
			files: []string{
				"Comics/ch01_page001.jpg",
				"Comics/ch01_page002.jpg",
				"Comics/ch02_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := DetectChapters(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}

func TestDetectChaptersFromFilenames(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "ch prefix pattern",
			files: []string{
				"page001_ch01.jpg",
				"page002_ch01.jpg",
				"page003_ch02.jpg",
				"page004_ch02.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "chapter prefix pattern",
			files: []string{
				"chapter1_page001.jpg",
				"chapter1_page002.jpg",
				"chapter2_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "no pattern found",
			files: []string{
				"page001.jpg",
				"page002.jpg",
				"page003.jpg",
			},
			expected: nil,
		},
		{
			name: "case insensitive",
			files: []string{
				"CH01_page001.jpg",
				"CH02_page002.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := detectChaptersFromFilenames(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}
