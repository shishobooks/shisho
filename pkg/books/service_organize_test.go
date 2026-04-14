package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrganizeBookFiles_RootLevel_CleansUpStaleBookFolderAfterTitleChange
// reproduces a bug where an enricher updates book.Title between initial
// sidecar write and organization, leaving an orphaned folder containing
// just the stale book sidecar next to the correctly organized folder.
//
// Sequence exercised:
//  1. Scan creates book.Filepath from the original title ("Old Title") and
//     writes a book sidecar inside that synthetic folder.
//  2. Enricher updates book.Title to "New Title".
//  3. Organization runs: it moves the root-level file into a new folder
//     computed from the new title and updates book.Filepath.
//
// The test asserts that the stale folder from step (1) is cleaned up and a
// fresh book sidecar lives at the new folder after organization.
func TestOrganizeBookFiles_RootLevel_CleansUpStaleBookFolderAfterTitleChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	libDir := t.TempDir()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  libDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// The scan first derives an organized folder from the file's initial
	// title "Old Title", creates that directory, and writes a book sidecar
	// into it. Replicate that synthetic pre-organize state on disk.
	oldBookFolder := filepath.Join(libDir, "[Test Author] Old Title")
	require.NoError(t, os.MkdirAll(oldBookFolder, 0755))
	staleSidecar := &sidecar.BookSidecar{Title: "Old Title"}
	require.NoError(t, sidecar.WriteBookSidecar(oldBookFolder, staleSidecar))
	staleSidecarPath := sidecar.BookSidecarPath(oldBookFolder)
	require.FileExists(t, staleSidecarPath, "sanity: stale sidecar should exist before organize")

	// The media file itself sits at the library root (pre-organization).
	rootLevelFile := filepath.Join(libDir, "source.epub")
	require.NoError(t, os.WriteFile(rootLevelFile, []byte("epub"), 0644))

	// Set up the book in the DB with the post-enricher title but the
	// pre-organize synthetic filepath, matching the state when
	// organizeBookFiles is invoked by the monitor/scan job.
	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Test Author",
		SortName:  "Author, Test",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "New Title",
		TitleSource:     models.DataSourcePlugin,
		SortTitle:       "New Title",
		SortTitleSource: models.DataSourcePlugin,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        oldBookFolder,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      rootLevelFile,
		FilesizeBytes: 4,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Reload the book so Authors relation is populated the same way the
	// production caller (monitor.organizeBooks) loads it.
	loadedBook, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	require.NoError(t, svc.OrganizeBookFiles(ctx, loadedBook))

	newBookFolder := filepath.Join(libDir, "[Test Author] New Title")
	newFilePath := filepath.Join(newBookFolder, "New Title.epub")

	// File moved to the new organized folder.
	assert.FileExists(t, newFilePath, "epub should be moved into new organized folder")
	_, err = os.Stat(rootLevelFile)
	assert.True(t, os.IsNotExist(err), "root-level file should no longer exist at original path")

	// Fresh book sidecar exists at the new folder.
	newSidecarPath := sidecar.BookSidecarPath(newBookFolder)
	assert.FileExists(t, newSidecarPath, "new book sidecar should exist in organized folder")

	// Old synthetic folder (and its stale sidecar) are cleaned up.
	_, err = os.Stat(staleSidecarPath)
	assert.True(t, os.IsNotExist(err), "stale book sidecar should be removed")
	_, err = os.Stat(oldBookFolder)
	assert.True(t, os.IsNotExist(err), "old synthetic book folder should be removed")

	// DB book.Filepath reflects the new organized folder.
	reloaded, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, newBookFolder, reloaded.Filepath)
}

