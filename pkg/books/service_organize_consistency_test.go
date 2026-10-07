package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// organizeFixture is a library with organizing on, one author, and helpers to
// add books and files, for tests that check disk and database stay in step.
type organizeFixture struct {
	t       *testing.T
	ctx     context.Context
	db      *bun.DB
	svc     *Service
	libDir  string
	library *models.Library
	person  *models.Person
}

func newOrganizeFixture(t *testing.T) *organizeFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	f := &organizeFixture{t: t, ctx: ctx, db: db, svc: NewService(db, appsettings.NewService(db)), libDir: t.TempDir()}

	f.library = &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(f.library).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: f.library.ID, Filepath: f.libDir}).Exec(ctx)
	require.NoError(t, err)

	f.person = &models.Person{LibraryID: f.library.ID, Name: "Emily Henry", SortName: "Henry, Emily"}
	_, err = db.NewInsert().Model(f.person).Exec(ctx)
	require.NoError(t, err)
	return f
}

func (f *organizeFixture) addBook(title, bookPath string) *models.Book {
	f.t.Helper()
	book := &models.Book{
		LibraryID:       f.library.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookPath,
	}
	_, err := f.db.NewInsert().Model(book).Exec(f.ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: f.person.ID, SortOrder: 1}).Exec(f.ctx)
	require.NoError(f.t, err)
	return book
}

