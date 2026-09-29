package genres

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func callGenreMerge(t *testing.T, h *handler, user *models.User, targetID, sourceID int) error {
	t.Helper()
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"source_id":%d}`, sourceID)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(targetID))
	auth.SetUser(c, user)
	return h.merge(c)
}

func genreUserWithAccess(libraryID int) *models.User {
	return &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &libraryID}}}
}

func requireGenreErr(t *testing.T, err error, status int) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, status, codeErr.HTTPCode, "error: %s", codeErr.Message)
}

func createMergeGenre(t *testing.T, db *bun.DB, lib *models.Library, name string) *models.Genre {
	t.Helper()
	g := &models.Genre{LibraryID: lib.ID, Name: name}
	_, err := db.NewInsert().Model(g).Exec(context.Background())
	require.NoError(t, err)
	return g
}

func genreExists(t *testing.T, db *bun.DB, id int) bool {
	t.Helper()
	count, err := db.NewSelect().Model((*models.Genre)(nil)).Where("id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count == 1
}

func genreBookCount(t *testing.T, db *bun.DB, id int) int {
	t.Helper()
	count, err := db.NewSelect().Model((*models.BookGenre)(nil)).Where("genre_id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count
}

func TestMergeGenres_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(t, db)
	lib := createTestLibrary(t, db)
	genre := createMergeGenre(t, db, lib, "Fantasy")
	createGenreBook(t, db, lib, nil, genre.ID)

	err := callGenreMerge(t, h, genreUserWithAccess(lib.ID), genre.ID, genre.ID)
	requireGenreErr(t, err, http.StatusUnprocessableEntity)

	assert.True(t, genreExists(t, db, genre.ID), "the Genre still exists")
	assert.Equal(t, 1, genreBookCount(t, db, genre.ID), "the Genre keeps its Book")
}

func TestMergeGenresService_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	genre := createMergeGenre(t, db, lib, "Fantasy")
	createGenreBook(t, db, lib, nil, genre.ID)

	err := svc.MergeGenres(context.Background(), genre.ID, genre.ID)
	requireGenreErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, genreExists(t, db, genre.ID))
	assert.Equal(t, 1, genreBookCount(t, db, genre.ID))
}

func TestMergeGenre_SourceInInaccessibleLibrary_Forbidden(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(t, db)
	visible := createTestLibrary(t, db)
	hidden := createTestLibrary(t, db)
	target := createMergeGenre(t, db, visible, "Target")
	source := createMergeGenre(t, db, hidden, "Source")

	err := callGenreMerge(t, h, genreUserWithAccess(visible.ID), target.ID, source.ID)
	requireGenreErr(t, err, http.StatusForbidden)
	assert.True(t, genreExists(t, db, source.ID), "the source Genre still exists")
}

func TestMergeGenre_SourceInOtherLibrary_Rejected(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(t, db)
	first := createTestLibrary(t, db)
	second := createTestLibrary(t, db)
	target := createMergeGenre(t, db, first, "Target")
	source := createMergeGenre(t, db, second, "Source")
	allAccess := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	err := callGenreMerge(t, h, allAccess, target.ID, source.ID)
	requireGenreErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, genreExists(t, db, source.ID), "the source Genre still exists")
}

func TestMergeGenre_MissingSource_NotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(t, db)
	lib := createTestLibrary(t, db)
	target := createMergeGenre(t, db, lib, "Target")

	err := callGenreMerge(t, h, genreUserWithAccess(lib.ID), target.ID, target.ID+1000)
	requireGenreErr(t, err, http.StatusNotFound)
	assert.True(t, genreExists(t, db, target.ID), "the target Genre still exists")
}

// A source whose name is already a third Genre's alias merges without
// tripping ux_genre_aliases_name_library_id. The third Genre keeps the alias.
func TestMergeGenres_SourceNameIsAnotherGenresAlias(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	target := createMergeGenre(t, db, lib, "Fantasy")
	source := createMergeGenre(t, db, lib, "Sci-Fi")
	other := createMergeGenre(t, db, lib, "Science Fiction")
	_, err := db.NewRaw("INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), other.ID, "sci-fi", lib.ID).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.MergeGenres(ctx, target.ID, source.ID))

	assert.False(t, genreExists(t, db, source.ID))
	aliasSvc := aliases.NewService(db)
	targetAliases, err := aliasSvc.ListAliases(ctx, aliases.GenreConfig, target.ID)
	require.NoError(t, err)
	assert.Empty(t, targetAliases, "the name stays with the Genre that already has it as an alias")
	otherAliases, err := aliasSvc.ListAliases(ctx, aliases.GenreConfig, other.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"sci-fi"}, otherAliases)
}
