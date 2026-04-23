package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateViewerSettings_RejectsBadEpubFontSize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "alice")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_epub_font_size": 999,
		"viewer_epub_theme": "light",
		"viewer_epub_flow": "paginated"
	}`
	req := httptest.NewRequest(http.MethodPut, "/settings/viewer", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.updateViewerSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "viewer_epub_font_size")
}

func TestUpdateViewerSettings_AcceptsValidEpubPayload(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "bob")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_epub_font_size": 130,
		"viewer_epub_theme": "dark",
		"viewer_epub_flow": "scrolled"
	}`
	req := httptest.NewRequest(http.MethodPut, "/settings/viewer", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateViewerSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ViewerSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, 130, resp.EpubFontSize)
	assert.Equal(t, models.EpubThemeDark, resp.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, resp.EpubFlow)
}

// TestUpdateViewerSettings_AcceptsSingleFieldPayload verifies that a payload
// containing only one field (omitting all others) updates just that field
// and leaves unrelated settings at their existing values.
func TestUpdateViewerSettings_AcceptsSingleFieldPayload(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "dave")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	// Only send viewer_epub_theme. Everything else must keep its default.
	body := `{"viewer_epub_theme": "sepia"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/viewer", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateViewerSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ViewerSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.EpubThemeSepia, resp.EpubTheme)
	// Other fields stay at defaults
	assert.Equal(t, 3, resp.PreloadCount)
	assert.Equal(t, models.FitModeHeight, resp.FitMode)
	assert.Equal(t, 100, resp.EpubFontSize)
	assert.Equal(t, models.EpubFlowPaginated, resp.EpubFlow)
}
