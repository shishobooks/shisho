package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An .m4a is an M4B (ADR 0018): a directory holding only one becomes a Book
// whose audiobook File carries the file's metadata and chapters.
func TestProcessScanJob_M4AAloneBecomesAudiobook(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Some Book")
	path := testgen.GenerateM4B(t, dir, "Some Book.m4a", testgen.M4BOptions{
		Title:    "Some Book",
		Artist:   "Some Author",
		Composer: "Some Narrator",
		Duration: 20,
		HasCover: true,
		Chapters: []testgen.M4BChapter{
			{Title: "Chapter One", Start: 0},
			{Title: "Chapter Two", Start: 10},
		},
	})

	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]
	assert.Equal(t, "Some Book", book.Title)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Some Author", book.Authors[0].Person.Name)

	require.Len(t, book.Files, 1)
	file := book.Files[0]
	assert.Equal(t, path, file.Filepath)
	assert.Equal(t, models.FileTypeM4B, file.FileType)
	assert.Equal(t, models.FileRoleMain, file.FileRole)
	assert.NotNil(t, file.AudiobookDurationSeconds)
	assert.NotNil(t, file.CoverImageFilename)

	files := tc.listFiles()
	require.Len(t, files, 1)
	narrators := make([]string, 0, len(files[0].Narrators))
	for _, n := range files[0].Narrators {
		narrators = append(narrators, n.Person.Name)
	}
	assert.Equal(t, []string{"Some Narrator"}, narrators)

	chapterTitles := []string{}
	for _, c := range tc.listChapters(file.ID) {
		chapterTitles = append(chapterTitles, c.Title)
	}
	assert.Equal(t, []string{"Chapter One", "Chapter Two"}, chapterTitles)
	assert.Equal(t, ".m4a", filepath.Ext(file.Filepath))
}

func TestProcessScanJob_M4ABesideEPUBJoinsItsBook(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "[Some Author] Some Book")
	testgen.GenerateEPUB(t, dir, "Some Book.epub", testgen.EPUBOptions{Title: "Some Book", Authors: []string{"Some Author"}})
	m4aPath := testgen.GenerateM4B(t, dir, "Some Book.m4a", testgen.M4BOptions{Title: "Some Book", Artist: "Some Author"})

	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.Len(t, allBooks[0].Files, 2)
	byType := map[string]*models.File{}
	for _, f := range allBooks[0].Files {
		byType[f.FileType] = f
	}
	require.Contains(t, byType, models.FileTypeM4B)
	assert.Equal(t, m4aPath, byType[models.FileTypeM4B].Filepath)
	assert.Equal(t, models.FileRoleMain, byType[models.FileTypeM4B].FileRole)
	assert.Contains(t, byType, models.FileTypeEPUB)
}

func TestProcessScanJob_M4AWithoutChaptersImports(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "One Track")
	testgen.GenerateM4B(t, dir, "One Track.m4a", testgen.M4BOptions{Title: "One Track", Artist: "Some Author"})

	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileTypeM4B, files[0].FileType)
	assert.Nil(t, files[0].ScanError)
	assert.Empty(t, tc.listChapters(files[0].ID))
}

func TestProcessScanJob_MislabeledM4AIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Junk")
	path := testgen.WriteFile(t, dir, "junk.m4a", []byte("just some text, not an audiobook"))

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listBooks())
	warnings := warningsFor(logs, path)
	require.Len(t, warnings, 1)
	assert.Equal(t, "mime type is not expected for extension", warnings[0].Message)
}

// Organizing keeps a File's own extension, so an organized .m4a stays .m4a,
// and names it as an M4B, with the narrator.
func TestProcessScanJob_OrganizedM4AKeepsExtensionAndAddsNarrator(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)
	testgen.GenerateM4B(t, libraryPath, "download.m4a", testgen.M4BOptions{
		Title:    "Some Book",
		Artist:   "Some Author",
		Composer: "Some Narrator",
	})

	require.NoError(t, tc.runScan())

	folder := filepath.Join(libraryPath, "[Some Author] Some Book")
	want := filepath.Join(folder, "Some Book {Some Narrator}.m4a")
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, want, files[0].Filepath)
	assert.Equal(t, models.FileTypeM4B, files[0].FileType)
	assert.FileExists(t, want)
	assert.FileExists(t, want+".metadata.json")
}

func TestMonitor_ProcessEvent_CreateScansNewM4A(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Some Book")
	path := testgen.GenerateM4B(t, bookDir, "Some Book.m4a", testgen.M4BOptions{Title: "Monitored M4A", Artist: "Some Author"})

	tc.worker.config.LibraryMonitorDelaySeconds = config.MinLibraryMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1
	require.True(t, m.isScannable(path))

	result := m.processEvent(tc.ctx, path, pendingEvent{Op: fsnotify.Create, LibraryID: 1})
	require.NotNil(t, result)
	assert.True(t, result.FileCreated)
	require.NotNil(t, result.Book)
	assert.Equal(t, "Monitored M4A", result.Book.Title)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileTypeM4B, files[0].FileType)
}

