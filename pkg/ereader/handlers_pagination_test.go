package ereader

import (
	"context"
	"fmt"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePageParam pins the ?page= query-string contract: any
// non-parseable, missing, or non-positive value defaults to page 1 so
// the handlers never render with a bogus offset.
func TestParsePageParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 1},
		{raw: "1", want: 1},
		{raw: "2", want: 2},
		{raw: "9999", want: 9999},
		{raw: "0", want: 1},
		{raw: "-1", want: 1},
		{raw: "abc", want: 1},
		{raw: " 2", want: 1},                   // strconv.Atoi rejects leading whitespace.
		{raw: "99999999999999999999", want: 1}, // overflow -> Atoi errors -> 1.
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("raw=%q", tc.raw), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, parsePageParam(tc.raw))
		})
	}
}

// TestListBooksPaginated_NoFilter confirms SQL-side pagination when no
// type filter is active: each page returns defaultPageSize books (or
// the remainder on the last page) and total reflects the full
// unfiltered count.
func TestListBooksPaginated_NoFilter(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// Seed defaultPageSize + 5 books to exercise multi-page behavior.
	total := defaultPageSize + 5
	for i := 0; i < total; i++ {
		b := &models.Book{
			LibraryID:       lib.ID,
			Title:           fmt.Sprintf("Book %03d", i),
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       fmt.Sprintf("Book %03d", i),
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        fmt.Sprintf("/b/%d", i),
		}
		_, err := db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
	}

	h := &handler{bookService: books.NewService(db, appsettings.NewService(db))}
	opts := books.ListBooksOptions{LibraryID: &lib.ID}

	got, gotTotal, err := h.listBooksPaginated(ctx, opts, 1, "")
	require.NoError(t, err)
	assert.Equal(t, total, gotTotal)
	assert.Len(t, got, defaultPageSize, "page 1 returns a full page")

	got, gotTotal, err = h.listBooksPaginated(ctx, opts, 2, "")
	require.NoError(t, err)
	assert.Equal(t, total, gotTotal)
	assert.Len(t, got, 5, "page 2 returns the remainder")

	// Out-of-range pages return an empty slice but still report the
	// real total so the UI can render "Page N of M" instead of
	// double-fetching to discover the bounds.
	got, gotTotal, err = h.listBooksPaginated(ctx, opts, 99, "")
	require.NoError(t, err)
	assert.Equal(t, total, gotTotal)
	assert.Empty(t, got)
}

// TestListBooksPaginated_TypeFilter confirms that an active type
// filter returns the filtered total (not the unfiltered count).
func TestListBooksPaginated_TypeFilter(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// 3 EPUB books and 2 CBZ books.
	mkBook := func(i int, fileType string) {
		b := &models.Book{
			LibraryID:       lib.ID,
			Title:           fmt.Sprintf("Book %03d", i),
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       fmt.Sprintf("Book %03d", i),
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        fmt.Sprintf("/b/%d", i),
		}
		_, err := db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
		f := &models.File{
			LibraryID:     lib.ID,
			BookID:        b.ID,
			FileType:      fileType,
			FileRole:      models.FileRoleMain,
			Filepath:      fmt.Sprintf("/b/%d/f", i),
			FilesizeBytes: 1,
		}
		_, err = db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		mkBook(i, models.FileTypeEPUB)
	}
	for i := 3; i < 5; i++ {
		mkBook(i, models.FileTypeCBZ)
	}

	h := &handler{bookService: books.NewService(db, appsettings.NewService(db))}
	opts := books.ListBooksOptions{LibraryID: &lib.ID}

	got, total, err := h.listBooksPaginated(ctx, opts, 1, "epub")
	require.NoError(t, err)
	assert.Equal(t, 3, total, "total reflects filtered count, not SQL count")
	assert.Len(t, got, 3)

	// "all" short-circuits the filter path (treated as no filter).
	_, total, err = h.listBooksPaginated(ctx, opts, 1, "all")
	require.NoError(t, err)
	assert.Equal(t, 5, total, `"all" is equivalent to no filter`)
}

// TestListBooksPaginated_TypeFilterPagesAndMatching confirms a type filter
// pages through the matching books, ignores the type's case, and matches a
// book by any of its main files but never by a supplement.
func TestListBooksPaginated_TypeFilterPagesAndMatching(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// mkBook adds a book whose files are main files of mainTypes and
	// supplements of supplementTypes.
	mkBook := func(i int, mainTypes []string, supplementTypes ...string) {
		b := &models.Book{
			LibraryID:       lib.ID,
			Title:           fmt.Sprintf("Book %03d", i),
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       fmt.Sprintf("Book %03d", i),
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        fmt.Sprintf("/b/%d", i),
		}
		_, err := db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
		add := func(fileType, role string) {
			_, err := db.NewInsert().Model(&models.File{
				LibraryID: lib.ID, BookID: b.ID, FileType: fileType, FileRole: role,
				Filepath: fmt.Sprintf("/b/%d/%s.%s", i, role, fileType), FilesizeBytes: 1,
			}).Exec(ctx)
			require.NoError(t, err)
		}
		for _, ft := range mainTypes {
			add(ft, models.FileRoleMain)
		}
		for _, ft := range supplementTypes {
			add(ft, models.FileRoleSupplement)
		}
	}

	// defaultPageSize + 3 books with an EPUB and a MOBI, two with only a
	// CBZ, and one with an EPUB main file and a PDF supplement.
	n := defaultPageSize + 3
	for i := 0; i < n; i++ {
		mkBook(i, []string{models.FileTypeEPUB, models.FileTypeMOBI})
	}
	mkBook(n, []string{models.FileTypeCBZ})
	mkBook(n+1, []string{models.FileTypeCBZ})
	mkBook(n+2, []string{models.FileTypeEPUB}, models.FileTypePDF)

	h := &handler{bookService: books.NewService(db, appsettings.NewService(db))}
	opts := books.ListBooksOptions{LibraryID: &lib.ID}

	got, total, err := h.listBooksPaginated(ctx, opts, 1, "mobi")
	require.NoError(t, err)
	assert.Equal(t, n, total)
	assert.Len(t, got, defaultPageSize)

	got, total, err = h.listBooksPaginated(ctx, opts, 2, "mobi")
	require.NoError(t, err)
	assert.Equal(t, n, total)
	assert.Len(t, got, 3)

	// The second type of a book's main files matches, in any case.
	_, total, err = h.listBooksPaginated(ctx, opts, 1, "EPUB")
	require.NoError(t, err)
	assert.Equal(t, n+1, total)

	// A supplement's type matches nothing.
	got, total, err = h.listBooksPaginated(ctx, opts, 1, "pdf")
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, got)
}
