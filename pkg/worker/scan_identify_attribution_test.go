package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifyTestParserManifest = `{
  "manifestVersion": 1,
  "id": "idtest-parser",
  "name": "IDTest Parser",
  "version": "1.0.0",
  "capabilities": {"fileParser": {"description": "Parses idtest files", "types": ["idtest"]}}
}`

// The parser stands in for embedded file metadata.
const identifyTestParserJS = `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Embedded Title",
          subtitle: "Embedded Subtitle",
          description: "Embedded description",
          publisher: "Embedded Publisher",
          url: "https://example.com/embedded",
          releaseDate: "2020-01-02",
          language: "en",
          abridged: true
        };
      }
    }
  };
})();`

const identifyTestEnricherManifest = `{
  "manifestVersion": 1,
  "id": "auto-enricher",
  "name": "Auto Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Proposes a description, publisher, and identifiers",
      "fileTypes": ["idtest"],
      "fields": ["description", "publisher", "identifiers"]
    }
  }
}`

// The enricher runs on every ordinary Scan with plugin priority, which is
// what used to overwrite values edited during Identify.
const identifyTestEnricherJS = `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(ctx) {
        return { results: [{
          description: "Plugin description",
          publisher: "Plugin Publisher",
          identifiers: [{ type: "isbn_13", value: "9780316769488" }, { type: "asin", value: "B01ABC1234" }]
        }] };
      }
    }
  };
})();`

// newIdentifyApplyServer serves the real POST /plugins/apply route against the
// test database, with the same store wiring pkg/server uses.
func newIdentifyApplyServer(t *testing.T, tc *testContext) *echo.Echo {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	store := books.NewPluginMetadataStore(tc.bookService)
	g := e.Group("/plugins", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user", &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}})
			return next(c)
		}
	})
	plugins.RegisterIdentifyRoutes(g, tc.worker.pluginService, tc.worker.pluginManager, &plugins.EnrichDeps{
		BookStore:       store,
		RelStore:        store,
		IdentStore:      store,
		PersonFinder:    people.NewService(tc.db),
		GenreFinder:     genres.NewService(tc.db),
		TagFinder:       tags.NewService(tc.db),
		PublisherFinder: publishers.NewService(tc.db),
		SearchIndexer:   tc.worker.searchService,
		// The production extractor, so page-based cover applies exercise the
		// real on-disk install rather than a stub.
		PageExtractor: books.NewPluginPageExtractor(cbzpages.NewCache(t.TempDir()), pdfpages.NewCache(t.TempDir(), 150, 85)),
	})
	return e
}

func postIdentifyApply(t *testing.T, e *echo.Echo, payload plugins.PluginApplyPayload) {
	t.Helper()
	postIdentifyApplyResponse(t, e, payload)
}

// postIdentifyApplyResponse is postIdentifyApply for tests that assert on the
// response body, such as the warnings a skipped cover produces.
func postIdentifyApplyResponse(t *testing.T, e *echo.Echo, payload plugins.PluginApplyPayload) plugins.PluginApplyResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp plugins.PluginApplyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// identifyScanFixture is a scanned one-file library whose embedded metadata
// comes from the idtest parser and whose auto-enricher proposes a description
// and publisher on every ordinary Scan.
type identifyScanFixture struct {
	tc *testContext
}

func newIdentifyScanFixture(t *testing.T) *identifyScanFixture {
	t.Helper()

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "idtest-parser", identifyTestParserManifest, identifyTestParserJS)
	installTestPlugin(t, tc, pluginDir, "auto-enricher", identifyTestEnricherManifest, identifyTestEnricherJS)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "auto-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(context.Background()))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := filepath.Join(libraryPath, "Embedded Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.idtest"), []byte("content"), 0644))

	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

func (f *identifyScanFixture) retrieve(t *testing.T) (*models.Book, *models.File) {
	t.Helper()
	allBooks := f.tc.listBooks()
	require.Len(t, allBooks, 1)
	book, err := f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &allBooks[0].ID})
	require.NoError(t, err)
	require.Len(t, book.Files, 1)
	return book, book.Files[0]
}

// ordinaryScan rescans the file the way a plain Resync does: no refresh, no
// reset, auto-enrichers on.
func (f *identifyScanFixture) ordinaryScan(t *testing.T, fileID int) {
	t.Helper()
	_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: fileID}, nil)
	require.NoError(t, err)
}

