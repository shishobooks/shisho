package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// setupDirectoryBackedFile creates an organized library with one Book whose
// EPUB sits in the Book's folder, and returns the database, the file, and a
// user who can edit it.
func setupDirectoryBackedFile(t *testing.T) (*bun.DB, *models.File, *models.User) {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	bookDir := filepath.Join(libraryDir, "[Author] Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	epubPath := filepath.Join(bookDir, "Book.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub"), 0644))

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))
	return db, file, user
}

func renameFileViaAPI(t *testing.T, db *bun.DB, user *models.User, fileID int, name string) {
	t.Helper()
	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(fileID), strings.NewReader(`{"name": "`+name+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())
}

func TestUpdateFile_Name_DirectoryBacked_SkipsPathClaimedByAnotherFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, file, user := setupDirectoryBackedFile(t)

	claimed := filepath.Join(filepath.Dir(file.Filepath), "New Name.epub")
	_, err := db.ExecContext(ctx, `INSERT INTO files (library_id, book_id, filepath, file_type, file_role)
		VALUES (?, ?, ?, 'epub', 'supplement')`, file.LibraryID, file.BookID, claimed)
	require.NoError(t, err)

	renameFileViaAPI(t, db, user, file.ID, "New Name")

	var reloaded models.File
	require.NoError(t, db.NewSelect().Model(&reloaded).Where("id = ?", file.ID).Scan(ctx))
	assert.Equal(t, filepath.Join(filepath.Dir(file.Filepath), "New Name (1).epub"), reloaded.Filepath)
	assert.FileExists(t, reloaded.Filepath)
	require.NotNil(t, reloaded.Name)
	assert.Equal(t, "New Name", *reloaded.Name)
}

func TestUpdateFile_Name_DirectoryBacked_UndoesRenameWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, file, user := setupDirectoryBackedFile(t)

	_, err := db.ExecContext(ctx, `CREATE TRIGGER fail_filepath_update BEFORE UPDATE OF filepath ON files
		BEGIN SELECT RAISE(ABORT, 'forced filepath update failure'); END`)
	require.NoError(t, err)

	renameFileViaAPI(t, db, user, file.ID, "New Name")

	assert.FileExists(t, file.Filepath, "the file is moved back")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(file.Filepath), "New Name.epub"))
	var reloaded models.File
	require.NoError(t, db.NewSelect().Model(&reloaded).Where("id = ?", file.ID).Scan(ctx))
	assert.Equal(t, file.Filepath, reloaded.Filepath)
	assert.Nil(t, reloaded.Name, "the name is not saved while the file keeps its old name")
}
