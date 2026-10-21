package worker

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessScanJob_KePubIsDetected(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	kepubDir := testgen.CreateSubDir(t, libraryPath, "Kobo Book")
	testgen.GenerateEPUB(t, kepubDir, "book.kepub.epub", testgen.EPUBOptions{
		Title: "Kobo Book", Authors: []string{"Jane Doe"}, HasCover: true, KoboMarkup: true,
	})
	calibreDir := testgen.CreateSubDir(t, libraryPath, "Calibre Book")
	testgen.GenerateEPUB(t, calibreDir, "Calibre Book - Jane Doe.kepub", testgen.EPUBOptions{
		Title: "Calibre Book", Authors: []string{"Jane Doe"}, KoboMarkup: true,
	})
	epubDir := testgen.CreateSubDir(t, libraryPath, "Plain Book")
	testgen.GenerateEPUB(t, epubDir, "book.epub", testgen.EPUBOptions{Title: "Plain Book", Authors: []string{"Jane Doe"}})

	require.NoError(t, tc.runScan())

	byTitle := map[string]*models.Book{}
	for _, b := range tc.listBooks() {
		byTitle[b.Title] = b
	}
	require.Len(t, byTitle, 3)

	kobo := byTitle["Kobo Book"]
	require.NotNil(t, kobo)
	assert.Equal(t, models.DataSourceEPUBMetadata, kobo.TitleSource, "a KePub's metadata is read as an EPUB's")
	require.Len(t, kobo.Files, 1)
	assert.Equal(t, models.FileTypeKePub, kobo.Files[0].FileType)
	assert.Equal(t, "book.kepub.epub", filepath.Base(kobo.Files[0].Filepath))
	assert.NotNil(t, kobo.Files[0].CoverImageFilename, "a KePub's cover is extracted")

	calibre := byTitle["Calibre Book"]
	require.NotNil(t, calibre, "a bare .kepub, as Calibre stores one, is a KePub too")
	require.Len(t, calibre.Files, 1)
	assert.Equal(t, models.FileTypeKePub, calibre.Files[0].FileType)
	assert.Equal(t, models.FileRoleMain, calibre.Files[0].FileRole)

	plain := byTitle["Plain Book"]
	require.NotNil(t, plain)
	require.Len(t, plain.Files, 1)
	assert.Equal(t, models.FileTypeEPUB, plain.Files[0].FileType)
}

func TestProcessScanJob_DRMProtectedKePubIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Locked Kobo Book")
	path := testgen.GenerateEPUB(t, dir, "book.kepub.epub", testgen.EPUBOptions{
		Title: "Locked Kobo Book", KoboMarkup: true, KoboDRM: true,
	})

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
	warnings := warningsFor(logs, path)
	require.Len(t, warnings, 1)
	assert.Equal(t, "skipped DRM-protected file", warnings[0].Message)
}

// A root-level KePub with no title in its metadata is titled from its name
// without the whole .kepub.epub extension.
func TestDeriveInitialTitle_KePubDropsItsWholeExtension(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "My Book", deriveInitialTitle("/lib/My Book.kepub.epub", true, nil))
}

func TestCheckExpectedMimeType_KePubExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"a.kepub.epub", "b.kepub", "c.KEPUB"} {
		path := testgen.GenerateEPUB(t, dir, name, testgen.EPUBOptions{Title: "Book", KoboMarkup: true})
		_, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok, name)
	}

	path := testgen.WriteFile(t, dir, "d.kepub", []byte("plain text, not a KePub"))
	_, ok, err := checkExpectedMimeType(path)
	require.NoError(t, err)
	assert.False(t, ok)
}

// Before bare .kepub files were KePubs, a scan attached one as a supplement,
// reading only its name. Promoted to a main File, it is read in full by the
// next scan: cover, chapters, and metadata.
func TestScan_PromotedKePubSupplementIsReadInFull(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Calibre Book")
	testgen.GenerateEPUB(t, dir, "Calibre Book - Jane Doe.epub", testgen.EPUBOptions{Title: "Calibre Book", Authors: []string{"Jane Doe"}})
	kepubPath := testgen.GenerateEPUB(t, dir, "Calibre Book - Jane Doe.kepub", testgen.EPUBOptions{
		Title: "Calibre Book", Authors: []string{"Jane Doe"}, HasCover: true, KoboMarkup: true,
	})
	require.NoError(t, tc.runScan())

	books := tc.listBooks()
	require.Len(t, books, 1, "a Calibre folder's EPUB and KePub are one book")
	var kepub *models.File
	for _, f := range tc.listFiles() {
		assert.Equal(t, models.FileRoleMain, f.FileRole, "%s is a main File", f.Filepath)
		if f.Filepath == kepubPath {
			kepub = f
		}
	}
	require.NotNil(t, kepub)
	// Make it what an earlier version left: a supplement with no cover, its
	// size and mtime recorded by a later scan.
	kepub.FileRole = models.FileRoleSupplement
	kepub.CoverImageFilename = nil
	_, err := tc.db.NewUpdate().Model(kepub).Column("file_role", "cover_image_filename").WherePK().Exec(tc.ctx)
	require.NoError(t, err)
	require.NoError(t, tc.runScan())
	require.NoError(t, tc.db.NewSelect().Model(kepub).WherePK().Scan(tc.ctx))
	require.NotNil(t, kepub.FileModifiedAt, "precondition: the scan recorded the supplement's mtime")

	require.NoError(t, tc.worker.bookService.PromoteSupplementToMain(tc.ctx, kepub.ID))
	require.NoError(t, tc.runScan())

	require.NoError(t, tc.db.NewSelect().Model(kepub).WherePK().Scan(tc.ctx))
	assert.Equal(t, models.FileRoleMain, kepub.FileRole)
	assert.NotNil(t, kepub.CoverImageFilename, "the promoted KePub's cover is extracted")
}
