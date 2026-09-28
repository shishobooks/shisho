package tags

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func callTagMerge(t *testing.T, h *handler, user *models.User, targetID, sourceID int) error {
	t.Helper()
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"source_id":%d}`, sourceID)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(targetID))
	c.Set("user", user)
	return h.merge(c)
}

func tagUserWithAccess(libraryID int) *models.User {
	return &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &libraryID}}}
}

func requireTagErr(t *testing.T, err error, status int) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, status, codeErr.HTTPCode, "error: %s", codeErr.Message)
}

func createMergeTag(t *testing.T, db *bun.DB, lib *models.Library, name string) *models.Tag {
	t.Helper()
	g := &models.Tag{LibraryID: lib.ID, Name: name}
	_, err := db.NewInsert().Model(g).Exec(context.Background())
	require.NoError(t, err)
	return g
}

func tagExists(t *testing.T, db *bun.DB, id int) bool {
	t.Helper()
	count, err := db.NewSelect().Model((*models.Tag)(nil)).Where("id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count == 1
}

func tagBookCount(t *testing.T, db *bun.DB, id int) int {
	t.Helper()
	count, err := db.NewSelect().Model((*models.BookTag)(nil)).Where("tag_id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count
}

func TestMergeTags_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	tag := createMergeTag(t, db, lib, "Cozy")
	createTagBook(t, db, lib, nil, tag.ID)

	err := callTagMerge(t, h, tagUserWithAccess(lib.ID), tag.ID, tag.ID)
	requireTagErr(t, err, http.StatusUnprocessableEntity)

	assert.True(t, tagExists(t, db, tag.ID), "the Tag still exists")
	assert.Equal(t, 1, tagBookCount(t, db, tag.ID), "the Tag keeps its Book")
}

func TestMergeTagsService_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	tag := createMergeTag(t, db, lib, "Cozy")
	createTagBook(t, db, lib, nil, tag.ID)

	err := svc.MergeTags(context.Background(), tag.ID, tag.ID)
	requireTagErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, tagExists(t, db, tag.ID))
	assert.Equal(t, 1, tagBookCount(t, db, tag.ID))
}

func TestMergeTag_SourceInInaccessibleLibrary_Forbidden(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	visible := createTestLibrary(t, db)
	hidden := createTestLibrary(t, db)
	target := createMergeTag(t, db, visible, "Target")
	source := createMergeTag(t, db, hidden, "Source")

	err := callTagMerge(t, h, tagUserWithAccess(visible.ID), target.ID, source.ID)
	requireTagErr(t, err, http.StatusForbidden)
	assert.True(t, tagExists(t, db, source.ID), "the source Tag still exists")
}

func TestMergeTag_SourceInOtherLibrary_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	first := createTestLibrary(t, db)
	second := createTestLibrary(t, db)
	target := createMergeTag(t, db, first, "Target")
	source := createMergeTag(t, db, second, "Source")
	allAccess := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	err := callTagMerge(t, h, allAccess, target.ID, source.ID)
	requireTagErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, tagExists(t, db, source.ID), "the source Tag still exists")
}

func TestMergeTag_MissingSource_NotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	target := createMergeTag(t, db, lib, "Target")

	err := callTagMerge(t, h, tagUserWithAccess(lib.ID), target.ID, target.ID+1000)
	requireTagErr(t, err, http.StatusNotFound)
	assert.True(t, tagExists(t, db, target.ID), "the target Tag still exists")
}
