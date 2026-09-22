package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifySeriesEnricherManifest = `{
  "manifestVersion": 1,
  "id": "series-enricher",
  "name": "Series Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub"],
      "fields": ["series", "seriesNumber"]
    }
  }
}`

const identifySeriesEnricherJS = `var plugin = {
  metadataEnricher: {
    search: function(ctx) {
      return { results: [{ series: "Plugin Series", seriesNumber: 2 }] };
    }
  }
};`

// An EPUB carries embedded series metadata at file-metadata priority; the
// enricher proposes a different membership at plugin priority on every
// ordinary Scan.
func newIdentifySeriesScanFixture(t *testing.T, autoEnrich bool) *identifyScanFixture {
	t.Helper()

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	js := `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`
	if autoEnrich {
		js = identifySeriesEnricherJS
	}
	installTestPlugin(t, tc, pluginDir, "series-enricher", identifySeriesEnricherManifest, js)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "series-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Embedded Title")
	number := 1.0
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:        "Embedded Title",
		Authors:      []string{"Embedded Author"},
		Series:       "Embedded Series",
		SeriesNumber: &number,
	})

	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

func seriesEntry(name string, number float64, extra map[string]any) map[string]any {
	entry := map[string]any{"name": name, "number": number}
	for k, v := range extra {
		entry[k] = v
	}
	return entry
}

func seriesApplyPayload(book *models.Book, file *models.File, entries []map[string]any, intent string) plugins.PluginApplyPayload {
	payload := plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:      map[string]any{"series": entries},
		PluginScope: "test", PluginID: "series-enricher",
	}
	if intent != "" {
		payload.Sources = map[string]string{"series": intent}
	}
	return payload
}

func membershipSummary(book *models.Book) []string {
	out := make([]string, 0, len(book.BookSeries))
	for _, bs := range book.BookSeries {
		name := ""
		if bs.Series != nil {
			name = bs.Series.Name
		}
		out = append(out, name+"|"+floatPtrString(bs.SeriesNumber)+"|"+floatPtrString(bs.SeriesNumberEnd)+"|"+stringPtrValue(bs.SeriesNumberUnit))
	}
	return out
}

func floatPtrString(v *float64) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(*v)
	return string(b)
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func TestIdentifySeries_ScanReadsBookSourceNotSeriesName(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, true)
	book, file := f.retrieve(t)
	require.Equal(t, []string{"Plugin Series|2||"}, membershipSummary(book), "precondition: the enricher wins the first Scan")
	require.Equal(t, models.PluginDataSource("test", "series-enricher"), *book.SeriesSource)

	// The Series name is manual but the membership is not. The old proxy
	// would have protected the membership by the name's source.
	_, err := f.tc.db.NewRaw("UPDATE series SET name_source = ? WHERE id = ?", models.DataSourceManual, book.BookSeries[0].SeriesID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("UPDATE books SET series_source = ? WHERE id = ?", models.DataSourceEPUBMetadata, book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("UPDATE book_series SET series_number = 9 WHERE book_id = ?", book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)

	f.ordinaryScan(t, file.ID)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Plugin Series|2||"}, membershipSummary(book), "a low-priority membership source is replaced regardless of the Series name's source")
	assert.Equal(t, models.PluginDataSource("test", "series-enricher"), *book.SeriesSource)
}

func TestIdentifySeries_BackfilledManualSourceSurvivesOrdinaryScan(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, true)
	book, file := f.retrieve(t)
	// What the backfill migration produces for a Book whose Series name was
	// edited manually before the column existed.
	_, err := f.tc.db.NewRaw("UPDATE series SET name = 'Renamed Series', name_source = ? WHERE id = ?", models.DataSourceManual, book.BookSeries[0].SeriesID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("UPDATE books SET series_source = ? WHERE id = ?", models.DataSourceManual, book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)

	f.ordinaryScan(t, file.ID)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Renamed Series|2||"}, membershipSummary(book))
	assert.Equal(t, models.DataSourceManual, *book.SeriesSource)
}

func TestIdentifySeries_NoOpPreservesSourceWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, source := range []string{models.DataSourceManual, models.PluginDataSource("test", "series-enricher")} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			f := newIdentifySeriesScanFixture(t, false)
			book, file := f.retrieve(t)
			require.Equal(t, []string{"Embedded Series|1||"}, membershipSummary(book))
			_, err := f.tc.db.NewRaw("UPDATE books SET series_source = ? WHERE id = ?", source, book.ID).Exec(f.tc.ctx)
			require.NoError(t, err)
			_, err = f.tc.db.NewRaw("INSERT INTO series_aliases (created_at, series_id, name, library_id) VALUES (CURRENT_TIMESTAMP, ?, ?, ?)", book.BookSeries[0].SeriesID, "Series Alias", book.LibraryID).Exec(f.tc.ctx)
			require.NoError(t, err)

			writes := &identifyWrites{}
			f.tc.db.AddQueryHook(writes)
			// A different spelling that resolves to the same Series through
			// an Alias is the same membership.
			postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), seriesApplyPayload(book, file, []map[string]any{seriesEntry("Series Alias", 1, nil)}, plugins.SourceIntentPlugin))
			updated, _ := f.retrieve(t)
			assert.Equal(t, membershipSummary(book), membershipSummary(updated))
			assert.Equal(t, &source, updated.SeriesSource)
			assert.Empty(t, writes.queries, "no-op must not delete/insert memberships or reindex FTS")
		})
	}
}

