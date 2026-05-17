package publishers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// setupHandlerTestDB creates an in-memory SQLite database using a named memory
// URI so that Bun's ScanAndCount (which opens a second connection for the COUNT
// query) sees the same database. Plain ":memory:" gives each connection its own
// private database, which causes "no such table" errors from ScanAndCount.
func setupHandlerTestDB(t *testing.T) *bun.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func newTestEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return e
}

func newTestHandler(db *bun.DB) *handler {
	return &handler{
		publisherService: NewService(db),
		aliasService:     aliases.NewService(db),
		searchService:    search.NewService(db),
	}
}

func seedPublisherWithFiles(t *testing.T, db *bun.DB, lib *models.Library, pubName string, fileNames []string) *models.Publisher {
	t.Helper()
	ctx := context.Background()

	publisher := &models.Publisher{LibraryID: lib.ID, Name: pubName}
	_, err := db.NewInsert().Model(publisher).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book for " + pubName,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book for " + pubName,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for _, name := range fileNames {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      "/tmp/" + name + ".epub",
			FilesizeBytes: 1,
			PublisherID:   &publisher.ID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	return publisher
}

func TestFiles_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "Penguin", []string{"f1", "f2", "f3"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestFiles_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "HarperCollins", []string{"f1", "f2", "f3", "f4", "f5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 5, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestFiles_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "Tor", []string{"f1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp, 2, "response should have exactly 2 keys")
	assert.Contains(t, resp, "items")
	assert.Contains(t, resp, "total")
}

func TestRetrieve_IncludesAncestorsAndDescendants(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	_, err := db.NewInsert().Model(root).Exec(ctx)
	require.NoError(t, err)

	middle := &models.Publisher{LibraryID: lib.ID, Name: "Middle", ParentID: &root.ID}
	_, err = db.NewInsert().Model(middle).Exec(ctx)
	require.NoError(t, err)

	leaf := &models.Publisher{LibraryID: lib.ID, Name: "Leaf", ParentID: &middle.ID}
	_, err = db.NewInsert().Model(leaf).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(middle.ID))

	err = h.retrieve(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Check ancestors (should be root)
	assert.Contains(t, resp, "ancestors")
	var ancestors []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err = json.Unmarshal(resp["ancestors"], &ancestors)
	require.NoError(t, err)
	require.Len(t, ancestors, 1)
	assert.Equal(t, "Root", ancestors[0].Name)
	assert.Equal(t, root.ID, ancestors[0].ID)

	// Check descendant_ids (should be leaf)
	assert.Contains(t, resp, "descendant_ids")
	var descendantIDs []int
	err = json.Unmarshal(resp["descendant_ids"], &descendantIDs)
	require.NoError(t, err)
	require.Len(t, descendantIDs, 1)
	assert.Equal(t, leaf.ID, descendantIDs[0])
}

func TestUpdate_SetParent(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child"}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := fmt.Sprintf(`{"parent_id": %d}`, parent.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(child.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the parent was set
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	assert.Equal(t, parent.ID, *updated.ParentID)
}

func TestUpdate_ClearParent(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &parent.ID}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := `{"parent_id": null}`
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(child.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the parent was cleared
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	assert.Nil(t, updated.ParentID)
}

func TestUpdate_CycleRejected(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	_, err := db.NewInsert().Model(pubA).Exec(ctx)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B", ParentID: &pubA.ID}
	_, err = db.NewInsert().Model(pubB).Exec(ctx)
	require.NoError(t, err)

	// Try to set A's parent to B (would create A->B->A cycle)
	e := newTestEcho(t)
	body := fmt.Sprintf(`{"parent_id": %d}`, pubB.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pubA.ID))

	err = h.update(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		pub := &models.Publisher{LibraryID: lib.ID, Name: fmt.Sprintf("Publisher %d", i)}
		_, err := db.NewInsert().Model(pub).Exec(ctx)
		require.NoError(t, err)
	}

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp, "items", "response should use 'items' key")
	assert.NotContains(t, resp, "publishers", "response should not use 'publishers' key")
	assert.Contains(t, resp, "total")

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestFiles_IncludesDescendantPublisherFiles(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	// Create publisher hierarchy: parent -> child
	parent := seedPublisherWithFiles(t, db, lib, "Parent Corp", []string{"parent-f1"})
	child := &models.Publisher{LibraryID: lib.ID, Name: "Child Imprint", ParentID: &parent.ID}
	_, err := db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	// Add files to the child publisher
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Child Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Child Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	childFile := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/child-f1.epub",
		FilesizeBytes: 1,
		PublisherID:   &child.ID,
	}
	_, err = db.NewInsert().Model(childFile).Exec(ctx)
	require.NoError(t, err)

	// Request files for the parent publisher
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(parent.ID))

	err = h.files(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "parent publisher should show files from self + child")

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestRetrieve_FileCountIncludesDescendants(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	// Create hierarchy: root -> child with files on both
	root := seedPublisherWithFiles(t, db, lib, "Root Publisher", []string{"root-file"})
	child := &models.Publisher{LibraryID: lib.ID, Name: "Child Publisher", ParentID: &root.ID}
	_, err := db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Child Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Child Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	childFile := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/child-pub-file.epub",
		FilesizeBytes: 1,
		PublisherID:   &child.ID,
	}
	_, err = db.NewInsert().Model(childFile).Exec(ctx)
	require.NoError(t, err)

	// Request the root publisher detail
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(root.ID))

	err = h.retrieve(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var fileCount int
	err = json.Unmarshal(resp["file_count"], &fileCount)
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount, "file_count should include descendant publisher files")
}


