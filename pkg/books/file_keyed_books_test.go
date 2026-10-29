package books

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Books split out with organizing off are keyed by their file. These tests
// cover what happens to such a key as files move, get renamed, or are deleted.

// newUnorganizedFixture is an organizeFixture whose library has organizing off.
func newUnorganizedFixture(t *testing.T) *organizeFixture {
	t.Helper()
	f := newOrganizeFixture(t)
	f.library.OrganizeFileStructure = false
	_, err := f.db.NewUpdate().Model(f.library).Column("organize_file_structure").WherePK().Exec(f.ctx)
	require.NoError(t, err)
	return f
}

func (f *organizeFixture) moveFiles(target *int, files ...*models.File) (*MoveFilesResult, error) {
	f.t.Helper()
	ids := make([]int, len(files))
	for i, file := range files {
		ids[i] = file.ID
	}
	return f.svc.MoveFilesToBook(f.ctx, MoveFilesOptions{FileIDs: ids, TargetBookID: target, LibraryID: f.library.ID})
}

// Splitting a file out, moving another file in, and splitting the first file
// out again must not collide with the Book the first split created.
func TestMoveFilesToBook_ResplitFileKeyingItsBook(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	a := filepath.Join(f.libDir, "Dune (1965).epub")
	b := filepath.Join(f.libDir, "Dune (2021).epub")
	source := f.addBook("Dune", filepath.Join(f.libDir, "[Emily Henry] Dune"))
	fileA := f.addFile(source, a, true)
	fileB := f.addFile(source, b, true)

	split, err := f.moveFiles(nil, fileA)
	require.NoError(t, err)
	require.Equal(t, a, split.TargetBook.Filepath)

	_, err = f.moveFiles(&split.TargetBook.ID, fileB)
	require.NoError(t, err)

	resplit, err := f.moveFiles(nil, f.reloadFile(fileA))
	require.NoError(t, err)
	assert.Equal(t, a, resplit.TargetBook.Filepath)
	assert.Equal(t, b, f.reloadBook(split.TargetBook).Filepath, "the Book left behind is keyed by its remaining file")
}

// A root-level Book keyed by its only file has no other key to take, so
// moving that file into a new Book is refused rather than colliding.
func TestMoveFilesToBook_OnlyFileOfFileKeyedBookToNewBook(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	a := filepath.Join(f.libDir, "a.epub")
	book := f.addBook("A", a)
	file := f.addFile(book, a, true)

	_, err := f.moveFiles(nil, file)
	require.ErrorIs(t, err, ErrNothingToSplit)
	assert.Equal(t, book.ID, f.reloadFile(file).BookID)
}

// A new Book is not keyed by a folder that holds another Book's files.
func TestMoveFilesToBook_NewBookSkipsFolderHoldingOtherBooksFiles(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	a := filepath.Join(folder, "a.epub")
	b := filepath.Join(folder, "b.epub")
	c := filepath.Join(folder, "c.epub")
	// No Book is keyed by the folder: its files belong to file-keyed Books.
	bookA := f.addBook("A", a)
	f.addFile(bookA, a, true)
	bookB := f.addBook("B", b)
	f.addFile(bookB, b, true)
	fileC := f.addFile(bookB, c, true)

	result, err := f.moveFiles(nil, fileC)
	require.NoError(t, err)
	assert.Equal(t, c, result.TargetBook.Filepath)
}

// Deleting a file a Book is keyed by rekeys the Book to a remaining file.
func TestDeleteFileAndCleanup_RekeysBookKeyedByDeletedFile(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	a := filepath.Join(f.libDir, "Folder", "a.epub")
	b := filepath.Join(f.libDir, "Folder", "b.epub")
	book := f.addBook("A", a)
	fileA := f.addFile(book, a, true)
	f.addFile(book, b, true)

	_, err := f.svc.DeleteFileAndCleanup(f.ctx, fileA.ID, f.library, map[string]struct{}{models.FileTypeEPUB: {}})
	require.NoError(t, err)
	assert.Equal(t, b, f.reloadBook(book).Filepath)
}

// With organizing on, moving a file into a Book keyed by its file puts the
// file in that Book's folder.
func TestMoveFilesToBook_Organized_IntoFileKeyedBook(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	b := filepath.Join(folder, "b.epub")
	target := f.addBook("B", b)
	f.addFile(target, b, true)
	other := filepath.Join(f.libDir, "Other", "c.epub")
	source := f.addBook("C", filepath.Join(f.libDir, "Other"))
	fileC := f.addFile(source, other, true)

	_, err := f.moveFiles(&target.ID, fileC)
	require.NoError(t, err)
	moved := filepath.Join(folder, "c.epub")
	assert.Equal(t, moved, f.reloadFile(fileC).Filepath)
	assert.FileExists(t, moved)
}

