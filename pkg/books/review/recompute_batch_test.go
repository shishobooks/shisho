package review

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type queryCounter struct {
	total, updates atomic.Int64
}

func (c *queryCounter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (c *queryCounter) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	c.total.Add(1)
	if event.Operation() == "UPDATE" {
		c.updates.Add(1)
	}
}

func insertLibrary(ctx context.Context, t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	return library
}

func insertBook(ctx context.Context, t *testing.T, db *bun.DB, library *models.Library, title string) *models.Book {
	t.Helper()
	book := &models.Book{LibraryID: library.ID, Title: title, TitleSource: "filepath", Filepath: "/tmp/" + title, SortTitle: title, SortTitleSource: "filepath", AuthorSource: "filepath"}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	return book
}

func insertBookGenre(ctx context.Context, t *testing.T, db *bun.DB, library *models.Library, book *models.Book) {
	t.Helper()
	genre := &models.Genre{LibraryID: library.ID, Name: "Genre for " + book.Title}
	_, err := db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genre.ID}).Exec(ctx)
	require.NoError(t, err)
}

// fileOptions are the file columns a test varies. Reviewed starts at the
// opposite of the expected result where a test wants to see a write.
type fileOptions struct {
	name     string
	fileType string
	role     string
	reviewed *bool
	override *string
}

func insertFile(ctx context.Context, t *testing.T, db *bun.DB, book *models.Book, opts fileOptions) *models.File {
	t.Helper()
	file := &models.File{
		LibraryID:      book.LibraryID,
		BookID:         book.ID,
		Filepath:       fmt.Sprintf("%s/%s", book.Filepath, opts.name),
		FileType:       opts.fileType,
		FileRole:       opts.role,
		FilesizeBytes:  1,
		Reviewed:       opts.reviewed,
		ReviewOverride: opts.override,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	return file
}

func storedReviewed(ctx context.Context, t *testing.T, db *bun.DB, fileID int) *bool {
	t.Helper()
	var file models.File
	require.NoError(t, db.NewSelect().Model(&file).Where("f.id = ?", fileID).Scan(ctx))
	return file.Reviewed
}

// Each file state the single-file path handles gets the same answer from a
// batch, whichever way its stored value starts.
func TestRecomputeForBooks_AppliesEveryFileState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	criteria := Criteria{BookFields: []string{FieldGenres}, AudioFields: []string{FieldNarrators}}

	incomplete := insertBook(ctx, t, db, library, "incomplete")
	complete := insertBook(ctx, t, db, library, "complete")
	insertBookGenre(ctx, t, db, library, complete)
	unlisted := insertBook(ctx, t, db, library, "unlisted")

	type want struct {
		file *models.File
		want *bool
	}
	cases := map[string]want{
		"a book missing a required field": {
			insertFile(ctx, t, db, incomplete, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)}), ptrBool(false),
		},
		"a complete book": {
			insertFile(ctx, t, db, complete, fileOptions{name: "b.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(false)}), ptrBool(true),
		},
		"a file never computed": {
			insertFile(ctx, t, db, complete, fileOptions{name: "c.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain}), ptrBool(true),
		},
		"an audio file of a complete book without its audio field": {
			insertFile(ctx, t, db, complete, fileOptions{name: "d.m4b", fileType: models.FileTypeM4B, role: models.FileRoleMain, reviewed: ptrBool(true)}), ptrBool(false),
		},
		"an override to reviewed on an incomplete book": {
			insertFile(ctx, t, db, incomplete, fileOptions{name: "e.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(false), override: ptrStr(models.ReviewOverrideReviewed)}), ptrBool(true),
		},
		"an override to unreviewed on a complete book": {
			insertFile(ctx, t, db, complete, fileOptions{name: "f.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true), override: ptrStr(models.ReviewOverrideUnreviewed)}), ptrBool(false),
		},
		"a supplement": {
			insertFile(ctx, t, db, complete, fileOptions{name: "g.pdf", fileType: models.FileTypePDF, role: models.FileRoleSupplement, reviewed: ptrBool(true)}), nil,
		},
		"a book that was not listed": {
			insertFile(ctx, t, db, unlisted, fileOptions{name: "h.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)}), ptrBool(true),
		},
	}

	require.NoError(t, RecomputeForBooks(ctx, db, []int{incomplete.ID, complete.ID}, criteria))

	for name, c := range cases {
		assert.Equal(t, c.want, storedReviewed(ctx, t, db, c.file.ID), name)
	}
}

// A shared resource can sit on tens of thousands of books, so the query count
// must follow the number of batches rather than the number of books.
func TestRecomputeForBooks_QueryCountFollowsBatchesNotBooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	queriesFor := func(bookCount int) int64 {
		db := testdb.New(t)
		library := insertLibrary(ctx, t, db)
		ids := make([]int, 0, bookCount)
		for i := range bookCount {
			book := insertBook(ctx, t, db, library, fmt.Sprintf("book-%d", i))
			insertFile(ctx, t, db, book, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)})
			ids = append(ids, book.ID)
		}
		counter := &queryCounter{}
		testdb.AddQueryHook(t, db, counter)
		require.NoError(t, RecomputeForBooks(ctx, db, ids, Default()))
		return counter.total.Load()
	}

	assert.Equal(t, queriesFor(2), queriesFor(40), "one batch runs the same queries for 2 books as for 40")
}

// Only the files whose value changes are written, in one statement per value.
func TestRecomputeForBooks_WritesOnlyChangedFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	ids := make([]int, 0, 10)
	for i := range 10 {
		book := insertBook(ctx, t, db, library, fmt.Sprintf("book-%d", i))
		insertFile(ctx, t, db, book, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)})
		ids = append(ids, book.ID)
	}
	counter := &queryCounter{}
	testdb.AddQueryHook(t, db, counter)

	require.NoError(t, RecomputeForBooks(ctx, db, ids, Default()))
	assert.Equal(t, int64(1), counter.updates.Load(), "ten files flipping the same way take one statement")

	counter.updates.Store(0)
	require.NoError(t, RecomputeForBooks(ctx, db, ids, Default()))
	assert.Zero(t, counter.updates.Load(), "a second run has nothing to change")
}

// A list longer than one batch reaches every book, including the ones past
// the boundary.
func TestRecomputeForBooks_CoversBooksAcrossBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	count := RecomputeBatchSize*2 + 1
	ids := make([]int, 0, count)
	fileIDs := make([]int, 0, count)
	for i := range count {
		book := insertBook(ctx, t, db, library, fmt.Sprintf("book-%d", i))
		fileIDs = append(fileIDs, insertFile(ctx, t, db, book, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)}).ID)
		ids = append(ids, book.ID)
	}

	require.NoError(t, RecomputeForBooks(ctx, db, ids, Default()))

	stale, err := db.NewSelect().Model((*models.File)(nil)).Where("reviewed = ?", true).Where("id IN (?)", bun.List(fileIDs)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, stale)
}

