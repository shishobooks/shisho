package opds

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateFeedAuthors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		authors         []Author
		expectedAuthors []Author
	}{
		{
			name:            "single author unchanged",
			authors:         []Author{{Name: "Alice"}},
			expectedAuthors: []Author{{Name: "Alice"}},
		},
		{
			name:            "multiple authors keeps first only",
			authors:         []Author{{Name: "Alice"}, {Name: "Bob"}},
			expectedAuthors: []Author{{Name: "Alice"}},
		},
		{
			name:            "three authors keeps first only",
			authors:         []Author{{Name: "Alice"}, {Name: "Bob"}, {Name: "Charlie"}},
			expectedAuthors: []Author{{Name: "Alice"}},
		},
		{
			name:            "no authors unchanged",
			authors:         nil,
			expectedAuthors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			feed := &Feed{
				Entries: []Entry{
					{
						ID:      "urn:test:1",
						Title:   "Test Book",
						Authors: tt.authors,
					},
				},
			}
			truncateFeedAuthors(feed)
			assert.Equal(t, tt.expectedAuthors, feed.Entries[0].Authors)
		})
	}
}

func TestIsKOReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userAgent string
		expected  bool
	}{
		{
			name:      "KOReader user agent",
			userAgent: "KOReader/2024.04 (Linux)",
			expected:  true,
		},
		{
			name:      "KOReader case variant",
			userAgent: "koreader/2024.04",
			expected:  false,
		},
		{
			name:      "non-KOReader user agent",
			userAgent: "Mozilla/5.0",
			expected:  false,
		},
		{
			name:      "empty user agent",
			userAgent: "",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.expected, isKOReader(c))
		})
	}
}

func TestRespondXML_KOReaderConcatsAuthors(t *testing.T) {
	t.Parallel()

	feed := &Feed{
		Xmlns:   AtomNS,
		ID:      "urn:test:feed",
		Title:   "Test Feed",
		Updated: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Entries: []Entry{
			{
				ID:    "urn:test:1",
				Title: "Multi-Author Book",
				Authors: []Author{
					{Name: "First Author"},
					{Name: "Second Author"},
				},
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "KOReader/2024.04 (Linux)")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := respondXML(c, feed)
	require.NoError(t, err)

	var parsed Feed
	err = xml.Unmarshal(rec.Body.Bytes(), &parsed)
	require.NoError(t, err)

	require.Len(t, parsed.Entries, 1)
	require.Len(t, parsed.Entries[0].Authors, 1)
	assert.Equal(t, "First Author", parsed.Entries[0].Authors[0].Name)
}

func TestRespondXML_NonKOReaderKeepsSeparateAuthors(t *testing.T) {
	t.Parallel()

	feed := &Feed{
		Xmlns:   AtomNS,
		ID:      "urn:test:feed",
		Title:   "Test Feed",
		Updated: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Entries: []Entry{
			{
				ID:    "urn:test:1",
				Title: "Multi-Author Book",
				Authors: []Author{
					{Name: "First Author"},
					{Name: "Second Author"},
				},
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := respondXML(c, feed)
	require.NoError(t, err)

	var parsed Feed
	err = xml.Unmarshal(rec.Body.Bytes(), &parsed)
	require.NoError(t, err)

	require.Len(t, parsed.Entries, 1)
	require.Len(t, parsed.Entries[0].Authors, 2)
	assert.Equal(t, "First Author", parsed.Entries[0].Authors[0].Name)
	assert.Equal(t, "Second Author", parsed.Entries[0].Authors[1].Name)
}
