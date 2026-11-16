package opds

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
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
		{
			name:            "empty authors unchanged",
			authors:         []Author{},
			expectedAuthors: []Author{},
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

func TestRespondXML_KOReaderTruncatesAuthors(t *testing.T) {
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

// KOReader plays no audiobook and offers no download for an acquisition it
// can't open, so a KOReader Device lists no audiobook: a book with only an
// M4B is left out, and a book with an EPUB too offers only the EPUB.
func TestOPDS_KOReaderDeviceListsNoAudiobooks(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("Audiobook Only", "", models.FileTypeM4B)
	both := f.addBook("Novel With Audio", "", models.FileTypeEPUB, models.FileTypeM4B)
	device := f.addDevice(devices.DeviceTypeKOReader)

	code, body := f.getAs(device.Credential, f.allBooksPath())
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "Audiobook Only")
	assert.Contains(t, body, acquisitionHref(both[models.FileTypeEPUB], "epub"))
	assert.NotContains(t, body, acquisitionHref(both[models.FileTypeM4B], "m4b"))
}

// A library holding only audiobooks has nothing a KOReader Device can open,
// so its root catalog leaves the library out rather than opening it to an
// empty feed.
func TestOPDS_KOReaderDeviceCatalogLeavesOutAnAudiobookOnlyLibrary(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("Audiobook Only", "", models.FileTypeM4B)
	device := f.addDevice(devices.DeviceTypeKOReader)

	code, body := f.getAs(device.Credential, "/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "Browse "+f.lib.Name)

	code, body = f.getAs("secret", "/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, "Browse "+f.lib.Name)
}

// The Device's type and rules decide what is served, never the User-Agent
// (ADR 0011), so KOReader signed in with the account password still gets
// the audiobooks the user's default delivers.
func TestOPDS_KOReaderUserAgentChangesNothingServed(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	audio := f.addBook("Audiobook Only", "", models.FileTypeM4B)

	rec := serveOPDSFrom(t, f.db, "KOReader/2026.07.1 (Linux)", "reader", "secret", f.allBooksPath())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), acquisitionHref(audio[models.FileTypeM4B], "m4b"))
}
