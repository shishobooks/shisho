package worker

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullMOBIOptions(kind testgen.MOBIKind, title string) testgen.MOBIOptions {
	return testgen.MOBIOptions{
		Kind:           kind,
		Title:          title,
		Authors:        []string{"Doe, Jane"},
		Description:    "<p>A <b>fine</b> story.</p>",
		Publisher:      "Big Pub",
		PublishingDate: "2021-03-04T06:00:00+00:00",
		Language:       "fr",
		ISBN:           "9780306406157",
		ASIN:           "B00ABCDEFG",
		Subjects:       []string{"Fantasy;Adventure"},
		HasCover:       true,
	}
}

// runScanWithJob runs a scan under a real job and returns its log entries.
func (tc *testContext) runScanWithJob() []*models.JobLog {
	tc.t.Helper()
	job := &models.Job{Type: models.JobTypeScan, Status: models.JobStatusInProgress, DataParsed: &models.JobScanData{}}
	require.NoError(tc.t, tc.jobService.CreateJob(tc.ctx, job))
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, job.ID, logger.FromContext(tc.ctx))
	require.NoError(tc.t, tc.worker.ProcessScanJob(tc.ctx, job, jobLog))
	logs, err := tc.jobLogService.ListJobLogs(tc.ctx, joblogs.ListJobLogsOptions{JobID: job.ID})
	require.NoError(tc.t, err)
	return logs
}

// warningsFor returns the job-log warnings whose data names path.
func warningsFor(logs []*models.JobLog, path string) []*models.JobLog {
	var result []*models.JobLog
	for _, l := range logs {
		if l.Level == models.LogLevelWarn && l.Data != nil && strings.Contains(*l.Data, path) {
			result = append(result, l)
		}
	}
	return result
}

func TestProcessScanJob_MOBIAndAZW3(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	cases := []struct {
		dir, filename string
		kind          testgen.MOBIKind
		wantType      string
	}{
		{"MOBI6 Book", "book.mobi", testgen.MOBIKindMOBI6, models.FileTypeMOBI},
		{"KF8 Book", "book.azw3", testgen.MOBIKindKF8, models.FileTypeAZW3},
		{"AZW Book", "book.azw", testgen.MOBIKindMOBI6, models.FileTypeMOBI},
		{"PRC Book", "book.prc", testgen.MOBIKindMOBI6, models.FileTypeMOBI},
		{"Combo Book", "book.mobi", testgen.MOBIKindCombo, models.FileTypeMOBI},
	}
	for _, c := range cases {
		dir := testgen.CreateSubDir(t, libraryPath, c.dir)
		testgen.GenerateMOBI(t, dir, c.filename, fullMOBIOptions(c.kind, c.dir+" Title"))
	}

	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, len(cases))
	byTitle := make(map[string]*models.Book)
	for _, b := range allBooks {
		byTitle[b.Title] = b
	}

	for _, c := range cases {
		book := byTitle[c.dir+" Title"]
		require.NotNil(t, book, c.dir)
		assert.Equal(t, models.DataSourceMOBIMetadata, book.TitleSource, c.dir)

		require.Len(t, book.Authors, 1, c.dir)
		assert.Equal(t, "Jane Doe", book.Authors[0].Person.Name, c.dir)
		require.NotNil(t, book.Description, c.dir)
		assert.Equal(t, "A fine story.", *book.Description, c.dir)

		genreNames := make([]string, 0, len(book.BookGenres))
		for _, bg := range book.BookGenres {
			genreNames = append(genreNames, bg.Genre.Name)
		}
		sort.Strings(genreNames)
		assert.Equal(t, []string{"Adventure", "Fantasy"}, genreNames, c.dir)

		require.Len(t, book.Files, 1, c.dir)
		file := book.Files[0]
		assert.Equal(t, c.wantType, file.FileType, c.dir)
		assert.Equal(t, c.filename, filepath.Base(file.Filepath), c.dir)
		require.NotNil(t, file.Publisher, c.dir)
		assert.Equal(t, "Big Pub", file.Publisher.Name, c.dir)
		require.NotNil(t, file.ReleaseDate, c.dir)
		assert.Equal(t, 2021, file.ReleaseDate.Year(), c.dir)
		require.NotNil(t, file.Language, c.dir)
		assert.Equal(t, "fr", *file.Language, c.dir)
		require.NotNil(t, file.CoverImageFilename, c.dir)
		assert.FileExists(t, covers.FileCoverPath(file), c.dir)

		ids := make(map[string]string)
		for _, id := range file.Identifiers {
			ids[id.Type] = id.Value
		}
		assert.Equal(t, map[string]string{"isbn_13": "9780306406157", "asin": "B00ABCDEFG"}, ids, c.dir)
	}
}