// Deleting a folder's Book with organizing on keeps the files of a Book
// split out of that folder.
func TestDeleteBookAndFiles_Organized_KeepsFilesOfBooksInFolder(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	a := filepath.Join(folder, "a.epub")
	b := filepath.Join(folder, "b.epub")
	folderBook := f.addBook("A", folder)
	f.addFile(folderBook, a, true)
	split := f.addBook("B", b)
	f.addFile(split, b, true)

	_, err := f.svc.DeleteBookAndFiles(f.ctx, folderBook.ID, f.library)
	require.NoError(t, err)
	assert.NoFileExists(t, a)
	assert.FileExists(t, b, "the split Book's file survives")
}

// A library path stored with a trailing slash is still a library root.
func TestDeleteBookAndFiles_Organized_LibraryRootWithTrailingSlash(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	_, err := f.db.NewUpdate().Model((*models.LibraryPath)(nil)).
		Set("filepath = ?", f.libDir+string(os.PathSeparator)).
		Where("library_id = ?", f.library.ID).
		Exec(f.ctx)
	require.NoError(t, err)

	other := filepath.Join(f.libDir, "Other.epub")
	require.NoError(t, os.WriteFile(other, []byte("other"), 0600))
	a := filepath.Join(f.libDir, "a.epub")
	book := f.addBook("A", f.libDir)
	f.addFile(book, a, true)

	_, err = f.svc.DeleteBookAndFiles(f.ctx, book.ID, f.library)
	require.NoError(t, err)
	assert.NoFileExists(t, a)
	assert.FileExists(t, other, "the library root survives")
}

// With organizing off, deleting a folder's Book removes its book sidecar.
func TestDeleteBookAndFiles_Unorganized_RemovesFolderBookSidecar(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	book := f.addBook("A", folder)
	f.addFile(book, filepath.Join(folder, "a.epub"), true)
	bookSidecar := filepath.Join(folder, "Folder.metadata.json")
	require.NoError(t, os.WriteFile(bookSidecar, []byte("{}"), 0600))

	_, err := f.svc.DeleteBookAndFiles(f.ctx, book.ID, f.library)
	require.NoError(t, err)
	assert.NoFileExists(t, bookSidecar)
}

// Deleting a Book removes only the book sidecar it writes: the one beside
// its first main file, not one that shares another main file's basename.
func TestDeleteBookAndFiles_KeepsOtherBooksSidecarSharingABasename(t *testing.T) {
	t.Parallel()
	f := newUnorganizedFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	a := filepath.Join(folder, "a.epub")
	bEpub := filepath.Join(folder, "b.epub")
	bPDF := filepath.Join(folder, "b.pdf")
	book := f.addBook("A", a)
	f.addFile(book, a, true)
	f.addFile(book, bEpub, true)
	other := f.addBook("B", bPDF)
	f.addFile(other, bPDF, true)
	require.NoError(t, os.WriteFile(bookSidecarFor(a), []byte("{}"), 0600))
	require.NoError(t, os.WriteFile(bookSidecarFor(bPDF), []byte("{}"), 0600))

	_, err := f.svc.DeleteBookAndFiles(f.ctx, book.ID, f.library)
	require.NoError(t, err)
	assert.NoFileExists(t, bookSidecarFor(a))
	assert.FileExists(t, bookSidecarFor(bPDF), "the other Book's sidecar survives")
}

// Organizing a Book keyed by a library root moves its files into a folder
// instead of renaming the library root.
func TestOrganizeBookFiles_BookKeyedByLibraryRoot(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	book := f.addBook("Beach Read", f.libDir)
	fileA := f.addFile(book, filepath.Join(f.libDir, "a.epub"), true)
	f.addFile(book, filepath.Join(f.libDir, "b.epub"), true)

	f.organize(book)

	assert.DirExists(t, f.libDir)
	folder := filepath.Join(f.libDir, "[Emily Henry] Beach Read")
	assert.Equal(t, folder, f.reloadBook(book).Filepath)
	assert.Equal(t, folder, filepath.Dir(f.reloadFile(fileA).Filepath))
}

// A rename that leaves book sidecars alone still carries the sidecar of a
// Book keyed by the renamed file.
func TestRecordOrganizedFilepath_MovesSidecarOfFileKeyedBook(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "Folder")
	oldPath := filepath.Join(folder, "download.epub")
	newPath := filepath.Join(folder, "Beach Read.epub")
	book := f.addBook("Beach Read", oldPath)
	file := f.addFile(book, oldPath, true)
	require.NoError(t, os.WriteFile(bookSidecarFor(oldPath), []byte("{}"), 0600))
	require.NoError(t, os.Rename(oldPath, newPath))

	require.NoError(t, f.svc.RecordOrganizedFilepath(f.ctx, file, oldPath, newPath, false))
	assert.Equal(t, newPath, f.reloadBook(book).Filepath)
	assert.FileExists(t, bookSidecarFor(newPath))
	assert.NoFileExists(t, bookSidecarFor(oldPath))
}