// TestIdentifyApply_ThenOrdinaryScan verifies Identify attribution by its
// user-visible consequence: what the next ordinary Scan does to the values.
func TestIdentifyApply_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyScanFixture(t)
	tc := f.tc

	book, file := f.retrieve(t)
	require.Equal(t, "Plugin description", *book.Description, "precondition: the auto-enricher wins the first Scan")
	require.Equal(t, "Plugin Publisher", file.Publisher.Name)
	require.Equal(t, "Embedded Subtitle", *book.Subtitle)

	postIdentifyApply(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			// Edited away from the proposal.
			"description": "My edited description",
			"publisher":   "My Publisher",
			// Explicit Clears.
			"subtitle":     "",
			"url":          "",
			"release_date": "",
			"language":     "",
		},
		Sources: map[string]string{
			"description": plugins.SourceIntentUser,
			"publisher":   plugins.SourceIntentUser,
		},
		PluginScope: "test",
		PluginID:    "auto-enricher",
	})

	book, file = f.retrieve(t)
	require.Equal(t, "My edited description", *book.Description)
	require.Equal(t, models.DataSourceManual, *book.DescriptionSource)
	require.Equal(t, "My Publisher", file.Publisher.Name)
	require.Equal(t, models.DataSourceManual, *file.PublisherSource)
	require.Nil(t, book.Subtitle)
	require.Nil(t, book.SubtitleSource)
	require.Nil(t, file.URL)
	require.Nil(t, file.URLSource)
	require.Nil(t, file.ReleaseDate)
	require.Nil(t, file.ReleaseDateSource)
	require.Nil(t, file.Language)
	require.Nil(t, file.LanguageSource)

	f.ordinaryScan(t, file.ID)

	book, file = f.retrieve(t)
	assert.Equal(t, "My edited description", *book.Description, "a manual Identify edit must survive auto-enrichment")
	assert.Equal(t, models.DataSourceManual, *book.DescriptionSource)
	assert.Equal(t, "My Publisher", file.Publisher.Name, "a manual Identify edit must survive auto-enrichment")
	assert.Equal(t, models.DataSourceManual, *file.PublisherSource)

	require.NotNil(t, book.Subtitle, "a cleared subtitle must be repopulated from embedded metadata")
	assert.Equal(t, "Embedded Subtitle", *book.Subtitle)
	require.NotNil(t, file.URL, "a cleared URL must be repopulated from embedded metadata")
	assert.Equal(t, "https://example.com/embedded", *file.URL)
	require.NotNil(t, file.ReleaseDate, "a cleared release date must be repopulated from embedded metadata")
	assert.Equal(t, "2020-01-02", file.ReleaseDate.UTC().Format("2006-01-02"))
	require.NotNil(t, file.Language, "a cleared language must be repopulated from embedded metadata")
	assert.Equal(t, "en", *file.Language)
}

// TestIdentifyApply_ClearedEnricherFields_ThenOrdinaryScan covers the cleared
// fields the first test spends on manual edits, plus Abridged.
func TestIdentifyApply_ClearedEnricherFields_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyScanFixture(t)

	book, file := f.retrieve(t)
	require.NotNil(t, file.Abridged, "precondition: the first Scan reads embedded abridged")
	require.True(t, *file.Abridged)

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"description": "",
			"publisher":   "",
			"abridged":    nil,
		},
		PluginScope: "test",
		PluginID:    "auto-enricher",
	})

	book, file = f.retrieve(t)
	require.Nil(t, book.Description)
	require.Nil(t, book.DescriptionSource)
	require.Nil(t, file.PublisherID)
	require.Nil(t, file.PublisherSource)
	require.Nil(t, file.Abridged)
	require.Nil(t, file.AbridgedSource)

	f.ordinaryScan(t, file.ID)

	book, file = f.retrieve(t)
	require.NotNil(t, book.Description, "a cleared description must be repopulated by the next Scan")
	assert.Equal(t, "Plugin description", *book.Description)
	require.NotNil(t, file.Publisher, "a cleared publisher must be repopulated by the next Scan")
	assert.Equal(t, "Plugin Publisher", file.Publisher.Name)
	require.NotNil(t, file.Abridged, "a cleared abridged flag must be repopulated from embedded metadata")
	assert.True(t, *file.Abridged)
}
