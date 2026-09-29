package publishers

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

func callPublisherMerge(t *testing.T, h *handler, user *models.User, targetID, sourceID int) error {
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

func publisherUserWithAccess(libraryID int) *models.User {
	return &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &libraryID}}}
}

func requirePublisherErr(t *testing.T, err error, status int) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, status, codeErr.HTTPCode, "error: %s", codeErr.Message)
}

func createMergePublisher(t *testing.T, db *bun.DB, lib *models.Library, name string, parentID *int) *models.Publisher {
	t.Helper()
	p := &models.Publisher{LibraryID: lib.ID, Name: name, ParentID: parentID}
	_, err := db.NewInsert().Model(p).Exec(context.Background())
	require.NoError(t, err)
	return p
}

func publisherExists(t *testing.T, db *bun.DB, id int) bool {
	t.Helper()
	count, err := db.NewSelect().Model((*models.Publisher)(nil)).Where("id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count == 1
}

func publisherFileCount(t *testing.T, db *bun.DB, id int) int {
	t.Helper()
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("publisher_id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count
}

func publisherParent(t *testing.T, db *bun.DB, id int) *int {
	t.Helper()
	var parentID *int
	require.NoError(t, db.NewSelect().Model((*models.Publisher)(nil)).Column("parent_id").Where("id = ?", id).Scan(context.Background(), &parentID))
	return parentID
}

func TestMergePublishers_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	publisher := createMergePublisher(t, db, lib, "Tor", nil)
	child := createMergePublisher(t, db, lib, "Tor Teen", &publisher.ID)
	createTestFile(t, db, lib, publisher.ID, "/tmp/self-merge.epub")

	err := callPublisherMerge(t, h, publisherUserWithAccess(lib.ID), publisher.ID, publisher.ID)
	requirePublisherErr(t, err, http.StatusUnprocessableEntity)

	assert.True(t, publisherExists(t, db, publisher.ID), "the Publisher still exists")
	assert.Equal(t, 1, publisherFileCount(t, db, publisher.ID), "the File keeps its Publisher")
	parent := publisherParent(t, db, child.ID)
	require.NotNil(t, parent, "the child keeps its parent")
	assert.Equal(t, publisher.ID, *parent)
}

func TestMergePublishersService_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	publisher := createMergePublisher(t, db, lib, "Tor", nil)
	createTestFile(t, db, lib, publisher.ID, "/tmp/self-merge.epub")

	err := svc.MergePublishers(context.Background(), publisher.ID, publisher.ID)
	requirePublisherErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, publisherExists(t, db, publisher.ID))
	assert.Equal(t, 1, publisherFileCount(t, db, publisher.ID))
}

func TestMergePublisher_SourceInInaccessibleLibrary_Forbidden(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	visible := createTestLibrary(t, db)
	hidden := createTestLibrary(t, db)
	target := createMergePublisher(t, db, visible, "Target", nil)
	source := createMergePublisher(t, db, hidden, "Source", nil)

	err := callPublisherMerge(t, h, publisherUserWithAccess(visible.ID), target.ID, source.ID)
	requirePublisherErr(t, err, http.StatusForbidden)
	assert.True(t, publisherExists(t, db, source.ID), "the source Publisher still exists")
}

func TestMergePublisher_SourceInOtherLibrary_Rejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	first := createTestLibrary(t, db)
	second := createTestLibrary(t, db)
	target := createMergePublisher(t, db, first, "Target", nil)
	source := createMergePublisher(t, db, second, "Source", nil)
	allAccess := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	err := callPublisherMerge(t, h, allAccess, target.ID, source.ID)
	requirePublisherErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, publisherExists(t, db, source.ID), "the source Publisher still exists")
}

func TestMergePublisher_MissingSource_NotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	target := createMergePublisher(t, db, lib, "Target", nil)

	err := callPublisherMerge(t, h, publisherUserWithAccess(lib.ID), target.ID, target.ID+1000)
	requirePublisherErr(t, err, http.StatusNotFound)
	assert.True(t, publisherExists(t, db, target.ID), "the target Publisher still exists")
}

// With root -> source -> middle -> target, re-parenting the source's
// children to the target would make middle and target each other's parent.
// The target takes the source's place under root instead, and middle becomes
// the target's child.
func TestMergePublishers_TargetIsGrandchildOfSource_NoCycle(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	root := createMergePublisher(t, db, lib, "Root", nil)
	source := createMergePublisher(t, db, lib, "Source", &root.ID)
	middle := createMergePublisher(t, db, lib, "Middle", &source.ID)
	target := createMergePublisher(t, db, lib, "Target", &middle.ID)

	require.NoError(t, svc.MergePublishers(ctx, target.ID, source.ID))

	targetParent := publisherParent(t, db, target.ID)
	require.NotNil(t, targetParent, "the target takes the source's parent")
	assert.Equal(t, root.ID, *targetParent)
	middleParent := publisherParent(t, db, middle.ID)
	require.NotNil(t, middleParent)
	assert.Equal(t, target.ID, *middleParent, "the source's child moves to the target")
	assert.False(t, publisherExists(t, db, source.ID))

	ancestors, err := svc.GetAncestors(ctx, middle.ID)
	require.NoError(t, err)
	require.Len(t, ancestors, 2, "the hierarchy has no cycle")
	assert.Equal(t, target.ID, ancestors[0].ID)
	assert.Equal(t, root.ID, ancestors[1].ID)
}

// Under a pre-existing cycle (source -> middle -> target -> source), the
// source's parent is the target itself, so the target becomes a root rather
// than its own parent.
func TestMergePublishers_TargetBelowSourceInCycle_BecomesRoot(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createTestLibrary(t, db)
	source := createMergePublisher(t, db, lib, "Source", nil)
	middle := createMergePublisher(t, db, lib, "Middle", &source.ID)
	target := createMergePublisher(t, db, lib, "Target", &middle.ID)
	_, err := db.NewUpdate().Model((*models.Publisher)(nil)).Set("parent_id = ?", target.ID).Where("id = ?", source.ID).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.MergePublishers(ctx, target.ID, source.ID))

	assert.Nil(t, publisherParent(t, db, target.ID), "the target becomes a root")
	middleParent := publisherParent(t, db, middle.ID)
	require.NotNil(t, middleParent)
	assert.Equal(t, target.ID, *middleParent)
}
