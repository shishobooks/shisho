package review

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string        { return &s }
func ptrInt(i int) *int              { return &i }
func ptrBool(b bool) *bool           { return &b }
func ptrTime(t time.Time) *time.Time { return &t }

func TestMissingFields_Supplement_NoCheck(t *testing.T) {
	t.Parallel()
	book := &models.Book{}
	file := &models.File{FileRole: models.FileRoleSupplement}
	require.Nil(t, MissingFields(book, file, Default()))
}

func TestMissingFields_EpubMissingAll_Default(t *testing.T) {
	t.Parallel()
	book := &models.Book{}
	file := &models.File{FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	missing := MissingFields(book, file, Default())
	require.ElementsMatch(t, []string{"authors", "description", "cover", "genres"}, missing)
}

func TestMissingFields_EpubComplete(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
	}
	require.Empty(t, MissingFields(book, file, Default()))
}

func TestMissingFields_M4BNeedsNarrators(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
	}
	require.Contains(t, MissingFields(book, file, Default()), "narrators")
}

func TestMissingFields_M4BComplete(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
		Narrators:          []*models.Narrator{{}},
	}
	require.Empty(t, MissingFields(book, file, Default()))
}

func TestMissingFields_NonAudio_DoesNotCheckAudioFields(t *testing.T) {
	t.Parallel()
	criteria := Criteria{
		BookFields:  []string{},
		AudioFields: []string{"narrators"},
	}
	book := &models.Book{}
	file := &models.File{FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	require.Empty(t, MissingFields(book, file, criteria))
}

func TestMissingFields_AllFields(t *testing.T) {
	t.Parallel()
	criteria := Criteria{
		BookFields: []string{
			FieldAuthors, FieldDescription, FieldCover, FieldGenres, FieldTags,
			FieldSeries, FieldSubtitle, FieldPublisher, FieldImprint,
			FieldIdentifiers, FieldReleaseDate, FieldLanguage, FieldURL,
		},
		AudioFields: []string{FieldNarrators, FieldChapters, FieldAbridged},
	}
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		BookTags:    []*models.BookTag{{}},
		BookSeries:  []*models.BookSeries{{}},
		Description: ptrStr("desc"),
		Subtitle:    ptrStr("sub"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("c.jpg"),
		PublisherID:        ptrInt(1),
		ImprintID:          ptrInt(1),
		Identifiers:        []*models.FileIdentifier{{}},
		ReleaseDate:        ptrTime(time.Now()),
		Language:           ptrStr("en"),
		URL:                ptrStr("https://example"),
		Narrators:          []*models.Narrator{{}},
		Chapters:           []*models.Chapter{{}},
		Abridged:           ptrBool(false),
	}
	require.Empty(t, MissingFields(book, file, criteria))
}

func TestIsComplete_True(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("d"),
	}
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("c.jpg"),
	}
	require.True(t, IsComplete(book, file, Default()))
}

func TestIsComplete_False(t *testing.T) {
	t.Parallel()
	require.False(t, IsComplete(&models.Book{}, &models.File{
		FileType: models.FileTypeEPUB,
		FileRole: models.FileRoleMain,
	}, Default()))
}