func TestSetChild_Success(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child"}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := fmt.Sprintf(`{"child_id": %d}`, child.ID)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(parent.ID))

	err = h.setChild(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify child's parent_id was set to parent
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	assert.Equal(t, parent.ID, *updated.ParentID)
}

func TestSetChild_CycleRejected(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	// Create hierarchy: A -> B (B is child of A)
	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	_, err := db.NewInsert().Model(pubA).Exec(ctx)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B", ParentID: &pubA.ID}
	_, err = db.NewInsert().Model(pubB).Exec(ctx)
	require.NoError(t, err)

	// Try to set A as child of B (would create B->A->B cycle)
	e := newTestEcho(t)
	body := fmt.Sprintf(`{"child_id": %d}`, pubA.ID)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pubB.ID))

	err = h.setChild(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestSetChild_SamePublisherRejected(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Self"}
	_, err := db.NewInsert().Model(pub).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := fmt.Sprintf(`{"child_id": %d}`, pub.ID)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err = h.setChild(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestSetChild_LibraryAccessEnforced(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child"}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	// User without access to this library (no LibraryAccess entries)
	user := &models.User{Username: "restricted", PasswordHash: "x", LibraryAccess: []*models.UserLibraryAccess{}}

	e := newTestEcho(t)
	body := fmt.Sprintf(`{"child_id": %d}`, child.ID)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(parent.ID))
	c.Set("user", user)

	err = h.setChild(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access")
}


func TestUpdate_RenameTriggersmerge_ParentIDStillApplied(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	// Create the target publisher (the name we're renaming TO)
	target := &models.Publisher{LibraryID: lib.ID, Name: "Target"}
	_, err := db.NewInsert().Model(target).Exec(ctx)
	require.NoError(t, err)

	// Create the source publisher (the one being renamed/merged)
	source := &models.Publisher{LibraryID: lib.ID, Name: "Source"}
	_, err = db.NewInsert().Model(source).Exec(ctx)
	require.NoError(t, err)

	// Create a parent publisher to set as the parent of the merged result
	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err = db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	// Rename source to "Target" (triggering merge) and simultaneously set parent_id
	e := newTestEcho(t)
	body := fmt.Sprintf(`{"name": "Target", "parent_id": %d}`, parent.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(source.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// The merge target should now have the parent set
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &target.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID, "parent_id should be set on merge target")
	assert.Equal(t, parent.ID, *updated.ParentID)
}

func TestRetrieve_IncludesChildrenAndDescendantFileCount(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	childA := &models.Publisher{LibraryID: lib.ID, Name: "ChildA", ParentID: &parent.ID}
	_, err = db.NewInsert().Model(childA).Exec(ctx)
	require.NoError(t, err)

	childB := &models.Publisher{LibraryID: lib.ID, Name: "ChildB", ParentID: &parent.ID}
	_, err = db.NewInsert().Model(childB).Exec(ctx)
	require.NoError(t, err)

	// Create files: 2 direct on parent, 3 on childA, 1 on childB
	// Use the actual publisher IDs for file creation
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Direct Parent Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Direct Parent Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      fmt.Sprintf("/tmp/parent_file_%d.epub", i),
			FilesizeBytes: 1,
			PublisherID:   &parent.ID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	book2 := &models.Book{
		LibraryID:       lib.ID,
		Title:           "ChildA Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "ChildA Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book2.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      fmt.Sprintf("/tmp/childA_file_%d.epub", i),
			FilesizeBytes: 1,
			PublisherID:   &childA.ID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	book3 := &models.Book{
		LibraryID:       lib.ID,
		Title:           "ChildB Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "ChildB Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book3).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        book3.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/childB_file_0.epub",
		FilesizeBytes: 1,
		PublisherID:   &childB.ID,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(parent.ID))

	err = h.retrieve(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Check file_count (direct)
	var fileCount int
	err = json.Unmarshal(resp["file_count"], &fileCount)
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount)

	// Check descendant_file_count (files on children)
	assert.Contains(t, resp, "descendant_file_count")
	var descendantFileCount int
	err = json.Unmarshal(resp["descendant_file_count"], &descendantFileCount)
	require.NoError(t, err)
	assert.Equal(t, 4, descendantFileCount) // 3 + 1

	// Check children
	assert.Contains(t, resp, "children")
	var children []struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		FileCount int    `json:"file_count"`
	}
	err = json.Unmarshal(resp["children"], &children)
	require.NoError(t, err)
	require.Len(t, children, 2)
	assert.Equal(t, "ChildA", children[0].Name)
	assert.Equal(t, 3, children[0].FileCount)
	assert.Equal(t, "ChildB", children[1].Name)
	assert.Equal(t, 1, children[1].FileCount)
}

func TestList_IncludesHierarchyCounts(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &parent.ID}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	// Create files on child
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Child Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Child Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      fmt.Sprintf("/tmp/list_child_file_%d.epub", i),
			FilesizeBytes: 1,
			PublisherID:   &child.ID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []struct {
			ID                       int     `json:"id"`
			Name                     string  `json:"name"`
			FileCount                int     `json:"file_count"`
			DescendantFileCount      int     `json:"descendant_file_count"`
			DescendantPublisherCount int     `json:"descendant_publisher_count"`
			ParentName               *string `json:"parent_name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)

	// Find parent and child in response (sorted by name: Child, Parent)
	var parentItem, childItem struct {
		ID                       int     `json:"id"`
		Name                     string  `json:"name"`
		FileCount                int     `json:"file_count"`
		DescendantFileCount      int     `json:"descendant_file_count"`
		DescendantPublisherCount int     `json:"descendant_publisher_count"`
		ParentName               *string `json:"parent_name"`
	}
	for _, item := range resp.Items {
		if item.Name == "Parent" {
			parentItem = item
		} else {
			childItem = item
		}
	}

	// Parent: 0 direct files, 2 descendant files, 1 descendant publisher
	assert.Equal(t, 0, parentItem.FileCount)
	assert.Equal(t, 2, parentItem.DescendantFileCount)
	assert.Equal(t, 1, parentItem.DescendantPublisherCount)
	assert.Nil(t, parentItem.ParentName)

	// Child: 2 direct files, 0 descendant files, 0 descendant publishers, parent name = "Parent"
	assert.Equal(t, 2, childItem.FileCount)
	assert.Equal(t, 0, childItem.DescendantFileCount)
	assert.Equal(t, 0, childItem.DescendantPublisherCount)
	require.NotNil(t, childItem.ParentName)
	assert.Equal(t, "Parent", *childItem.ParentName)
}