// TestOrganizeBookFiles_RootLevel_PreservesUserFilesInStaleFolder pins the
// "leave user data alone" contract: if the stale pre-organize folder happens
// to contain files the user put there, organization must NOT delete them
// along with the stale sidecar. The folder stays behind with the user files
// intact.
func TestOrganizeBookFiles_RootLevel_PreservesUserFilesInStaleFolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	libDir := t.TempDir()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{LibraryID: library.ID, Filepath: libDir}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	oldBookFolder := filepath.Join(libDir, "[Test Author] Old Title")
	require.NoError(t, os.MkdirAll(oldBookFolder, 0755))
	require.NoError(t, sidecar.WriteBookSidecar(oldBookFolder, &sidecar.BookSidecar{Title: "Old Title"}))

	// A file the user dropped into the (synthetic) book folder. Must survive.
	userFile := filepath.Join(oldBookFolder, "notes.txt")
	require.NoError(t, os.WriteFile(userFile, []byte("user notes"), 0644))

	rootLevelFile := filepath.Join(libDir, "source.epub")
	require.NoError(t, os.WriteFile(rootLevelFile, []byte("epub"), 0644))

	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Test Author",
		SortName:  "Author, Test",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "New Title",
		TitleSource:     models.DataSourcePlugin,
		SortTitle:       "New Title",
		SortTitleSource: models.DataSourcePlugin,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        oldBookFolder,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      rootLevelFile,
		FilesizeBytes: 4,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	loadedBook, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	require.NoError(t, svc.OrganizeBookFiles(ctx, loadedBook))

	// Media file was still moved to the correct new folder.
	newBookFolder := filepath.Join(libDir, "[Test Author] New Title")
	assert.FileExists(t, filepath.Join(newBookFolder, "New Title.epub"))
	assert.FileExists(t, sidecar.BookSidecarPath(newBookFolder))

	// Stale sidecar was removed, but the user file and the folder itself
	// survived because os.Remove refused to delete a non-empty directory.
	assert.NoFileExists(t, sidecar.BookSidecarPath(oldBookFolder), "stale sidecar should be removed")
	assert.FileExists(t, userFile, "user-dropped file must be preserved")
	info, err := os.Stat(oldBookFolder)
	require.NoError(t, err, "stale folder should still exist because it was non-empty")
	assert.True(t, info.IsDir())
}

// TestOrganizeBookFiles_DirectoryBased_WritesFreshSidecarAfterFolderRename
// asserts that when an enricher changes book.Title and organization renames
// the book folder to match, a fresh book sidecar is written at the new
// folder path. The directory-based branch previously deleted the old sidecar
// (carried along by the folder rename under its old filename) without
// re-seeding a new one, leaving the book sidecar-less until the next scan.
func TestOrganizeBookFiles_DirectoryBased_WritesFreshSidecarAfterFolderRename(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	libDir := t.TempDir()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Directory-based book: files already live under book.Filepath.
	oldBookFolder := filepath.Join(libDir, "[Test Author] Old Title")
	require.NoError(t, os.MkdirAll(oldBookFolder, 0755))

	// Old-title book sidecar (the pre-enricher state).
	require.NoError(t, sidecar.WriteBookSidecar(oldBookFolder, &sidecar.BookSidecar{Title: "Old Title"}))

	// The media file inside the book folder.
	oldFilePath := filepath.Join(oldBookFolder, "Old Title.epub")
	require.NoError(t, os.WriteFile(oldFilePath, []byte("epub"), 0644))

	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Test Author",
		SortName:  "Author, Test",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "New Title",
		TitleSource:     models.DataSourcePlugin,
		SortTitle:       "New Title",
		SortTitleSource: models.DataSourcePlugin,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        oldBookFolder,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      oldFilePath,
		FilesizeBytes: 4,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	loadedBook, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	require.NoError(t, svc.OrganizeBookFiles(ctx, loadedBook))

	newBookFolder := filepath.Join(libDir, "[Test Author] New Title")
	newFilePath := filepath.Join(newBookFolder, "New Title.epub")

	assert.FileExists(t, newFilePath, "file should be renamed inside the renamed folder")
	_, err = os.Stat(oldBookFolder)
	assert.True(t, os.IsNotExist(err), "old folder should no longer exist after rename")

	// The old-named sidecar that was carried along by the folder rename
	// should be gone, and a fresh sidecar with the new folder name should
	// exist so the book is never left sidecar-less.
	oldNamedSidecarInNewFolder := filepath.Join(newBookFolder, "[Test Author] Old Title.metadata.json")
	assert.NoFileExists(t, oldNamedSidecarInNewFolder, "stale-named sidecar should be removed")
	assert.FileExists(t, sidecar.BookSidecarPath(newBookFolder), "fresh book sidecar should exist at new folder")

	reloaded, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, newBookFolder, reloaded.Filepath)
}