// Shisho parses .m4a itself (ADR 0019), so a plugin file parser that
// declares m4a is not used for it, and the scan's job log names that plugin
// and the extension once, however many .m4a files the scan sees.
func TestProcessScanJob_PluginParserDeclaringM4AIsSkippedAndLogged(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "m4a-parser",
  "name": "M4A Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses m4a files",
      "types": ["m4a", "zzaudio"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Plugin Title - Should Not Happen" };
      }
    }
  };
})();`
	installTestPlugin(t, tc, pluginDir, "m4a-parser", manifest, mainJS)
	require.NoError(t, tc.worker.pluginManager.LoadAll(context.Background()))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	for _, title := range []string{"First Book", "Second Book"} {
		dir := testgen.CreateSubDir(t, libraryPath, title)
		testgen.GenerateM4B(t, dir, title+".m4a", testgen.M4BOptions{Title: title, Artist: "Some Author"})
	}

	logs := tc.runScanWithJob()

	titles := []string{}
	for _, b := range tc.listBooks() {
		titles = append(titles, b.Title)
	}
	assert.ElementsMatch(t, []string{"First Book", "Second Book"}, titles)
	for _, f := range tc.listFiles() {
		assert.Equal(t, models.FileTypeM4B, f.FileType)
	}

	var named []*models.JobLog
	for _, l := range logs {
		if l.Plugin != nil && *l.Plugin == "m4a-parser" {
			named = append(named, l)
		}
	}
	require.Len(t, named, 1)
	assert.Equal(t, models.LogLevelWarn, named[0].Level)
	require.NotNil(t, named[0].Data)
	assert.Contains(t, *named[0].Data, `"extensions":["m4a"]`)
	assert.NotContains(t, *named[0].Data, "zzaudio", "only built-in extensions are named")
}

// runM4ARetypeMigration runs the upgrade migration that retypes .m4a Files
// against the test database, which testdb has already migrated, so it calls
// the migration directly rather than through a migrator's applied list.
func (tc *testContext) runM4ARetypeMigration() {
	tc.t.Helper()
	for _, m := range migrations.Migrations.Sorted() {
		if m.Name == "20261115035343" {
			require.NoError(tc.t, m.Up(tc.ctx, migrations.NewMigrator(tc.db), &m))
			return
		}
	}
	tc.t.Fatal("the .m4a retype migration is not registered")
}

// A library upgraded from before .m4a was an M4B has .m4a Files tracked as
// supplements, typed by their extension, and as main Files a plugin parsed.
// After the migration, the next scan reads both as M4Bs with the built-in
// parser, keeping their IDs.
func TestProcessScanJob_ScanAfterM4AMigrationReadsMetadata(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	m4aOpts := func(title string) testgen.M4BOptions {
		return testgen.M4BOptions{
			Title:    title,
			Artist:   "Some Author",
			Composer: "Some Narrator",
			Duration: 20,
			HasCover: true,
			Chapters: []testgen.M4BChapter{{Title: "Chapter One", Start: 0}, {Title: "Chapter Two", Start: 10}},
		}
	}

	// An EPUB with an .m4a beside it that the old scan tracked as a
	// supplement.
	ebookDir := testgen.CreateSubDir(t, libraryPath, "[Some Author] Ebook Book")
	testgen.GenerateEPUB(t, ebookDir, "Ebook Book.epub", testgen.EPUBOptions{Title: "Ebook Book", Authors: []string{"Some Author"}})
	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	ebookBook := allBooks[0]
	supplementPath := testgen.GenerateM4B(t, ebookDir, "Ebook Book.m4a", m4aOpts("Ebook Book"))
	supplementStat, err := os.Stat(supplementPath)
	require.NoError(t, err)
	supplement := &models.File{
		LibraryID:     ebookBook.LibraryID,
		BookID:        ebookBook.ID,
		Filepath:      supplementPath,
		FileType:      "m4a",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: supplementStat.Size(),
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, supplement))

	// An .m4a alone in its directory that a plugin parser read, with its
	// size and mtime recorded, so without the migration a scan would see it
	// as unchanged.
	pluginDir := testgen.CreateSubDir(t, libraryPath, "Plugin Book")
	pluginPath := testgen.GenerateM4B(t, pluginDir, "Plugin Book.m4a", m4aOpts("Plugin Book"))
	pluginSource := models.PluginDataSource("test", "m4a-parser")
	pluginBook := &models.Book{
		LibraryID:    ebookBook.LibraryID,
		Filepath:     pluginDir,
		Title:        "Plugin Book",
		TitleSource:  pluginSource,
		SortTitle:    "Plugin Book",
		AuthorSource: pluginSource,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, pluginBook))
	stat, err := os.Stat(pluginPath)
	require.NoError(t, err)
	modTime := stat.ModTime()
	pluginParsed := &models.File{
		LibraryID:      ebookBook.LibraryID,
		BookID:         pluginBook.ID,
		Filepath:       pluginPath,
		FileType:       "m4a",
		FileRole:       models.FileRoleMain,
		FilesizeBytes:  stat.Size(),
		FileModifiedAt: &modTime,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, pluginParsed))

	tc.runM4ARetypeMigration()
	require.NoError(t, tc.runScan())

	files := map[int]*models.File{}
	for _, f := range tc.listFiles() {
		files[f.ID] = f
	}
	require.Len(t, files, 3)
	for _, c := range []struct {
		id     int
		bookID int
	}{
		{supplement.ID, ebookBook.ID},
		{pluginParsed.ID, pluginBook.ID},
	} {
		file := files[c.id]
		require.NotNil(t, file, "File %d keeps its ID", c.id)
		assert.Equal(t, c.bookID, file.BookID)
		assert.Equal(t, models.FileTypeM4B, file.FileType)
		assert.Equal(t, models.FileRoleMain, file.FileRole)
		assert.NotNil(t, file.AudiobookDurationSeconds, "File %d", c.id)
		assert.NotNil(t, file.CoverImageFilename, "File %d", c.id)
		narrators := []string{}
		for _, n := range file.Narrators {
			narrators = append(narrators, n.Person.Name)
		}
		assert.Equal(t, []string{"Some Narrator"}, narrators, "File %d", c.id)
		assert.Len(t, tc.listChapters(c.id), 2, "File %d", c.id)
	}
}
