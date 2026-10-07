package opds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A series from another library is not found under this library's feed, so
// its name does not leak into the feed title. Both the plain and the KePub
// series feeds share the check.
func TestOPDSLibrarySeriesBooks_SeriesFromOtherLibrary_Returns404(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	var libs [2]*models.Library
	for i := range libs {
		libs[i] = &models.Library{
			Name:                     "Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(libs[i]).Exec(ctx)
		require.NoError(t, err)
	}
	s := &models.Series{
		LibraryID:      libs[1].ID,
		Name:           "Secret Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Secret Series",
		SortNameSource: models.DataSourceManual,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	_, err := db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db, books.NewService(db, appsettings.NewService(db)))
	builders := map[string]func(libraryID int) (*Feed, error){
		"plain": func(libraryID int) (*Feed, error) {
			return svc.BuildLibrarySeriesBooksFeed(ctx, "http://x", "", libraryID, s.ID, 10, 0, nil)
		},
		"kepub": func(libraryID int) (*Feed, error) {
			return svc.BuildLibrarySeriesBooksFeedKepub(ctx, "http://x", "", libraryID, s.ID, 10, 0, nil)
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			feed, err := build(libs[0].ID)
			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr, "feed title: %v", feed)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.Equal(t, "Series not found.", codeErr.Message)

			// Positive control: the series feed works under its own library.
			feed, err = build(libs[1].ID)
			require.NoError(t, err)
			assert.Equal(t, "Secret Series", feed.Title)
		})
	}
}

// The plain and KePub series feed handlers both return 404 for a series from
// another library, through the full handler path.
func TestOPDSLibrarySeriesBooksHandlers_SeriesFromOtherLibrary_Returns404(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	var libs [2]*models.Library
	for i := range libs {
		libs[i] = &models.Library{
			Name:                     "Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(libs[i]).Exec(ctx)
		require.NoError(t, err)
	}
	s := &models.Series{
		LibraryID:      libs[1].ID,
		Name:           "Secret Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Secret Series",
		SortNameSource: models.DataSourceManual,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	_, err := db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	bookService := books.NewService(db, appsettings.NewService(db))
	h := &handler{
		opdsService:     NewService(db, bookService),
		bookService:     bookService,
		settingsService: settings.NewService(db),
	}
	user := &models.User{ID: 1, IsActive: true, LibraryAccess: []*models.UserLibraryAccess{{}}}

	serve := func(fn echo.HandlerFunc, libraryID int) (*httptest.ResponseRecorder, error) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.SetParamNames("types", "libraryID", "seriesID")
		c.SetParamValues(models.FileTypeEPUB, strconv.Itoa(libraryID), strconv.Itoa(s.ID))
		auth.SetUser(c, user)
		return rec, fn(c)
	}

	for name, fn := range map[string]echo.HandlerFunc{
		"plain": h.librarySeriesBooks,
		"kepub": h.librarySeriesBooksKepub,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, err := serve(fn, libs[0].ID)
			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.NotContains(t, rec.Body.String(), "Secret Series")

			// Positive control: the feed renders under the series' own library.
			rec, err = serve(fn, libs[1].ID)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "Secret Series")
		})
	}
}