// addFile inserts a files row and, when onDisk is set, writes the file.
func (f *organizeFixture) addFile(book *models.Book, path string, onDisk bool) *models.File {
	f.t.Helper()
	if onDisk {
		require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(f.t, os.WriteFile(path, []byte("epub"), 0600))
	}
	file := &models.File{
		LibraryID:     f.library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      path,
		FilesizeBytes: 4,
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

func (f *organizeFixture) organize(book *models.Book) {
	f.t.Helper()
	loaded, err := f.svc.RetrieveBook(f.ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(f.t, err)
	require.NoError(f.t, f.svc.OrganizeBookFiles(f.ctx, loaded))
}

func (f *organizeFixture) reloadFile(file *models.File) *models.File {
	f.t.Helper()
	reloaded, err := f.svc.RetrieveFile(f.ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(f.t, err)
	return reloaded
}

func (f *organizeFixture) reloadBook(book *models.Book) *models.Book {
	f.t.Helper()
	reloaded, err := f.svc.RetrieveBook(f.ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(f.t, err)
	return reloaded
}

// failFilepathUpdates makes every UPDATE of files.filepath fail, standing in
// for a database error after the file has moved on disk.
func (f *organizeFixture) failFilepathUpdates() {
	f.t.Helper()
	_, err := f.db.ExecContext(f.ctx, `CREATE TRIGGER fail_filepath_update BEFORE UPDATE OF filepath ON files
		BEGIN SELECT RAISE(ABORT, 'forced filepath update failure'); END`)
	require.NoError(f.t, err)
}

// The reported failure: a files row left behind by a deleted Book still
// claims the organized destination, which is free on disk.
func TestOrganizeBookFiles_RootLevel_SkipsPathClaimedByAnotherFile(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "[Emily Henry] Beach Read")
	claimedPath := filepath.Join(folder, "Beach Read.epub")
	other := f.addBook("Old Beach Read", filepath.Join(f.libDir, "elsewhere"))
	f.addFile(other, claimedPath, false)

	book := f.addBook("Beach Read", folder)
	original := filepath.Join(f.libDir, "Beach Read (z-library).epub")
	file := f.addFile(book, original, true)

	f.organize(book)

	reloaded := f.reloadFile(file)
	assert.Equal(t, filepath.Join(folder, "Beach Read (1).epub"), reloaded.Filepath)
	assert.FileExists(t, reloaded.Filepath, "the database row points where the file is")
	assert.NoFileExists(t, original)
	assert.NoFileExists(t, claimedPath)
}

func TestOrganizeBookFiles_RootLevel_UndoesMoveWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "[Emily Henry] Beach Read")
	book := f.addBook("Beach Read", folder)
	original := filepath.Join(f.libDir, "download.epub")
	file := f.addFile(book, original, true)
	require.NoError(t, os.WriteFile(original+".cover.jpg", []byte("cover"), 0600))
	f.failFilepathUpdates()

	f.organize(book)

	assert.FileExists(t, original, "the file is moved back")
	assert.FileExists(t, original+".cover.jpg", "its cover is moved back")
	assert.NoDirExists(t, folder, "the folder organize created is removed")
	assert.Equal(t, original, f.reloadFile(file).Filepath)
	assert.Equal(t, folder, f.reloadBook(book).Filepath, "the book keeps its path")
}

func TestOrganizeBookFiles_RenameInPlace_SkipsClaimedPathAndUndoesOnFailure(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	// Files in a subfolder that is not the book's folder are renamed in place.
	subfolder := filepath.Join(f.libDir, "Downloads")
	book := f.addBook("Beach Read", filepath.Join(f.libDir, "[Emily Henry] Beach Read"))
	original := filepath.Join(subfolder, "download.epub")
	file := f.addFile(book, original, true)
	other := f.addBook("Other", filepath.Join(f.libDir, "other"))
	f.addFile(other, filepath.Join(subfolder, "Beach Read.epub"), false)

	f.organize(book)

	renamed := f.reloadFile(file)
	assert.Equal(t, filepath.Join(subfolder, "Beach Read (1).epub"), renamed.Filepath)
	assert.FileExists(t, renamed.Filepath)

	// Rename it again with the update failing.
	_, err := f.db.NewUpdate().Model((*models.Book)(nil)).Set("title = ?", "Book Lovers").Where("id = ?", book.ID).Exec(f.ctx)
	require.NoError(t, err)
	f.failFilepathUpdates()

	f.organize(book)

	assert.FileExists(t, renamed.Filepath, "the file is moved back")
	assert.NoFileExists(t, filepath.Join(subfolder, "Book Lovers.epub"))
	assert.Equal(t, renamed.Filepath, f.reloadFile(file).Filepath)
}

func TestOrganizeBookFiles_DirectoryBased_SkipsFolderHoldingAnotherBooksFiles(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	claimedFolder := filepath.Join(f.libDir, "[Emily Henry] Beach Read")
	other := f.addBook("Old Beach Read", claimedFolder)
	f.addFile(other, filepath.Join(claimedFolder, "Beach Read.epub"), false)

	oldFolder := filepath.Join(f.libDir, "beach read download")
	book := f.addBook("Beach Read", oldFolder)
	file := f.addFile(book, filepath.Join(oldFolder, "Beach Read.epub"), true)

	f.organize(book)

	newFolder := filepath.Join(f.libDir, "[Emily Henry] Beach Read (1)")
	assert.Equal(t, newFolder, f.reloadBook(book).Filepath)
	reloaded := f.reloadFile(file)
	assert.Equal(t, filepath.Join(newFolder, "Beach Read.epub"), reloaded.Filepath)
	assert.FileExists(t, reloaded.Filepath)
	assert.NoDirExists(t, claimedFolder)
}

func TestOrganizeBookFiles_DirectoryBased_UndoesFolderRenameWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	oldFolder := filepath.Join(f.libDir, "beach read download")
	book := f.addBook("Beach Read", oldFolder)
	original := filepath.Join(oldFolder, "Beach Read.epub")
	file := f.addFile(book, original, true)
	f.failFilepathUpdates()

	loaded, err := f.svc.RetrieveBook(f.ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Error(t, f.svc.OrganizeBookFiles(f.ctx, loaded))

	assert.FileExists(t, original, "the folder is renamed back")
	assert.NoDirExists(t, filepath.Join(f.libDir, "[Emily Henry] Beach Read"))
	assert.Equal(t, oldFolder, f.reloadBook(book).Filepath)
	assert.Equal(t, original, f.reloadFile(file).Filepath)
}

func TestOrganizeBookFiles_DirectoryBased_UndoesPromotionWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)

	folder := filepath.Join(f.libDir, "[Emily Henry] Beach Read")
	book := f.addBook("Beach Read", folder)
	f.addFile(book, filepath.Join(folder, "Beach Read.epub"), true)
	rootFile := f.addFile(book, filepath.Join(f.libDir, "extra.epub"), true)
	f.failFilepathUpdates()

	f.organize(book)

	assert.FileExists(t, rootFile.Filepath, "the promoted file is moved back to the library root")
	assert.NoFileExists(t, filepath.Join(folder, "Beach Read (1).epub"))
	assert.Equal(t, rootFile.Filepath, f.reloadFile(rootFile).Filepath)
	assert.FileExists(t, filepath.Join(folder, "Beach Read.epub"), "the folder and its other file are untouched")
}

