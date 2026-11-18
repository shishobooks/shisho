package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// GET /books/sidecar-impact counts the distinct Books whose sidecars a rename
// or merge rewrites, for the note the rename and merge dialogs show.

func seedGenreBooks(t *testing.T, db *bun.DB, libraryID int, genre *models.Genre, n int) []*models.Book {
	t.Helper()
	ctx := context.Background()
	out := make([]*models.Book, 0, n)
	for i := 0; i < n; i++ {
		book := &models.Book{LibraryID: libraryID, Title: "B", TitleSource: models.DataSourceFilepath, SortTitle: "B", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: t.TempDir()}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genre.ID}).Exec(ctx)
		require.NoError(t, err)
		out = append(out, book)
	}
	return out
}

func sidecarImpact(t *testing.T, db *bun.DB, user *models.User, query string) *httptest.ResponseRecorder {
	t.Helper()
	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/sidecar-impact?"+query, nil)
	return executeRequestWithUser(t, e, req, user)
}

func TestSidecarImpact_CountsDistinctBooksAcrossTargetAndSource(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	library, _, _ := seedBookAndFile(t, db, "Unrelated", nil, nil, models.FileRoleMain)
	user := impactUser(t, db, "admin", models.RoleAdmin, library.ID, true)

	target := &models.Genre{LibraryID: library.ID, Name: "Fiction"}
	source := &models.Genre{LibraryID: library.ID, Name: "Novels"}
	for _, g := range []*models.Genre{target, source} {
		_, err := db.NewInsert().Model(g).Exec(ctx)
		require.NoError(t, err)
	}
	targetBooks := seedGenreBooks(t, db, library.ID, target, 3)
	seedGenreBooks(t, db, library.ID, source, 2)
	// One Book holds both, and is rewritten once.
	_, err := db.NewInsert().Model(&models.BookGenre{BookID: targetBooks[0].ID, GenreID: source.ID}).Exec(ctx)
	require.NoError(t, err)

	rr := sidecarImpact(t, db, user, "kind=genre&ids="+itoa(target.ID))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp SidecarImpactResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.BookCount)

	rr = sidecarImpact(t, db, user, "kind=genre&ids="+itoa(target.ID)+"&ids="+itoa(source.ID))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	// Three target Books and two source Books, one shared target Book
	// counted once: 5, where adding the two counts would give 6.
	assert.Equal(t, 5, resp.BookCount)
}

func TestSidecarImpact_CountsBooksOfNarratedFiles(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	library, book, file := seedBookAndFile(t, db, "Audio", nil, nil, models.FileRoleMain)
	user := impactUser(t, db, "admin", models.RoleAdmin, library.ID, true)
	person := &models.Person{LibraryID: library.ID, Name: "Narrator", SortName: "Narrator", SortNameSource: models.DataSourceFilepath}
	_, err := db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Narrator{FileID: file.ID, PersonID: person.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	rr := sidecarImpact(t, db, user, "kind=person&ids="+itoa(person.ID))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp SidecarImpactResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.BookCount, "a Book the person authored and narrated counts once")
}

func TestSidecarImpact_Errors(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	library, _, _ := seedBookAndFile(t, db, "A", nil, nil, models.FileRoleMain)
	genre := &models.Genre{LibraryID: library.ID, Name: "Fiction"}
	_, err := db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)

	admin := impactUser(t, db, "admin", models.RoleAdmin, library.ID, true)
	assert.Equal(t, http.StatusNotFound, sidecarImpact(t, db, admin, "kind=genre&ids=99999").Code)
	assert.Equal(t, http.StatusUnprocessableEntity, sidecarImpact(t, db, admin, "kind=nope&ids="+itoa(genre.ID)).Code)

	noAccess := impactUser(t, db, "elsewhere", models.RoleAdmin, library.ID, false)
	assert.Equal(t, http.StatusForbidden, sidecarImpact(t, db, noAccess, "kind=genre&ids="+itoa(genre.ID)).Code)

	// A role that reads books but can't rename them.
	viewer := impactUser(t, db, "viewer", models.RoleViewer, library.ID, true)
	assert.Equal(t, http.StatusForbidden, sidecarImpact(t, db, viewer, "kind=genre&ids="+itoa(genre.ID)).Code)
}

// impactUser creates a user with the named role, with or without access to
// the library, and loads its role's permissions.
func impactUser(t *testing.T, db *bun.DB, username, roleName string, libraryID int, hasAccess bool) *models.User {
	t.Helper()
	ctx := context.Background()
	var roleID int
	require.NoError(t, db.NewSelect().Table("roles").Column("id").Where("name = ?", roleName).Scan(ctx, &roleID))
	user := &models.User{Username: username, PasswordHash: "hash", RoleID: roleID, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	var access []*models.UserLibraryAccess
	if hasAccess {
		row := &models.UserLibraryAccess{UserID: user.ID, LibraryID: &libraryID}
		_, err = db.NewInsert().Model(row).Exec(ctx)
		require.NoError(t, err)
		access = append(access, row)
	}
	user = loadUserWithRole(t, db, user)
	user.LibraryAccess = access
	return user
}
