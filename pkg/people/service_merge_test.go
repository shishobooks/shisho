package people

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// authorRow is one authors row as the merge tests compare it.
type authorRow struct {
	PersonID int
	Role     *string
}

func insertAuthor(t *testing.T, db *bun.DB, bookID, personID, sortOrder int, role *string) {
	t.Helper()
	_, err := db.NewInsert().
		Model(&models.Author{BookID: bookID, PersonID: personID, SortOrder: sortOrder, Role: role}).
		Exec(context.Background())
	require.NoError(t, err)
}

func bookAuthorRows(t *testing.T, db *bun.DB, bookID int) []authorRow {
	t.Helper()
	var rows []authorRow
	require.NoError(t, db.NewSelect().
		Model((*models.Author)(nil)).
		Column("person_id", "role").
		Where("book_id = ?", bookID).
		Order("sort_order").
		Scan(context.Background(), &rows))
	return rows
}

// When the source and the target author the same Book in the same role,
// re-pointing the source's row would duplicate the target's, which
// ux_authors_book_person_role rejects. The merge drops the source's row and
// keeps the target's.
func TestMergePeople_SharedBookSameRole_DropsSourceRow(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	source := createNamedPerson(t, svc, lib, "Source Person")
	target := createNamedPerson(t, svc, lib, "Target Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource)
	writer := testgen.StringPtr(models.AuthorRoleWriter)
	insertAuthor(t, db, bookID, target.ID, 1, writer)
	insertAuthor(t, db, bookID, source.ID, 2, writer)

	movedBookIDs, err := svc.MergePeople(ctx, target.ID, source.ID)
	require.NoError(t, err)

	assert.Equal(t, []int{bookID}, movedBookIDs)
	assert.Equal(t, []authorRow{{PersonID: target.ID, Role: writer}}, bookAuthorRows(t, db, bookID))
}

// A generic Author has a NULL role, and SQLite treats NULLs as distinct in a
// unique index, so the index would not catch a duplicate here. The merge
// still drops the source's row so the Book does not list the target twice.
func TestMergePeople_SharedBookGenericRole_DropsSourceRow(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	source := createNamedPerson(t, svc, lib, "Source Person")
	target := createNamedPerson(t, svc, lib, "Target Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource, target.ID, source.ID)

	_, err := svc.MergePeople(ctx, target.ID, source.ID)
	require.NoError(t, err)

	assert.Equal(t, []authorRow{{PersonID: target.ID}}, bookAuthorRows(t, db, bookID))
}

// A source row in a different role than the target's is not a duplicate, so
// the merge re-points it and the Book keeps both roles.
func TestMergePeople_SharedBookDifferentRole_RepointsSourceRow(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	source := createNamedPerson(t, svc, lib, "Source Person")
	target := createNamedPerson(t, svc, lib, "Target Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource)
	writer := testgen.StringPtr(models.AuthorRoleWriter)
	penciller := testgen.StringPtr(models.AuthorRolePenciller)
	insertAuthor(t, db, bookID, target.ID, 1, writer)
	insertAuthor(t, db, bookID, source.ID, 2, penciller)

	_, err := svc.MergePeople(ctx, target.ID, source.ID)
	require.NoError(t, err)

	assert.Equal(t, []authorRow{
		{PersonID: target.ID, Role: writer},
		{PersonID: target.ID, Role: penciller},
	}, bookAuthorRows(t, db, bookID))
}

// When the source and the target narrate the same File, re-pointing the
// source's row would violate ux_narrators_file_person. The merge drops the
// source's row and keeps the target's.
func TestMergePeople_SharedNarratedFile_DropsSourceRow(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	source := createNamedPerson(t, svc, lib, "Source Person")
	target := createNamedPerson(t, svc, lib, "Target Person")
	fileID := createNarratedFile(t, db, lib, nil, target.ID, source.ID)

	_, err := svc.MergePeople(ctx, target.ID, source.ID)
	require.NoError(t, err)

	_, personIDs := retrieveNarratedFile(t, db, fileID)
	assert.Equal(t, []int{target.ID}, personIDs)
}

// Merging a Person into itself would re-point its rows to itself and then
// delete it, so the merge rejects it before touching anything.
func TestMergePeople_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	person := createNamedPerson(t, svc, lib, "Only Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource, person.ID)

	_, err := svc.MergePeople(ctx, person.ID, person.ID)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "validation_error", codeErr.Code)

	_, err = svc.RetrievePerson(ctx, RetrievePersonOptions{ID: &person.ID})
	require.NoError(t, err, "the Person still exists")
	assert.Equal(t, []authorRow{{PersonID: person.ID}}, bookAuthorRows(t, db, bookID))
}

// SeriesForBooks returns each Series holding any of the Books once, and
// nothing for an empty list.
func TestSeriesForBooks_ReturnsEachSeriesOnce(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	first := createAuthoredBook(t, db, lib, personDeletePluginSource)
	second := createAuthoredBook(t, db, lib, personDeletePluginSource)
	outside := createAuthoredBook(t, db, lib, personDeletePluginSource)
	shared := &models.Series{LibraryID: lib.ID, Name: "Shared", SortName: "Shared", NameSource: models.DataSourceManual, SortNameSource: models.DataSourceFilepath}
	other := &models.Series{LibraryID: lib.ID, Name: "Other", SortName: "Other", NameSource: models.DataSourceManual, SortNameSource: models.DataSourceFilepath}
	for _, s := range []*models.Series{shared, other} {
		_, err := db.NewInsert().Model(s).Exec(ctx)
		require.NoError(t, err)
	}
	for _, bs := range []*models.BookSeries{
		{BookID: first, SeriesID: shared.ID, SortOrder: 1},
		{BookID: second, SeriesID: shared.ID, SortOrder: 2},
		{BookID: outside, SeriesID: other.ID, SortOrder: 1},
	} {
		_, err := db.NewInsert().Model(bs).Exec(ctx)
		require.NoError(t, err)
	}

	series, err := svc.SeriesForBooks(ctx, []int{first, second})
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Equal(t, shared.ID, series[0].ID)

	series, err = svc.SeriesForBooks(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, series)
}