// nestedLayout is Book A in a folder named oldName with a file of its own, and
// Book B nested inside A's folder at extras/b.epub, both on disk. Organizing A
// renames its folder to "[Emily Henry] Beach Read".
type nestedLayout struct {
	oldFolder, newFolder string
	a, b                 *models.Book
	aFile, bFile         *models.File
}

func (f *organizeFixture) addNestedLayout(oldName string) nestedLayout {
	f.t.Helper()
	l := nestedLayout{
		oldFolder: filepath.Join(f.libDir, oldName),
		newFolder: filepath.Join(f.libDir, "[Emily Henry] Beach Read"),
	}
	l.a = f.addBook("Beach Read", l.oldFolder)
	l.aFile = f.addFile(l.a, filepath.Join(l.oldFolder, "Beach Read.epub"), true)
	l.b = f.addBook("Book Lovers", filepath.Join(l.oldFolder, "extras"))
	l.bFile = f.addFile(l.b, filepath.Join(l.oldFolder, "extras", "b.epub"), true)
	return l
}

func TestOrganizeBookFiles_DirectoryBased_RewritesNestedBooks(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")

	f.organize(l.a)

	assert.Equal(t, l.newFolder, f.reloadBook(l.a).Filepath)
	assert.Equal(t, filepath.Join(l.newFolder, "Beach Read.epub"), f.reloadFile(l.aFile).Filepath)
	assert.Equal(t, filepath.Join(l.newFolder, "extras"), f.reloadBook(l.b).Filepath)
	nested := f.reloadFile(l.bFile)
	assert.Equal(t, filepath.Join(l.newFolder, "extras", "b.epub"), nested.Filepath)
	assert.FileExists(t, nested.Filepath, "the nested Book's row points where its file is")
}

func TestOrganizeBookFiles_DirectoryBased_LeavesSiblingFolderSharingNamePrefix(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")
	// " 2" sorts just before the separator and "0" just after it.
	var siblings []*models.File
	for _, suffix := range []string{" 2", "0"} {
		siblingFolder := l.oldFolder + suffix
		sibling := f.addBook("Old Title"+suffix, siblingFolder)
		siblings = append(siblings, f.addFile(sibling, filepath.Join(siblingFolder, "d.epub"), true))
	}

	f.organize(l.a)

	for _, sibling := range siblings {
		assert.Equal(t, filepath.Dir(sibling.Filepath), f.reloadBook(&models.Book{ID: sibling.BookID}).Filepath)
		assert.Equal(t, sibling.Filepath, f.reloadFile(sibling).Filepath)
		assert.FileExists(t, sibling.Filepath)
	}
}

// The old folder name is matched exactly: LIKE wildcards in it match only
// themselves, and a folder differing only in case is a different folder.
func TestOrganizeBookFiles_DirectoryBased_MatchesFolderNameLiterally(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old_Title 100%")
	wildcardFolder := filepath.Join(f.libDir, "[Emily Henry] OldXTitle 100 percent")
	wildcardMatch := f.addBook("Wildcard", wildcardFolder)
	wildcardFile := f.addFile(wildcardMatch, filepath.Join(wildcardFolder, "x.epub"), true)
	// Rows only: a case-insensitive file system cannot hold both folders.
	caseFolder := filepath.Join(f.libDir, "[emily henry] old_title 100%")
	caseMatch := f.addBook("Case", caseFolder)
	caseFile := f.addFile(caseMatch, filepath.Join(caseFolder, "y.epub"), false)

	f.organize(l.a)

	assert.Equal(t, filepath.Join(l.newFolder, "extras", "b.epub"), f.reloadFile(l.bFile).Filepath)
	assert.Equal(t, wildcardFolder, f.reloadBook(wildcardMatch).Filepath)
	assert.Equal(t, wildcardFile.Filepath, f.reloadFile(wildcardFile).Filepath)
	assert.Equal(t, caseFolder, f.reloadBook(caseMatch).Filepath)
	assert.Equal(t, caseFile.Filepath, f.reloadFile(caseFile).Filepath)
}

