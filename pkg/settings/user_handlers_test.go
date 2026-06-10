package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserSettings_RejectsBadEpubFontSize(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "viewer_epub_font_size")
}

func TestUpdateUserSettings_AcceptsValidEpubPayload(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, 130, resp.EpubFontSize)
	assert.Equal(t, models.EpubThemeDark, resp.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, resp.EpubFlow)
}

// TestUpdateUserSettings_EmptyBodyIsNoop verifies the explicit no-op
// contract: an empty JSON object updates nothing and returns the current
// (or default) values unchanged. Locks down the behavior so a future
// refactor doesn't accidentally start treating nil pointers as "zero the
// field".
func TestUpdateUserSettings_EmptyBodyIsNoop(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "erin")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	// All fields are at their defaults — no prior row existed.
	assert.Equal(t, 3, resp.PreloadCount)
	assert.Equal(t, models.FitModeHeight, resp.FitMode)
	assert.Equal(t, 100, resp.EpubFontSize)
	assert.Equal(t, models.EpubThemeLight, resp.EpubTheme)
	assert.Equal(t, models.EpubFlowPaginated, resp.EpubFlow)
}

// TestUpdateUserSettings_AcceptsSingleFieldPayload verifies that a payload
// containing only one field (omitting all others) updates just that field
// and leaves unrelated settings at their existing values.
func TestUpdateUserSettings_AcceptsSingleFieldPayload(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "dave")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	// Only send viewer_epub_theme. Everything else must keep its default.
	body := `{"viewer_epub_theme": "sepia"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.EpubThemeSepia, resp.EpubTheme)
	// Other fields stay at defaults
	assert.Equal(t, 3, resp.PreloadCount)
	assert.Equal(t, models.FitModeHeight, resp.FitMode)
	assert.Equal(t, 100, resp.EpubFontSize)
	assert.Equal(t, models.EpubFlowPaginated, resp.EpubFlow)
}

func TestUpdateUserSettings_AcceptsValidGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-valid")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "l"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeLarge, resp.GallerySize)
}

func TestUpdateUserSettings_RejectsInvalidGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-bad")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "huge"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gallery_size")
}

func TestUpdateUserSettings_AcceptsValidPlaybackSpeeds(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "speedy-valid")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	for _, speed := range []string{"0.5", "0.75", "1", "1.25", "1.5", "1.75", "2", "2.5", "3"} {
		body := `{"viewer_playback_speed": ` + speed + `}`
		req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", user)

		require.NoError(t, h.updateUserSettings(c), "speed %s should be accepted", speed)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp UserSettingsResponse
		require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
		expected, err := strconv.ParseFloat(speed, 64)
		require.NoError(t, err)
		assert.InDelta(t, expected, resp.PlaybackSpeed, 0)
	}
}

func TestUpdateUserSettings_RejectsInvalidPlaybackSpeeds(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "speedy-bad")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	// Out-of-range values and in-range values that aren't one of the
	// discrete steps must both be rejected.
	for _, speed := range []string{"0", "-1", "0.25", "1.1", "1.999", "3.5", "10"} {
		body := `{"viewer_playback_speed": ` + speed + `}`
		req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", user)

		err := h.updateUserSettings(c)
		require.Error(t, err, "speed %s should be rejected", speed)
		assert.Contains(t, err.Error(), "viewer_playback_speed")
	}
}

func TestGetUserSettings_DefaultsToNormalPlaybackSpeed(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "speedy-default")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodGet, "/settings/user", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.getUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.InDelta(t, 1.0, resp.PlaybackSpeed, 0)
}

func TestGetUserSettings_DefaultsToMediumGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-default")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodGet, "/settings/user", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.getUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeMedium, resp.GallerySize)
}
