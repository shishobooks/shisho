package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seriesWrites captures membership and Series row churn during a scan.
type seriesWrites struct {
	mu      sync.Mutex
	queries []string
}

func (q *seriesWrites) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (q *seriesWrites) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	sql := strings.ToUpper(strings.TrimSpace(event.Query))
	// The books FTS row has a series_names column and is rewritten on every
	// scan; only membership and Series row writes are churn.
	if (strings.HasPrefix(sql, "INSERT") || strings.HasPrefix(sql, "DELETE")) &&
		(strings.Contains(sql, `"BOOK_SERIES"`) || strings.Contains(sql, `"SERIES"`)) {
		q.mu.Lock()
		q.queries = append(q.queries, event.Query)
		q.mu.Unlock()
	}
}

func (q *seriesWrites) all() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.queries...)
}

// renameAttachedSeries simulates a user renaming the Series on the series
// page while keeping the old name as an Alias.
func renameAttachedSeries(t *testing.T, f *identifyScanFixture, book *models.Book, newName string) {
	t.Helper()
	require.NotEmpty(t, book.BookSeries)
	series := book.BookSeries[0].Series
	_, err := f.tc.db.NewRaw("UPDATE series SET name = ?, name_source = ? WHERE id = ?", newName, models.DataSourceManual, series.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("INSERT INTO series_aliases (created_at, series_id, name, library_id) VALUES (CURRENT_TIMESTAMP, ?, ?, ?)", series.ID, series.Name, book.LibraryID).Exec(f.tc.ctx)
	require.NoError(t, err)
}

// A renamed Series whose old name survives as an Alias is still the same
// Series. A scan that sees the old name in embedded metadata, a sidecar, or an
// enricher proposal must not replace the membership or create a duplicate.
func TestScanSeries_AliasOfAttachedSeriesDoesNotChurn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		autoEnrich   bool
		keepSidecar  bool
		wantSource   string
		wantSummary  string
		embeddedName string
	}{
		{name: "embedded metadata", wantSource: models.DataSourceEPUBMetadata, wantSummary: "Correct Name|1||", embeddedName: "Embedded Series"},
		{name: "sidecar", keepSidecar: true, wantSource: models.DataSourceEPUBMetadata, wantSummary: "Correct Name|1||", embeddedName: "Embedded Series"},
		{name: "enricher proposal", autoEnrich: true, keepSidecar: true, wantSource: models.PluginDataSource("test", "series-enricher"), wantSummary: "Correct Name|2||", embeddedName: "Plugin Series"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newIdentifySeriesScanFixture(t, tc.autoEnrich)
			book, file := f.retrieve(t)
			require.Equal(t, tc.embeddedName, book.BookSeries[0].Series.Name)
			renameAttachedSeries(t, f, book, "Correct Name")
			if !tc.keepSidecar {
				require.NoError(t, os.Remove(sidecar.BookSidecarPath(book.Filepath)))
			}

			writes := &seriesWrites{}
			f.tc.db.AddQueryHook(writes)
			f.ordinaryScan(t, file.ID)
			book, _ = f.retrieve(t)
			assert.Equal(t, []string{tc.wantSummary}, membershipSummary(book))
			assert.Equal(t, &tc.wantSource, book.SeriesSource)
			assert.Empty(t, writes.all(), "an aliased name is the same membership and must not delete, insert, or create Series rows")
			var seriesCount int
			require.NoError(t, f.tc.db.NewRaw("SELECT COUNT(*) FROM series").Scan(f.tc.ctx, &seriesCount))
			assert.Equal(t, 1, seriesCount, "no duplicate Series row for the old name")
		})
	}
}

// Refresh bypasses priority: a manual membership is replaced by the current
// sources, and the source follows the replacement.
func TestScanSeries_RefreshReplacesManualMembership(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, true)
	book, file := f.retrieve(t)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), seriesApplyPayload(book, file, []map[string]any{seriesEntry("Plugin Series", 5, nil)}, plugins.SourceIntentUser))
	book, _ = f.retrieve(t)
	manual := models.DataSourceManual
	require.Equal(t, &manual, book.SeriesSource)
	require.Equal(t, []string{"Plugin Series|5||"}, membershipSummary(book))

	_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true}, nil)
	require.NoError(t, err)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Plugin Series|2||"}, membershipSummary(book))
	plugin := models.PluginDataSource("test", "series-enricher")
	assert.Equal(t, &plugin, book.SeriesSource)
}