func TestOrganizeBookFiles_DirectoryBased_LeavesNestedBooksWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")
	f.failFilepathUpdates()

	loaded, err := f.svc.RetrieveBook(f.ctx, RetrieveBookOptions{ID: &l.a.ID})
	require.NoError(t, err)
	require.Error(t, f.svc.OrganizeBookFiles(f.ctx, loaded))

	assert.FileExists(t, l.bFile.Filepath, "the folder is renamed back")
	assert.NoDirExists(t, l.newFolder)
	assert.Equal(t, l.oldFolder, f.reloadBook(l.a).Filepath)
	assert.Equal(t, l.aFile.Filepath, f.reloadFile(l.aFile).Filepath)
	assert.Equal(t, l.b.Filepath, f.reloadBook(l.b).Filepath)
	assert.Equal(t, l.bFile.Filepath, f.reloadFile(l.bFile).Filepath)
}

// Library paths can overlap, and the rename moves everything under the folder
// whichever library a row belongs to.
func TestOrganizeBookFiles_DirectoryBased_RewritesNestedBooksInOtherLibraries(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")
	other := &models.Library{
		Name:                     "Other Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := f.db.NewInsert().Model(other).Exec(f.ctx)
	require.NoError(t, err)
	otherBook := f.addBook("Other", filepath.Join(l.oldFolder, "other"))
	otherFile := f.addFile(otherBook, filepath.Join(l.oldFolder, "other", "c.epub"), true)
	_, err = f.db.NewUpdate().Model((*models.Book)(nil)).Set("library_id = ?", other.ID).Where("id = ?", otherBook.ID).Exec(f.ctx)
	require.NoError(t, err)
	_, err = f.db.NewUpdate().Model((*models.File)(nil)).Set("library_id = ?", other.ID).Where("id = ?", otherFile.ID).Exec(f.ctx)
	require.NoError(t, err)

	f.organize(l.a)

	assert.Equal(t, filepath.Join(l.newFolder, "other"), f.reloadBook(otherBook).Filepath)
	reloaded := f.reloadFile(otherFile)
	assert.Equal(t, filepath.Join(l.newFolder, "other", "c.epub"), reloaded.Filepath)
	assert.FileExists(t, reloaded.Filepath)
}

func TestOrganizeBookFiles_DirectoryBased_ReindexesNestedBooks(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")
	require.NoError(t, search.NewService(f.db).ReindexBookByID(f.ctx, l.b.ID))

	f.organize(l.a)

	var row struct {
		Filepath  string
		Filenames string
	}
	require.NoError(t, f.db.NewRaw(`SELECT filepath, filenames FROM books_fts WHERE rowid = ?`, l.b.ID).Scan(f.ctx, &row))
	assert.Equal(t, filepath.Join(l.newFolder, "extras"), row.Filepath)
	assert.Equal(t, filepath.Join(l.newFolder, "extras", "b.epub"), row.Filenames)
}

// A Book whose row is not at the folder it was organized from fails the
// record, so organize renames the folder back instead of leaving the
// database behind.
func TestRecordRenamedBookFolder_FailsWhenBookIsNotRecordedAtOldFolder(t *testing.T) {
	t.Parallel()
	f := newOrganizeFixture(t)
	l := f.addNestedLayout("[Emily Henry] Old Title")
	stale := *l.a
	stale.Filepath = filepath.Join(f.libDir, "elsewhere")
	staleFolder := stale.Filepath
	staleFile := f.addFile(l.b, filepath.Join(staleFolder, "s.epub"), false)

	_, err := f.svc.recordRenamedBookFolder(f.ctx, &stale, nil, l.newFolder, time.Now())

	require.Error(t, err)
	assert.Equal(t, staleFolder, stale.Filepath, "the in-memory Book keeps its path")
	assert.Equal(t, staleFile.Filepath, f.reloadFile(staleFile).Filepath, "nothing is rewritten")
}
