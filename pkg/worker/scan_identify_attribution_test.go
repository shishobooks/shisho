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
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/models"
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
          language: "en"
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
      "description": "Proposes a description and publisher",
      "fileTypes": ["idtest"],
      "fields": ["description", "publisher"]
    }
  }
}`

// The enricher runs on every ordinary Scan with plugin priority, which is
// what used to overwrite values edited during Identify.
const identifyTestEnricherJS = `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(ctx) {
        return { results: [{ description: "Plugin description", publisher: "Plugin Publisher" }] };
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
	})
	return e
}

func postIdentifyApply(t *testing.T, e *echo.Echo, payload plugins.PluginApplyPayload) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestIdentifyApply_ThenOrdinaryScan verifies Identify attribution by its
// user-visible consequence: what the next ordinary Scan does to the values.
func TestIdentifyApply_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

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

	retrieve := func() (*models.Book, *models.File) {
		allBooks := tc.listBooks()
		require.Len(t, allBooks, 1)
		book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &allBooks[0].ID})
		require.NoError(t, err)
		require.Len(t, book.Files, 1)
		return book, book.Files[0]
	}

	book, file := retrieve()
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

	book, file = retrieve()
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

	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	book, file = retrieve()
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