// A hybrid book shares one membership collection across its files. A manual
// membership set through the ebook survives ordinary scans of both files,
// including the audiobook that carries no series metadata of its own.
func TestScanSeries_HybridBookKeepsManualMembership(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	f := newIdentifySeriesScanFixture(t, true)
	book, ebook := f.retrieve(t)
	testgen.GenerateM4B(t, filepath.Dir(ebook.Filepath), "book.m4b", testgen.M4BOptions{Title: "Embedded Title", Artist: "Embedded Author"})
	require.NoError(t, f.tc.runScan())
	retrieve := func() (*models.Book, *models.File, *models.File) {
		t.Helper()
		require.Len(t, f.tc.listBooks(), 1)
		full, err := f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
		require.NoError(t, err)
		require.Len(t, full.Files, 2)
		var epub, m4b *models.File
		for _, file := range full.Files {
			switch file.FileType {
			case models.FileTypeEPUB:
				epub = file
			case models.FileTypeM4B:
				m4b = file
			}
		}
		require.NotNil(t, epub)
		require.NotNil(t, m4b)
		return full, epub, m4b
	}
	book, ebook, audio := retrieve()
	require.Equal(t, []string{"Plugin Series|2||"}, membershipSummary(book))

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), seriesApplyPayload(book, ebook, []map[string]any{seriesEntry("Plugin Series", 5, nil)}, plugins.SourceIntentUser))
	f.ordinaryScan(t, audio.ID)
	f.ordinaryScan(t, ebook.ID)
	book, _, _ = retrieve()
	assert.Equal(t, []string{"Plugin Series|5||"}, membershipSummary(book))
	manual := models.DataSourceManual
	assert.Equal(t, &manual, book.SeriesSource)
}

// Restoring a CBZ membership during a scan is path-affecting: the organized
// folder must regain its series number suffix.
func TestScanSeries_RestoredMembershipReorganizesCBZFolder(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	// The apply route needs a loaded plugin to attribute to; this one proposes nothing.
	installTestPlugin(t, tc, pluginDir, "series-enricher", identifySeriesEnricherManifest, `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)
	number := 1.0
	testgen.GenerateCBZ(t, libraryPath, "Comic Run.cbz", testgen.CBZOptions{HasComicInfo: true, Title: "Comic Run", Series: "Comic Run", SeriesNumber: &number, Writer: "QA Comics"})
	require.NoError(t, tc.runScan())
	f := &identifyScanFixture{tc: tc}
	book, file := f.retrieve(t)
	require.Equal(t, "[QA Comics] Comic Run v001", filepath.Base(book.Filepath), "precondition: organized with the series number")
	require.Equal(t, []string{"Comic Run|1||"}, membershipSummary(book))
	organizedFileName := filepath.Base(file.Filepath)

	postIdentifyApply(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:  map[string]any{"series": []any{}},
		Sources: map[string]string{"series": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "series-enricher",
	})
	book, file = f.retrieve(t)
	require.Empty(t, book.BookSeries)
	require.Equal(t, "[QA Comics] Comic Run", filepath.Base(book.Filepath), "precondition: the clear removed the suffix")

	f.ordinaryScan(t, file.ID)
	book, file = f.retrieve(t)
	assert.Equal(t, []string{"Comic Run|1||"}, membershipSummary(book))
	assert.Equal(t, "[QA Comics] Comic Run v001", filepath.Base(book.Filepath))
	assert.Equal(t, filepath.Join(book.Filepath, organizedFileName), file.Filepath)
	assert.FileExists(t, file.Filepath)
}

// A hybrid book with a main CBZ uses CBZ folder naming, so a membership
// restored while scanning its EPUB is path-affecting too.
func TestScanSeries_RestoredMembershipReorganizesHybridCBZFolder(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "series-enricher", identifySeriesEnricherManifest, `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)
	bookDir := testgen.CreateSubDir(t, libraryPath, "Comic Run")
	number := 1.0
	testgen.GenerateCBZ(t, bookDir, "Comic Run.cbz", testgen.CBZOptions{HasComicInfo: true, Title: "Comic Run", Series: "Comic Run", SeriesNumber: &number, Writer: "QA Comics"})
	testgen.GenerateEPUB(t, bookDir, "Comic Run.epub", testgen.EPUBOptions{Title: "Comic Run", Authors: []string{"QA Comics"}, Series: "Comic Run", SeriesNumber: &number})
	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	bookID := tc.listBooks()[0].ID
	retrieve := func() (*models.Book, *models.File) {
		t.Helper()
		book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
		require.NoError(t, err)
		require.Len(t, book.Files, 2)
		for _, file := range book.Files {
			if file.FileType == models.FileTypeEPUB {
				return book, file
			}
		}
		t.Fatal("no EPUB file")
		return nil, nil
	}
	book, epub := retrieve()
	require.Equal(t, "[QA Comics] Comic Run v001", filepath.Base(book.Filepath), "precondition: hybrid book uses CBZ folder naming")

	postIdentifyApply(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &epub.ID,
		Fields:  map[string]any{"series": []any{}},
		Sources: map[string]string{"series": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "series-enricher",
	})
	book, epub = retrieve()
	require.Empty(t, book.BookSeries)
	require.Equal(t, "[QA Comics] Comic Run", filepath.Base(book.Filepath), "precondition: the clear removed the suffix")

	// Only the EPUB is rescanned; it carries the series and is not itself a CBZ.
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: epub.ID}, nil)
	require.NoError(t, err)
	book, _ = retrieve()
	assert.Equal(t, []string{"Comic Run|1||"}, membershipSummary(book))
	assert.Equal(t, "[QA Comics] Comic Run v001", filepath.Base(book.Filepath))
	for _, file := range book.Files {
		assert.FileExists(t, file.Filepath)
	}
}