func TestProcessScanJob_MOBIExtensionWithOtherContentIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Not A Book")
	path := testgen.WriteFile(t, dir, "fake.azw3", []byte("just some text, not a MOBI file"))

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listBooks())
	warnings := warningsFor(logs, path)
	require.Len(t, warnings, 1)
	assert.Equal(t, "mime type is not expected for extension", warnings[0].Message)
}

func TestProcessScanJob_DRMProtectedMOBIIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	var paths []string
	for _, c := range []struct {
		dir, filename string
		kind          testgen.MOBIKind
	}{
		{"Locked MOBI6", "book.mobi", testgen.MOBIKindMOBI6},
		{"Locked KF8", "book.azw3", testgen.MOBIKindKF8},
		{"Locked Combo", "book.mobi", testgen.MOBIKindCombo},
	} {
		opts := fullMOBIOptions(c.kind, c.dir)
		opts.Encrypted = true
		dir := testgen.CreateSubDir(t, libraryPath, c.dir)
		paths = append(paths, testgen.GenerateMOBI(t, dir, c.filename, opts))
	}

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
	for _, path := range paths {
		warnings := warningsFor(logs, path)
		require.Len(t, warnings, 1, path)
		assert.Equal(t, "skipped DRM-protected file", warnings[0].Message)
		assert.Contains(t, *warnings[0].Data, "DRM-protected")
	}
}

func TestProcessScanJob_FileReplacedByDRMCopyIsRemoved(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Zzlocked Book")
	path := testgen.GenerateMOBI(t, dir, "book.azw3", fullMOBIOptions(testgen.MOBIKindKF8, "Zzlocked Book"))

	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "precondition: the DRM-free file was imported")
	libID := allBooks[0].LibraryID
	require.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(libID, "Zzlocked"), "precondition: the Book is searchable")

	opts := fullMOBIOptions(testgen.MOBIKindKF8, "Zzlocked Book")
	opts.Encrypted = true
	require.NoError(t, os.WriteFile(path, testgen.BuildMOBI(t, opts), 0o644))
	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listFiles(), "the file record is removed")
	assert.Empty(t, tc.listBooks(), "the Book had no other file")
	assert.Empty(t, tc.searchBookIDs(libID, "Zzlocked"))
	warnings := warningsFor(logs, path)
	require.NotEmpty(t, warnings)
	assert.Equal(t, "skipped DRM-protected file", warnings[0].Message)
}

func TestResync_FileReplacedByDRMCopyIsRemoved(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Twin Book")
	testgen.GenerateEPUB(t, dir, "first.epub", testgen.EPUBOptions{Title: "Twin Book", Authors: []string{"Author"}})
	path := testgen.GenerateMOBI(t, dir, "zzlocked.mobi", testgen.MOBIOptions{Title: "Twin Book", Authors: []string{"Author"}})
	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "precondition: both Files belong to one Book")
	libID := allBooks[0].LibraryID
	require.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(libID, "zzlocked"), "precondition: the MOBI's path matches the Book")

	var mobiFile *models.File
	for _, f := range tc.listFiles() {
		if f.Filepath == path {
			mobiFile = f
		}
	}
	require.NotNil(t, mobiFile)
	require.NoError(t, os.WriteFile(path, testgen.BuildMOBI(t, testgen.MOBIOptions{Title: "Twin Book", Encrypted: true}), 0o644))

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: mobiFile.ID, ForceRefresh: true}, nil)
	require.NoError(t, err)
	require.True(t, result.FileDeleted)
	assert.False(t, result.BookDeleted, "the Book keeps its EPUB")

	require.Len(t, tc.listFiles(), 1)
	assert.Equal(t, models.FileTypeEPUB, tc.listFiles()[0].FileType)
	assert.Empty(t, tc.searchBookIDs(libID, "zzlocked"), "the removed File's path no longer matches the Book")
}