func TestIdentifySeries_ChangesUseIntent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []map[string]any
		want    []string
	}{
		{"number", []map[string]any{seriesEntry("Embedded Series", 3, nil)}, []string{"Embedded Series|3||"}},
		{"range end", []map[string]any{seriesEntry("Embedded Series", 1, map[string]any{"series_number_end": 3})}, []string{"Embedded Series|1|3|"}},
		{"unit", []map[string]any{seriesEntry("Embedded Series", 1, map[string]any{"series_number_unit": "chapter"})}, []string{"Embedded Series|1||chapter"}},
		{"identity", []map[string]any{seriesEntry("Other Series", 1, nil)}, []string{"Other Series|1||"}},
		{"added membership", []map[string]any{seriesEntry("Embedded Series", 1, nil), seriesEntry("Other Series", 4, nil)}, []string{"Embedded Series|1||", "Other Series|4||"}},
	}
	for _, intent := range []string{plugins.SourceIntentPlugin, plugins.SourceIntentUser, ""} {
		for _, tc := range cases {
			intentName := intent
			if intentName == "" {
				intentName = "missing"
			}
			t.Run(intentName+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				f := newIdentifySeriesScanFixture(t, false)
				book, file := f.retrieve(t)
				postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), seriesApplyPayload(book, file, tc.entries, intent))
				updated, _ := f.retrieve(t)
				assert.Equal(t, tc.want, membershipSummary(updated))
				want := models.DataSourceManual
				if intent == plugins.SourceIntentPlugin {
					want = models.PluginDataSource("test", "series-enricher")
				}
				assert.Equal(t, &want, updated.SeriesSource)
				bookSidecar, err := sidecar.ReadBookSidecarFromModel(updated, file)
				require.NoError(t, err)
				require.NotNil(t, bookSidecar)
				require.Len(t, bookSidecar.Series, len(tc.want))
			})
		}
	}
}

func TestIdentifySeries_ReorderIsAChange(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, false)
	book, file := f.retrieve(t)
	e := newIdentifyApplyServer(t, f.tc)
	postIdentifyApply(t, e, seriesApplyPayload(book, file, []map[string]any{seriesEntry("Embedded Series", 1, nil), seriesEntry("Other Series", 4, nil)}, plugins.SourceIntentPlugin))
	book, _ = f.retrieve(t)
	require.Equal(t, []string{"Embedded Series|1||", "Other Series|4||"}, membershipSummary(book))

	postIdentifyApply(t, e, seriesApplyPayload(book, file, []map[string]any{seriesEntry("Other Series", 4, nil), seriesEntry("Embedded Series", 1, nil)}, plugins.SourceIntentUser))
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Other Series|4||", "Embedded Series|1||"}, membershipSummary(book))
	manual := models.DataSourceManual
	assert.Equal(t, &manual, book.SeriesSource)
}

func TestIdentifySeries_ManualMembershipSurvivesOrdinaryScan(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, true)
	book, file := f.retrieve(t)
	require.Equal(t, []string{"Plugin Series|2||"}, membershipSummary(book), "precondition: the enricher wins the first Scan")

	// Keep the plugin's Series but correct its number. The Series name keeps
	// its plugin source; only the membership becomes manual.
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), seriesApplyPayload(book, file, []map[string]any{seriesEntry("Plugin Series", 5, nil)}, plugins.SourceIntentUser))
	book, _ = f.retrieve(t)
	require.Equal(t, []string{"Plugin Series|5||"}, membershipSummary(book))
	manual := models.DataSourceManual
	require.Equal(t, &manual, book.SeriesSource)
	assert.Equal(t, models.PluginDataSource("test", "series-enricher"), book.BookSeries[0].Series.NameSource)

	f.ordinaryScan(t, file.ID)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Plugin Series|5||"}, membershipSummary(book))
	assert.Equal(t, &manual, book.SeriesSource)
}

