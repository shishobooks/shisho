package bookpreviews

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type fixture struct {
	t       *testing.T
	ctx     context.Context
	db      *bun.DB
	library *models.Library
	day     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	library := &models.Library{Name: "Library", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	return &fixture{t: t, ctx: ctx, db: db, library: library, day: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fixture) insert(model any) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(model).Exec(f.ctx)
	require.NoError(f.t, err)
}

// book adds a book added `addedDay` days after the fixture's first day, with
// one main file released on `released` (nil for no release date).
func (f *fixture) book(title string, addedDay int, released *time.Time) (*models.Book, *models.File) {
	f.t.Helper()
	added := f.day.AddDate(0, 0, addedDay)
	book := &models.Book{
		LibraryID:       f.library.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/library/" + title,
		CreatedAt:       added,
		UpdatedAt:       added,
	}
	f.insert(book)
	cover := title + ".cover.jpg"
	file := &models.File{
		LibraryID:          f.library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           "/library/" + title + "/" + title + ".epub",
		FilesizeBytes:      1,
		CoverImageFilename: &cover,
		ReleaseDate:        released,
		CreatedAt:          added,
		UpdatedAt:          added,
	}
	f.insert(file)
	return book, file
}

func (f *fixture) year(y int) *time.Time {
	d := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	return &d
}

func titles(previews []PreviewBook) []string {
	out := make([]string, len(previews))
	for i, p := range previews {
		out[i] = p.Title
	}
	return out
}

func TestLoad_GenresPickNewestReleasedThenNewestAdded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	genre := &models.Genre{LibraryID: f.library.ID, Name: "Fantasy", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(genre)

	for _, b := range []struct {
		title    string
		added    int
		released *time.Time
	}{
		{"Old Release", 9, f.year(1990)},
		{"New Release", 1, f.year(2022)},
		{"Mid Release", 2, f.year(2010)},
		{"Undated Recent", 8, nil},
		{"Undated Older", 3, nil},
	} {
		book, _ := f.book(b.title, b.added, b.released)
		f.insert(&models.BookGenre{BookID: book.ID, GenreID: genre.ID})
	}

	previews, err := NewService(f.db).Load(f.ctx, Genres, []int{genre.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"New Release", "Mid Release", "Old Release"}, titles(previews[genre.ID]))
	for _, p := range previews[genre.ID] {
		assert.NotEmpty(t, p.CoverCacheKey, "each preview carries its cover key")
	}
}

func TestLoad_UndatedBooksFallBackToDateAdded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tag := &models.Tag{LibraryID: f.library.ID, Name: "Manga", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(tag)
	for _, b := range []struct {
		title string
		added int
	}{{"First Added", 1}, {"Last Added", 5}, {"Middle Added", 3}} {
		book, _ := f.book(b.title, b.added, nil)
		f.insert(&models.BookTag{BookID: book.ID, TagID: tag.ID})
	}

	previews, err := NewService(f.db).Load(f.ctx, Tags, []int{tag.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"Last Added", "Middle Added", "First Added"}, titles(previews[tag.ID]))
}

func TestLoad_PeopleShowAuthoredBooksBeforeNarratedOnes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	author := &models.Person{LibraryID: f.library.ID, Name: "Both", SortName: "Both", SortNameSource: string(models.DataSourceFilepath), CreatedAt: f.day, UpdatedAt: f.day}
	narrator := &models.Person{LibraryID: f.library.ID, Name: "Voice", SortName: "Voice", SortNameSource: string(models.DataSourceFilepath), CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(author)
	f.insert(narrator)

	written, _ := f.book("Written", 1, f.year(2000))
	f.insert(&models.Author{BookID: written.ID, PersonID: author.ID, SortOrder: 1})
	// Released later, but only narrated, so it comes after the written book.
	_, narratedFile := f.book("Narrated", 2, f.year(2023))
	f.insert(&models.Narrator{FileID: narratedFile.ID, PersonID: author.ID, SortOrder: 1})
	f.insert(&models.Narrator{FileID: narratedFile.ID, PersonID: narrator.ID, SortOrder: 2})

	previews, err := NewService(f.db).Load(f.ctx, People, []int{author.ID, narrator.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"Written", "Narrated"}, titles(previews[author.ID]))
	assert.Equal(t, []string{"Narrated"}, titles(previews[narrator.ID]), "a narrator previews the books of narrated files")
}

func TestLoad_PublishersIncludeSubPublishersBooks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	parent := &models.Publisher{LibraryID: f.library.ID, Name: "Parent", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(parent)
	child := &models.Publisher{LibraryID: f.library.ID, Name: "Imprint", ParentID: &parent.ID, CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(child)

	_, file := f.book("Imprint Book", 1, f.year(2015))
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).Set("publisher_id = ?", child.ID).Where("id = ?", file.ID).Exec(f.ctx)
	require.NoError(t, err)

	previews, err := NewService(f.db).Load(f.ctx, Publishers, []int{parent.ID, child.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"Imprint Book"}, titles(previews[parent.ID]), "a parent previews its imprints' books, as its file count counts their files")
	assert.Equal(t, []string{"Imprint Book"}, titles(previews[child.ID]))
}

func TestLoad_EntitiesWithoutBooksGetNone(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	genre := &models.Genre{LibraryID: f.library.ID, Name: "Empty", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(genre)

	previews, err := NewService(f.db).Load(f.ctx, Genres, []int{genre.ID})
	require.NoError(t, err)
	assert.Empty(t, previews[genre.ID])

	none, err := NewService(f.db).Load(f.ctx, Genres, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestLoad_ABookLinkedTwiceShowsOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	person := &models.Person{LibraryID: f.library.ID, Name: "Twice", SortName: "Twice", SortNameSource: string(models.DataSourceFilepath), CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(person)
	book, file := f.book("Same Book", 1, f.year(2000))
	f.insert(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1})
	f.insert(&models.Narrator{FileID: file.ID, PersonID: person.ID, SortOrder: 1})

	previews, err := NewService(f.db).Load(f.ctx, People, []int{person.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"Same Book"}, titles(previews[person.ID]))
}

// addFile adds another file to book, released on `released`.
func (f *fixture) addFile(book *models.Book, name string, fileType string, released *time.Time) *models.File {
	f.t.Helper()
	cover := name + ".cover.jpg"
	file := &models.File{
		LibraryID:          f.library.ID,
		BookID:             book.ID,
		FileType:           fileType,
		FileRole:           models.FileRoleMain,
		Filepath:           "/library/" + book.Title + "/" + name,
		FilesizeBytes:      1,
		CoverImageFilename: &cover,
		ReleaseDate:        released,
		CreatedAt:          book.CreatedAt,
		UpdatedAt:          book.CreatedAt,
	}
	f.insert(file)
	return file
}

func TestLoad_ReadsReleaseDatesLikeTheReleaseSort(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	genre := &models.Genre{LibraryID: f.library.ID, Name: "Editions", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(genre)

	// The release sort reads the newest file's date, so this book counts as
	// released in 2000 even though an older file says 2023.
	reissued, _ := f.book("Reissued", 1, f.year(2023))
	f.addFile(reissued, "reissue.epub", models.FileTypeEPUB, f.year(2000))
	f.insert(&models.BookGenre{BookID: reissued.ID, GenreID: genre.ID})
	single, _ := f.book("Single", 2, f.year(2010))
	f.insert(&models.BookGenre{BookID: single.ID, GenreID: genre.ID})

	previews, err := NewService(f.db).Load(f.ctx, Genres, []int{genre.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"Single", "Reissued"}, titles(previews[genre.ID]))
}

func TestLoad_CoverKeyFollowsTheLibrarysCoverChoice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.db.NewUpdate().Model(f.library).Set("cover_aspect_ratio = ?", models.CoverAspectRatioAudiobook).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	tag := &models.Tag{LibraryID: f.library.ID, Name: "Hybrid", CreatedAt: f.day, UpdatedAt: f.day}
	f.insert(tag)
	book, _ := f.book("Hybrid", 1, nil)
	m4b := f.addFile(book, "hybrid.m4b", models.FileTypeM4B, nil)
	f.insert(&models.BookTag{BookID: book.ID, TagID: tag.ID})

	previews, err := NewService(f.db).Load(f.ctx, Tags, []int{tag.ID})
	require.NoError(t, err)

	require.Len(t, previews[tag.ID], 1)
	// The book cover endpoint serves the audiobook's cover in an audiobook
	// library, so the key names the M4B file.
	assert.Equal(t, fmt.Sprintf("%d-%d", m4b.ID, m4b.UpdatedAt.Unix()), previews[tag.ID][0].CoverCacheKey)
}

func TestLoad_CoverAspectMatchesTheGallery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		libraryAspect string
		withM4B       bool
		want          string
	}{
		{models.CoverAspectRatioBook, true, "book"},
		{models.CoverAspectRatioAudiobook, false, "square"},
		{models.CoverAspectRatioBookFallbackAudiobook, false, "book"},
		{models.CoverAspectRatioAudiobookFallbackBook, true, "square"},
		{models.CoverAspectRatioAudiobookFallbackBook, false, "book"},
	} {
		t.Run(fmt.Sprintf("%s/m4b=%v", tc.libraryAspect, tc.withM4B), func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			_, err := f.db.NewUpdate().Model(f.library).Set("cover_aspect_ratio = ?", tc.libraryAspect).WherePK().Exec(f.ctx)
			require.NoError(t, err)
			tag := &models.Tag{LibraryID: f.library.ID, Name: "T", CreatedAt: f.day, UpdatedAt: f.day}
			f.insert(tag)
			book, _ := f.book("Book", 1, nil)
			if tc.withM4B {
				f.addFile(book, "book.m4b", models.FileTypeM4B, nil)
			}
			f.insert(&models.BookTag{BookID: book.ID, TagID: tag.ID})

			previews, err := NewService(f.db).Load(f.ctx, Tags, []int{tag.ID})
			require.NoError(t, err)

			require.Len(t, previews[tag.ID], 1)
			assert.Equal(t, tc.want, previews[tag.ID][0].CoverAspect)
		})
	}
}

