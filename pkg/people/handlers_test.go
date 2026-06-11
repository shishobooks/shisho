package people

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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

func createTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

func newTestHandler(db *bun.DB) *handler {
	return &handler{
		personService: NewService(db),
		aliasService:  aliases.NewService(db),
		searchService: search.NewService(db),
	}
}

func seedPersonWithAuthoredBooks(t *testing.T, db *bun.DB, lib *models.Library, personName string, bookTitles []string) *models.Person {
	t.Helper()
	ctx := context.Background()

	person := &models.Person{
		LibraryID:      lib.ID,
		Name:           personName,
		SortName:       personName,
		SortNameSource: models.DataSourceFilepath,
	}
	_, err := db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	for _, title := range bookTitles {
		book := &models.Book{
			LibraryID:       lib.ID,
			Title:           title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        t.TempDir(),
		}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		author := &models.Author{
			BookID:    book.ID,
			PersonID:  person.ID,
			SortOrder: 1,
		}
		_, err = db.NewInsert().Model(author).Exec(ctx)
		require.NoError(t, err)
	}

	return person
}

func seedPersonWithNarratedFiles(t *testing.T, db *bun.DB, lib *models.Library, personName string, fileNames []string) *models.Person {
	t.Helper()
	ctx := context.Background()

	person := &models.Person{
		LibraryID:      lib.ID,
		Name:           personName,
		SortName:       personName,
		SortNameSource: models.DataSourceFilepath,
	}
	_, err := db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	for _, name := range fileNames {
		book := &models.Book{
			LibraryID:       lib.ID,
			Title:           "Book for " + name,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Book for " + name,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        t.TempDir(),
		}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeM4B,
			FileRole:      models.FileRoleMain,
			Filepath:      "/tmp/" + name + ".m4b",
			FilesizeBytes: 1,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		narrator := &models.Narrator{
			FileID:    file.ID,
			PersonID:  person.ID,
			SortOrder: 1,
		}
		_, err = db.NewInsert().Model(narrator).Exec(ctx)
		require.NoError(t, err)
	}

	return person
}

func TestAuthoredBooks_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	// Seed one more book than the default limit so the test pins the
	// default-limit=24 contract instead of passing for any bind default.
	titles := make([]string, 25)
	for i := range titles {
		titles[i] = fmt.Sprintf("Book %02d", i+1)
	}
	person := seedPersonWithAuthoredBooks(t, db, lib, "Author A", titles)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.authoredBooks(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 25, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 24, "default limit must be 24")
}

func TestAuthoredBooks_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	person := seedPersonWithAuthoredBooks(t, db, lib, "Author B", []string{"B1", "B2", "B3", "B4", "B5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.authoredBooks(c)
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

func TestAuthoredBooks_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	person := seedPersonWithAuthoredBooks(t, db, lib, "Author C", []string{"B1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.authoredBooks(c)
	require.NoError(t, err)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp, 2, "response should have exactly 2 keys")
	assert.Contains(t, resp, "items")
	assert.Contains(t, resp, "total")
}

func TestNarratedFiles_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	// Seed one more file than the default limit so the test pins the
	// default-limit=24 contract instead of passing for any bind default.
	names := make([]string, 25)
	for i := range names {
		names[i] = fmt.Sprintf("n%02d", i+1)
	}
	person := seedPersonWithNarratedFiles(t, db, lib, "Narrator A", names)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.narratedFiles(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 25, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 24, "default limit must be 24")
}

func TestNarratedFiles_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	person := seedPersonWithNarratedFiles(t, db, lib, "Narrator B", []string{"n1", "n2", "n3", "n4", "n5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.narratedFiles(c)
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

func TestNarratedFiles_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	person := seedPersonWithNarratedFiles(t, db, lib, "Narrator C", []string{"n1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(person.ID))

	err := h.narratedFiles(c)
	require.NoError(t, err)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp, 2, "response should have exactly 2 keys")
	assert.Contains(t, resp, "items")
	assert.Contains(t, resp, "total")
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		person := &models.Person{
			LibraryID:      lib.ID,
			Name:           fmt.Sprintf("Person %d", i),
			SortName:       fmt.Sprintf("Person %d", i),
			SortNameSource: models.DataSourceFilepath,
		}
		_, err := db.NewInsert().Model(person).Exec(ctx)
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
	assert.NotContains(t, resp, "people", "response should not use 'people' key")
	assert.Contains(t, resp, "total")

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestList_ResponseAliasesSerializeAsStringArray(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	person := seedPersonWithAuthoredBooks(t, db, lib, "Brandon Sanderson", []string{"Book1"})

	// Seed two aliases so we can assert they round-trip as JSON strings.
	_, err := db.NewRaw(
		"INSERT INTO person_aliases (created_at, person_id, name, library_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		time.Now(), person.ID, "B. Sanderson", lib.ID,
		time.Now(), person.ID, "Brandon S.", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	assert.True(t, hasItems, "list response must have 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	// Each item's aliases must serialize as a JSON array of strings (the #324 fix
	// at the wire level), and the counts must be present.
	var resp struct {
		Items []struct {
			ID                int             `json:"id"`
			Name              string          `json:"name"`
			AuthoredBookCount int             `json:"authored_book_count"`
			NarratedFileCount int             `json:"narrated_file_count"`
			Aliases           json.RawMessage `json:"aliases"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Brandon Sanderson", resp.Items[0].Name)
	assert.Equal(t, 1, resp.Items[0].AuthoredBookCount)

	// aliases must be a JSON array whose elements are strings, not objects.
	var aliasStrings []string
	require.NoError(t, json.Unmarshal(resp.Items[0].Aliases, &aliasStrings),
		"aliases must unmarshal into []string, proving it is a JSON array of strings")
	assert.ElementsMatch(t, []string{"B. Sanderson", "Brandon S."}, aliasStrings)
}
