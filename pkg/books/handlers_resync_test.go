package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// recordingScanner implements the Scanner interface and records the options it
// was invoked with, returning a canned successful result.
type recordingScanner struct {
	called bool
	opts   ScanOptions
}

func (s *recordingScanner) Scan(_ context.Context, opts ScanOptions) (*ScanResult, error) {
	s.called = true
	s.opts = opts
	return &ScanResult{}, nil
}

// setupTestServerWithScanner sets up an Echo server with the book routes
// registered against the provided scanner.
func setupTestServerWithScanner(t *testing.T, db *bun.DB, scanner Scanner) *echo.Echo {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()

	authService := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	authMiddleware := auth.NewMiddleware(authService)

	g := e.Group("/books")
	RegisterRoutesWithGroup(g, db, cfg, authMiddleware, scanner, nil, nil, appsettings.NewService(db))

	return e
}

func TestResyncBook_InvalidModeRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	scanner := &recordingScanner{}
	e := setupTestServerWithScanner(t, db, scanner)

	req := httptest.NewRequest(http.MethodPost, "/books/"+strconv.Itoa(book.ID)+"/resync", strings.NewReader(`{"mode":"bogus"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "response body: %s", rr.Body.String())
	assert.False(t, scanner.called, "an invalid mode must be rejected before reaching the scanner")
}

func TestResyncFile_InvalidModeRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book, "epub", createTestEPUBFile(t))
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	scanner := &recordingScanner{}
	e := setupTestServerWithScanner(t, db, scanner)

	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID)+"/resync", strings.NewReader(`{"mode":"bogus"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "response body: %s", rr.Body.String())
	assert.False(t, scanner.called, "an invalid mode must be rejected before reaching the scanner")
}

func TestResyncBook_ValidModesReachScanner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode             string
		wantForceRefresh bool
		wantSkipPlugins  bool
		wantReset        bool
	}{
		{mode: ResyncModeScan, wantForceRefresh: false, wantSkipPlugins: false, wantReset: false},
		{mode: ResyncModeRefresh, wantForceRefresh: true, wantSkipPlugins: false, wantReset: false},
		{mode: ResyncModeReset, wantForceRefresh: true, wantSkipPlugins: true, wantReset: true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			t.Parallel()
			db := setupTestDB(t)
			library, book := setupTestLibraryAndBook(t, db)
			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			scanner := &recordingScanner{}
			e := setupTestServerWithScanner(t, db, scanner)

			req := httptest.NewRequest(http.MethodPost, "/books/"+strconv.Itoa(book.ID)+"/resync", strings.NewReader(`{"mode":"`+tt.mode+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rr := executeRequestWithUser(t, e, req, user)

			assert.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())
			require.True(t, scanner.called, "a valid mode must reach the scanner")
			assert.Equal(t, book.ID, scanner.opts.BookID)
			assert.Equal(t, tt.wantForceRefresh, scanner.opts.ForceRefresh, "ForceRefresh")
			assert.Equal(t, tt.wantSkipPlugins, scanner.opts.SkipPlugins, "SkipPlugins")
			assert.Equal(t, tt.wantReset, scanner.opts.Reset, "Reset")
		})
	}
}