// bareFile adds a file with no cover to book.
func (f *fixture) bareFile(book *models.Book, name, fileType, role string) {
	f.t.Helper()
	f.insert(&models.File{
		LibraryID:     f.library.ID,
		BookID:        book.ID,
		FileType:      fileType,
		FileRole:      role,
		Filepath:      "/library/" + book.Title + "/" + name,
		FilesizeBytes: 1,
		CreatedAt:     book.CreatedAt,
		UpdatedAt:     book.CreatedAt,
	})
}

// Books with no cover take the shape the gallery gives their placeholder,
// from the file types they have (getCoverFileType).
func TestLoad_CoverAspectWithoutACover(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		libraryAspect string
		files         [][2]string // file type, role
		want          string
	}{
		{"ebook and audiobook, audiobook first", models.CoverAspectRatioAudiobookFallbackBook,
			[][2]string{{models.FileTypeEPUB, models.FileRoleMain}, {models.FileTypeM4B, models.FileRoleMain}}, "square"},
		{"ebook and audiobook, book first", models.CoverAspectRatioBookFallbackAudiobook,
			[][2]string{{models.FileTypeEPUB, models.FileRoleMain}, {models.FileTypeM4B, models.FileRoleMain}}, "book"},
		{"audiobook only, book first", models.CoverAspectRatioBookFallbackAudiobook,
			[][2]string{{models.FileTypeM4B, models.FileRoleMain}}, "square"},
		{"supplement audiobook ignored", models.CoverAspectRatioAudiobookFallbackBook,
			[][2]string{{models.FileTypeEPUB, models.FileRoleMain}, {models.FileTypeM4B, models.FileRoleSupplement}}, "book"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			_, err := f.db.NewUpdate().Model(f.library).Set("cover_aspect_ratio = ?", tc.libraryAspect).WherePK().Exec(f.ctx)
			require.NoError(t, err)
			genre := &models.Genre{LibraryID: f.library.ID, Name: "G", CreatedAt: f.day, UpdatedAt: f.day}
			f.insert(genre)
			book := &models.Book{
				LibraryID: f.library.ID, Title: "Bare", TitleSource: models.DataSourceFilepath,
				SortTitle: "Bare", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath,
				Filepath: "/library/Bare", CreatedAt: f.day, UpdatedAt: f.day,
			}
			f.insert(book)
			for i, file := range tc.files {
				f.bareFile(book, fmt.Sprintf("f%d.%s", i, file[0]), file[0], file[1])
			}
			f.insert(&models.BookGenre{BookID: book.ID, GenreID: genre.ID})

			previews, err := NewService(f.db).Load(f.ctx, Genres, []int{genre.ID})
			require.NoError(t, err)

			require.Len(t, previews[genre.ID], 1)
			assert.Empty(t, previews[genre.ID][0].CoverCacheKey)
			assert.Equal(t, tc.want, previews[genre.ID][0].CoverAspect)
		})
	}
}
