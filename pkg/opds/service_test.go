package opds

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

// TestBookToEntry_LanguageAndPublisher verifies that bookToEntryWithKepub
// populates dc:language and dc:publisher from the book's files.
func TestBookToEntry_LanguageAndPublisher(t *testing.T) {
	t.Parallel()

	english := "en-US"
	french := "fr"

	tests := []struct {
		name              string
		files             []*models.File
		primaryFileID     *int
		expectedLanguage  string
		expectedPublisher string
	}{
		{
			name: "single file with language and publisher",
			files: []*models.File{
				{
					ID:       1,
					FileType: models.FileTypeEPUB,
					Language: &english,
					Publisher: &models.Publisher{
						Name: "Penguin",
					},
				},
			},
			expectedLanguage:  "en-US",
			expectedPublisher: "Penguin",
		},
		{
			name: "falls back to first file with each field set",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB}, // no language, no publisher
				{
					ID:       2,
					FileType: models.FileTypeCBZ,
					Language: &english,
				},
				{
					ID:       3,
					FileType: models.FileTypeM4B,
					Publisher: &models.Publisher{
						Name: "Audible",
					},
				},
			},
			expectedLanguage:  "en-US",
			expectedPublisher: "Audible",
		},
		{
			name: "primary file is preferred over other files",
			files: []*models.File{
				{
					ID:        1,
					FileType:  models.FileTypeEPUB,
					Language:  &french,
					Publisher: &models.Publisher{Name: "Gallimard"},
				},
				{
					ID:        2,
					FileType:  models.FileTypeM4B,
					Language:  &english,
					Publisher: &models.Publisher{Name: "Audible"},
				},
			},
			primaryFileID:     intPtr(2),
			expectedLanguage:  "en-US",
			expectedPublisher: "Audible",
		},
		{
			name: "no files with metadata leaves fields empty",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB},
			},
			expectedLanguage:  "",
			expectedPublisher: "",
		},
		{
			name:              "no files leaves fields empty",
			files:             nil,
			expectedLanguage:  "",
			expectedPublisher: "",
		},
		{
			// Supplements are not the book — their language/publisher
			// shouldn't populate the OPDS entry. Practical case: a PDF
			// supplement carries no Language/Publisher today, but a future
			// parser path could leak supplement metadata into the book entry.
			name: "ignores supplement files when filling book metadata",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeM4B}, // main, no metadata
				{
					ID:        2,
					FileType:  models.FileTypePDF,
					FileRole:  models.FileRoleSupplement,
					Language:  &english,
					Publisher: &models.Publisher{Name: "Supplement Co"},
				},
			},
			expectedLanguage:  "",
			expectedPublisher: "",
		},
	}

	svc := &Service{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := &models.Book{
				ID:            1,
				Title:         "Test Book",
				Files:         tt.files,
				PrimaryFileID: tt.primaryFileID,
			}
			entry := svc.bookToEntryWithKepub("http://example.com/opds/v1", book, "", nil, false)
			assert.Equal(t, tt.expectedLanguage, entry.Language)
			assert.Equal(t, tt.expectedPublisher, entry.Publisher)
		})
	}
}

func intPtr(i int) *int {
	return &i
}

// TestBookToEntry_CoverLinkUsesOPDSPath verifies that the cover image link
// in an OPDS entry stays inside the /opds/v1 path. The cover endpoint must
// live under the OPDS group so it can authenticate via Basic Auth and so
// the Caddy /opds/* handler proxies it to the backend in production —
// linking to /books/:id/cover (the React route) returns the SPA shell to
// OPDS clients.
func TestBookToEntry_CoverLinkUsesOPDSPath(t *testing.T) {
	t.Parallel()

	coverFilename := "book.cover.jpg"
	book := &models.Book{
		ID:    42,
		Title: "Test Book",
		Files: []*models.File{
			{
				ID:                 1,
				FileType:           models.FileTypeEPUB,
				CoverImageFilename: &coverFilename,
			},
		},
	}

	svc := &Service{}
	baseURL := "http://example.com/opds/v1"
	entry := svc.bookToEntryWithKepub(baseURL, book, "book", nil, false)

	wantHref := "http://example.com/opds/v1/books/42/cover"
	var imageHref, thumbHref string
	for _, link := range entry.Links {
		switch link.Rel {
		case "http://opds-spec.org/image":
			imageHref = link.Href
		case "http://opds-spec.org/image/thumbnail":
			thumbHref = link.Href
		}
	}
	assert.Equal(t, wantHref, imageHref, "image link must point at the OPDS cover endpoint")
	assert.Equal(t, wantHref, thumbHref, "thumbnail link must point at the OPDS cover endpoint")
}

// TestBookToEntry_CoverLinkRespectsForwardedPrefix verifies that when the
// baseURL carries a reverse-proxy prefix (e.g. Caddy adds X-Forwarded-Prefix
// /api in dev), the cover URL keeps the prefix so it round-trips through
// the same proxy. Stripping /opds/v1 and reattaching to a bare host would
// drop the prefix and route the cover to a different handler.
func TestBookToEntry_CoverLinkRespectsForwardedPrefix(t *testing.T) {
	t.Parallel()

	coverFilename := "book.cover.jpg"
	book := &models.Book{
		ID:    42,
		Title: "Test Book",
		Files: []*models.File{
			{
				ID:                 1,
				FileType:           models.FileTypeEPUB,
				CoverImageFilename: &coverFilename,
			},
		},
	}

	svc := &Service{}
	baseURL := "https://example.com/api/opds/v1"
	entry := svc.bookToEntryWithKepub(baseURL, book, "book", nil, false)

	wantHref := "https://example.com/api/opds/v1/books/42/cover"
	for _, link := range entry.Links {
		if link.Rel == "http://opds-spec.org/image" {
			assert.Equal(t, wantHref, link.Href)
			return
		}
	}
	t.Fatalf("expected an image link with rel http://opds-spec.org/image, got %+v", entry.Links)
}