func TestScanWithPluginInputConverter_ReceivesDRMProtectedAZW3(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "unlock-converter",
  "name": "Unlock Converter",
  "version": "1.0.0",
  "capabilities": {
    "inputConverter": {
      "description": "Converts azw3 to epub",
      "sourceTypes": ["azw3"],
      "mimeTypes": ["application/x-mobipocket-ebook"],
      "targetType": "epub"
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(ctx) {
        var targetPath = ctx.targetDir + "/unlocked.epub";
        shisho.fs.writeTextFile(targetPath, "converted from " + ctx.sourcePath);
        return { success: true, targetPath: targetPath };
      }
    }
  };
})();`
	installTestPlugin(t, tc, pluginDir, "unlock-converter", manifest, mainJS)
	require.NoError(t, plugins.NewService(tc.db).AppendToOrder(context.Background(), models.PluginHookInputConverter, "test", "unlock-converter"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(context.Background()))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Locked Book")
	opts := fullMOBIOptions(testgen.MOBIKindKF8, "Locked Book")
	opts.Encrypted = true
	testgen.GenerateMOBI(t, dir, "book.azw3", opts)

	require.NoError(t, tc.runScan())

	assert.FileExists(t, filepath.Join(dir, "unlocked.epub"), "the converter ran on the DRM-protected file")
	for _, f := range tc.listFiles() {
		assert.NotEqual(t, models.FileTypeAZW3, f.FileType, "the DRM-protected file is never recorded")
	}
}

func TestProcessScanJob_AZW3CoverBeatsMOBI(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Two Formats")
	testgen.GenerateMOBI(t, dir, "a.mobi", testgen.MOBIOptions{Title: "Two Formats", HasCover: true})
	testgen.GenerateMOBI(t, dir, "b.azw3", testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "Two Formats", HasCover: true})

	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.Len(t, allBooks[0].Files, 2)
	selected := covers.SelectFile(allBooks[0].Files, "book")
	require.NotNil(t, selected)
	assert.Equal(t, models.FileTypeAZW3, selected.FileType)
}

func TestCheckExpectedMimeType_MOBIExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"a.mobi", "b.azw", "c.prc", "d.AZW3"} {
		path := testgen.GenerateMOBI(t, dir, name, testgen.MOBIOptions{Title: "Book"})
		_, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok, name)
	}

	path := testgen.WriteFile(t, dir, "e.mobi", []byte("plain text"))
	_, ok, err := checkExpectedMimeType(path)
	require.NoError(t, err)
	assert.False(t, ok)
}

// A MOBI supplement is main-eligible, so it is promoted when the Book's last
// main file disappears.
func TestScanFileByID_MissingLastMainFile_PromotesMOBISupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "[Author] Promoted")
	epubPath := testgen.GenerateEPUB(t, dir, "main.epub", testgen.EPUBOptions{Title: "Promoted", Authors: []string{"Author"}})
	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	mainFile := allBooks[0].Files[0]

	suppPath := testgen.GenerateMOBI(t, dir, "copy.azw", testgen.MOBIOptions{Title: "Promoted"})
	supplement := &models.File{
		LibraryID:     allBooks[0].LibraryID,
		BookID:        allBooks[0].ID,
		Filepath:      suppPath,
		FileType:      models.FileTypeMOBI,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 100,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, supplement))
	require.NoError(t, os.Remove(epubPath))

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: mainFile.ID}, nil)
	require.NoError(t, err)
	assert.True(t, result.FileDeleted)
	assert.False(t, result.BookDeleted)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, supplement.ID, files[0].ID)
	assert.Equal(t, models.FileRoleMain, files[0].FileRole)
}

// The scanner's extension list and models' extension-to-type map must name
// the same extensions, or a format is half supported.
func TestExtensionsToScanMatchesBuiltInFileExtensions(t *testing.T) {
	t.Parallel()

	var scanned []string
	for ext := range extensionsToScan {
		scanned = append(scanned, strings.TrimPrefix(ext, "."))
	}
	sort.Strings(scanned)
	assert.Equal(t, models.BuiltInFileExtensions(), scanned)
}

// A resync of a file whose Book is missing, and which is now DRM-protected,
// removes the record without importing the file.
func TestResync_OrphanedDRMProtectedFileIsRemoved(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Orphan")
	path := testgen.GenerateMOBI(t, dir, "book.mobi", testgen.MOBIOptions{Title: "Orphan"})
	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)
	tc.orphanBook(files[0].BookID)
	require.NoError(t, os.WriteFile(path, testgen.BuildMOBI(t, testgen.MOBIOptions{Title: "Orphan", Encrypted: true}), 0o644))

	result, err := tc.worker.Scan(tc.ctx, books.ScanOptions{FileID: files[0].ID})
	require.NoError(t, err)
	assert.True(t, result.FileDeleted)
	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

// The cover and sidecar of a file replaced by a DRM-protected copy are
// removed with its record, so a DRM-free copy put back later gets its own
// cover rather than adopting the old one.
func TestResync_DRMReplacementRemovesCoverAndSidecar(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Covered")
	path := testgen.GenerateMOBI(t, dir, "book.mobi", testgen.MOBIOptions{Title: "Covered", HasCover: true})
	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)
	coverPath := covers.FileCoverPath(files[0])
	require.FileExists(t, coverPath)
	sidecarPath := sidecar.FileSidecarPath(path)
	require.FileExists(t, sidecarPath, "precondition: the scan wrote the file's sidecar")

	require.NoError(t, os.WriteFile(path, testgen.BuildMOBI(t, testgen.MOBIOptions{Title: "Covered", Encrypted: true}), 0o644))
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: files[0].ID, ForceRefresh: true}, nil)
	require.NoError(t, err)
	require.True(t, result.FileDeleted)

	assert.NoFileExists(t, coverPath)
	assert.NoFileExists(t, sidecarPath)
	assert.FileExists(t, path, "the DRM-protected file itself is left alone")
}