func TestIdentifySeries_ClearNullsSourceAndScanRepopulates(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, false)
	book, file := f.retrieve(t)
	e := newIdentifyApplyServer(t, f.tc)
	// Establish plugin provenance first; a leftover plugin source on an empty
	// collection would outrank the embedded metadata.
	postIdentifyApply(t, e, seriesApplyPayload(book, file, []map[string]any{seriesEntry("Plugin Series", 2, nil)}, plugins.SourceIntentPlugin))
	book, _ = f.retrieve(t)
	require.Equal(t, models.PluginDataSource("test", "series-enricher"), *book.SeriesSource)

	postIdentifyApply(t, e, seriesApplyPayload(book, file, []map[string]any{}, plugins.SourceIntentUser))
	book, _ = f.retrieve(t)
	assert.Empty(t, book.BookSeries)
	assert.Nil(t, book.SeriesSource)
	var sourceIsNull bool
	require.NoError(t, f.tc.db.NewSelect().Table("books").ColumnExpr("series_source IS NULL").Where("id = ?", book.ID).Scan(f.tc.ctx, &sourceIsNull))
	assert.True(t, sourceIsNull, "an Explicit Clear leaves no provenance behind")
	bookSidecar, err := sidecar.ReadBookSidecarFromModel(book, file)
	require.NoError(t, err)
	require.NotNil(t, bookSidecar)
	assert.Empty(t, bookSidecar.Series)

	f.ordinaryScan(t, file.ID)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Embedded Series|1||"}, membershipSummary(book))
	epub := models.DataSourceEPUBMetadata
	assert.Equal(t, &epub, book.SeriesSource)
}

func TestIdentifySeries_SidecarScanWritesSidecarSource(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, false)
	book, file := f.retrieve(t)
	bookSidecar, err := sidecar.ReadBookSidecarFromModel(book, file)
	require.NoError(t, err)
	require.NotNil(t, bookSidecar)
	number := 7.0
	bookSidecar.Series = []sidecar.SeriesMetadata{{Name: "Sidecar Series", Number: &number}}
	require.NoError(t, sidecar.WriteBookSidecar(book.Filepath, bookSidecar))

	f.ordinaryScan(t, file.ID)
	book, _ = f.retrieve(t)
	assert.Equal(t, []string{"Sidecar Series|7||"}, membershipSummary(book))
	sidecarSource := models.DataSourceSidecar
	assert.Equal(t, &sidecarSource, book.SeriesSource)
}

func TestIdentifySeries_MalformedGroupRejectedBeforeMutation(t *testing.T) {
	t.Parallel()
	for name, entries := range map[string][]map[string]any{
		"reversed range":    {seriesEntry("Embedded Series", 3, map[string]any{"series_number_end": 1})},
		"end without start": {{"name": "Embedded Series", "series_number_end": 3}},
		"invalid unit":      {seriesEntry("Embedded Series", 1, map[string]any{"series_number_unit": "bogus"})},
		"non-numeric":       {{"name": "Embedded Series", "number": "one"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newIdentifySeriesScanFixture(t, false)
			book, file := f.retrieve(t)
			epub := models.DataSourceEPUBMetadata
			require.Equal(t, &epub, book.SeriesSource)
			writes := &identifyWrites{}
			f.tc.db.AddQueryHook(writes)
			body, err := json.Marshal(seriesApplyPayload(book, file, entries, plugins.SourceIntentUser))
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newIdentifyApplyServer(t, f.tc).ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			updated, _ := f.retrieve(t)
			assert.Equal(t, membershipSummary(book), membershipSummary(updated))
			assert.Equal(t, &epub, updated.SeriesSource)
			assert.Empty(t, writes.queries)
		})
	}
}

func TestIdentifySeries_InsertFailureReturnsError(t *testing.T) {
	t.Parallel()
	f := newIdentifySeriesScanFixture(t, false)
	book, file := f.retrieve(t)
	_, err := f.tc.db.ExecContext(f.tc.ctx, "CREATE TRIGGER fail_insert BEFORE INSERT ON book_series BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END")
	require.NoError(t, err)
	body, err := json.Marshal(seriesApplyPayload(book, file, []map[string]any{seriesEntry("Other Series", 1, nil)}, plugins.SourceIntentUser))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newIdentifyApplyServer(t, f.tc).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	updated, _ := f.retrieve(t)
	// Delete and insert are not wrapped in a transaction (out of scope per
	// ADR 0006), so only the attribution is asserted: the source must not be
	// rewritten to the intent's value for a collection that failed to persist.
	assert.Equal(t, book.SeriesSource, updated.SeriesSource)
}
