package libraries

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func storedLibraryPaths(t *testing.T, db *bun.DB, id int) []string {
	t.Helper()
	var paths []string
	require.NoError(t, db.NewSelect().Model((*models.LibraryPath)(nil)).Column("filepath").
		Where("library_id = ?", id).Order("filepath ASC").Scan(context.Background(), &paths))
	return paths
}

// A path pasted with spaces around it is saved without them.
func TestCreateLibrary_TrimsLibraryPaths(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	dir := t.TempDir()

	status, resp := newLibraryPoster(t, db)("/libraries",
		`{"name":"Books","cover_aspect_ratio":"book","library_paths":["  `+dir+`/ "]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, []string{dir}, storedLibraryPaths(t, db, int(resp["id"].(float64))))
}

func TestUpdateLibrary_TrimsLibraryPaths(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seeded := seedLibraryWithContent(context.Background(), t, db, "Books")
	dir := t.TempDir()

	status, resp := newLibraryPoster(t, db)("/libraries/"+strconv.Itoa(seeded.LibraryID),
		`{"library_paths":["`+dir+`  "]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, []string{dir}, storedLibraryPaths(t, db, seeded.LibraryID))
}

// A path that is blank once trimmed is rejected rather than saved empty.
func TestCreateLibrary_RejectsBlankLibraryPath(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	status, resp := newLibraryPoster(t, db)("/libraries",
		`{"name":"Books","cover_aspect_ratio":"book","library_paths":["   "]}`)
	assert.Equal(t, http.StatusUnprocessableEntity, status, resp)
}