func TestRecomputeForBooks_NoBooksIsANoOp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	counter := &queryCounter{}
	testdb.AddQueryHook(t, db, counter)

	require.NoError(t, RecomputeForBooks(ctx, db, nil, Default()))
	assert.Zero(t, counter.total.Load())
}

// A file meets the identifier and chapter criteria from its own rows, so a
// batch has to load both relations and hand them to the right files.
func TestRecomputeForBooks_ReadsFileRelations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	book := insertBook(ctx, t, db, library, "book")
	criteria := Criteria{BookFields: []string{FieldIdentifiers, FieldChapters}}

	insertIdentifier := func(file *models.File) {
		_, err := db.NewInsert().Model(&models.FileIdentifier{FileID: file.ID, Type: "isbn_13", Value: fmt.Sprintf("97800000%d", file.ID), Source: models.DataSourceManual}).Exec(ctx)
		require.NoError(t, err)
	}
	insertChapter := func(file *models.File) {
		_, err := db.NewInsert().Model(&models.Chapter{FileID: file.ID, SortOrder: 0, Title: "One"}).Exec(ctx)
		require.NoError(t, err)
	}
	epub := func(name string, reviewed bool) *models.File {
		return insertFile(ctx, t, db, book, fileOptions{name: name, fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(reviewed)})
	}
	both := epub("both.epub", false)
	insertIdentifier(both)
	insertChapter(both)
	identifierOnly := epub("identifier.epub", true)
	insertIdentifier(identifierOnly)
	chapterOnly := epub("chapter.epub", true)
	insertChapter(chapterOnly)

	require.NoError(t, RecomputeForBooks(ctx, db, []int{book.ID}, criteria))

	assert.Equal(t, ptrBool(true), storedReviewed(ctx, t, db, both.ID))
	assert.Equal(t, ptrBool(false), storedReviewed(ctx, t, db, identifierOnly.ID))
	assert.Equal(t, ptrBool(false), storedReviewed(ctx, t, db, chapterOnly.ID))
}

// One unreadable row costs only its own book: the rest of its batch and the
// later batches are still written, and the error names the book.
func TestRecomputeForBooks_IsolatesAFailingBook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	count := RecomputeBatchSize + 2
	ids := make([]int, 0, count)
	fileIDs := make([]int, 0, count)
	for i := range count {
		book := insertBook(ctx, t, db, library, fmt.Sprintf("book-%d", i))
		fileIDs = append(fileIDs, insertFile(ctx, t, db, book, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain, reviewed: ptrBool(true)}).ID)
		ids = append(ids, book.ID)
	}
	_, err := db.NewUpdate().Model((*models.File)(nil)).Set("created_at = 'not a time'").Where("id = ?", fileIDs[1]).Exec(ctx)
	require.NoError(t, err)

	err = RecomputeForBooks(ctx, db, ids, Default())

	require.ErrorContains(t, err, fmt.Sprintf("book %d", ids[1]))
	stale, err := db.NewSelect().Model((*models.File)(nil)).Where("reviewed = ?", true).Where("id != ?", fileIDs[1]).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, stale, "every other file is recomputed")
}

// A cancellation fails every book the same way, so it returns at once rather
// than retrying each book of the batch.
func TestRecomputeForBooks_StopsWhenCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db := testdb.New(t)

	require.ErrorIs(t, RecomputeForBooks(ctx, db, []int{1, 2, 3}, Default()), context.Canceled)
}

// A file whose book is gone violates a foreign key, so it takes already bad
// data, but it must surface as an error rather than a panic.
func TestRecomputeForBooks_FileWithoutBookIsAnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	library := insertLibrary(ctx, t, db)
	book := insertBook(ctx, t, db, library, "book")
	insertFile(ctx, t, db, book, fileOptions{name: "a.epub", fileType: models.FileTypeEPUB, role: models.FileRoleMain})
	_, err := db.Exec("PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	_, err = db.NewDelete().Model((*models.Book)(nil)).Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)

	require.Error(t, RecomputeForBooks(ctx, db, []int{book.ID}, Default()))
}
