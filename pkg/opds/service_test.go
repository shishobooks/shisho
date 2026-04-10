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
