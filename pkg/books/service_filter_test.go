package books

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedPerson + seedAuthor are scoped to filter tests; the existing
// seed helpers in service_sort_test.go don't touch authors because
// sort tests don't need them.
func seedPerson(t *testing.T, db *bun.DB, lib *models.Library, name string) *models.Person {
	t.Helper()
	p := &models.Person{
		LibraryID:      lib.ID,
		Name:           name,
		SortName:       name,
		SortNameSource: models.DataSourceFilepath,
	}
	_, err := db.NewInsert().Model(p).Exec(context.Background())
	require.NoError(t, err)
	return p
}

func seedAuthor(t *testing.T, db *bun.DB, book *models.Book, person *models.Person) {
	t.Helper()
	// authors.sort_order is NOT NULL with no default; the model uses
	// `nullzero` which would strip a 0 from the INSERT, so seed with
	// 1 rather than the natural 0-index.
	a := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	_, err := db.NewInsert().Model(a).Exec(context.Background())
	require.NoError(t, err)
}

// TestListBooks_PersonIDFilter confirms PersonID restricts results to
// books authored by that person, with the user-supplied Sort applied.
//
// This is the seam the eReader AuthorBooks handler relies on — the old
// implementation called peopleService.GetAuthoredBooks (which orders by
// title only and ignores library scope), then filtered in Go. Routing
// through the books service with PersonID + Sort fixes both: the SQL
// applies the user's sort preference, and the LibraryID filter scopes
// to a single library before the Person join.
func TestListBooks_PersonIDFilter(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	// Two books by Alice, one by Bob — proves Bob's book is excluded.
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-2*time.Hour))
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	bobsBook := seedBook(t, db, lib, "BobsBook", "BobsBook", now.Add(-time.Hour))

	alice := seedPerson(t, db, lib, "Alice")
	bob := seedPerson(t, db, lib, "Bob")
	seedAuthor(t, db, apple, alice)
	seedAuthor(t, db, cheese, alice)
	seedAuthor(t, db, bobsBook, bob)

	// Filter to Alice + sort by date_added DESC.
	got, total, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		PersonID:  &alice.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	require.Equal(t, 2, total, "PersonID filter excludes Bob's book")
	require.Len(t, got, 2)
	// date_added DESC → cheese (now) before apple (now-2h).
	assert.Equal(t, cheese.ID, got[0].ID)
	assert.Equal(t, apple.ID, got[1].ID)
}

// TestListBooks_PersonIDFilter_ScopesToLibrary confirms PersonID
// composes with LibraryID — the same person can author books in
// multiple libraries (the persons table is library-scoped, but in
// practice users can have the same name across libraries via separate
// Person rows). The bigger guarantee is that LibraryID still narrows
// the result set when both are set, so the eReader's per-library view
// doesn't leak books from sibling libraries.
func TestListBooks_PersonIDFilter_ScopesToLibrary(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	libA := seedLibrary(t, db, "LibA")
	libB := seedLibrary(t, db, "LibB")

	now := time.Now()
	bookInA := seedBook(t, db, libA, "BookA", "BookA", now)
	bookInB := seedBook(t, db, libB, "BookB", "BookB", now)
	// Same person row authoring books across two libraries — contrived,
	// but the SQL filter shouldn't care about the person's home library.
	alice := seedPerson(t, db, libA, "Alice")
	seedAuthor(t, db, bookInA, alice)
	seedAuthor(t, db, bookInB, alice)

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &libA.ID,
		PersonID:  &alice.ID,
	})
	require.NoError(t, err)
	require.Len(t, got, 1, "LibraryID restricts to libA only")
	assert.Equal(t, bookInA.ID, got[0].ID)
}

// TestListBooks_ReviewedFilter confirms that:
//   - "needs_review" returns books where any main file has reviewed=FALSE or NULL
//   - "reviewed" returns books where all main files have reviewed=TRUE
//   - NULL is treated as "needs review" (migration gap state)
func TestListBooks_ReviewedFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "L")

	bookCounter := 0
	mkBook := func(reviewed *bool) int {
		bookCounter++
		book := seedBook(t, db, lib, "T"+strconv.Itoa(bookCounter), "T"+strconv.Itoa(bookCounter), time.Now())
		f := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			Filepath:      "/tmp/" + strconv.Itoa(book.ID),
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			FilesizeBytes: 1,
			Reviewed:      reviewed,
		}
		_, err := db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
		return book.ID
	}

	tru := true
	fal := false
	_ = mkBook(&tru)          // reviewed=TRUE  → should NOT appear in needs_review
	bookFalse := mkBook(&fal) // reviewed=FALSE → needs_review
	bookNull := mkBook(nil)   // reviewed=NULL  → needs_review (migration gap)

	// needs_review: FALSE + NULL should appear; TRUE should not
	books, _, err := svc.ListBooksWithTotal(ctx, ListBooksOptions{
		LibraryID:      &lib.ID,
		ReviewedFilter: "needs_review",
	})
	require.NoError(t, err)
	gotIDs := make([]int, 0, len(books))
	for _, b := range books {
		gotIDs = append(gotIDs, b.ID)
	}
	require.ElementsMatch(t, []int{bookFalse, bookNull}, gotIDs, "needs_review: FALSE and NULL books only")

	// reviewed: only TRUE should appear
	books, _, err = svc.ListBooksWithTotal(ctx, ListBooksOptions{
		LibraryID:      &lib.ID,
		ReviewedFilter: "reviewed",
	})
	require.NoError(t, err)
	gotIDs = make([]int, 0, len(books))
	for _, b := range books {
		gotIDs = append(gotIDs, b.ID)
	}
	// Only the TRUE book should appear; FALSE and NULL are excluded
	require.Len(t, gotIDs, 1, "reviewed: only the TRUE book")
	assert.NotContains(t, gotIDs, bookFalse)
	assert.NotContains(t, gotIDs, bookNull)
}
