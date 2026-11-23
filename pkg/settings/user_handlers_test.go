package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserSettings_RejectsBadReflowableFontSize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_reflowable_font_size": 999,
		"viewer_reflowable_theme": "light",
		"viewer_reflowable_flow": "paginated"
	}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "viewer_reflowable_font_size")
}

func TestUpdateUserSettings_AcceptsValidReflowablePayload(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "bob")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_reflowable_font_size": 130,
		"viewer_reflowable_theme": "dark",
		"viewer_reflowable_flow": "scrolled"
	}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, 130, resp.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeDark, resp.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowScrolled, resp.ReflowableFlow)
}

// TestUpdateUserSettings_EmptyBodyIsNoop verifies the explicit no-op
// contract: an empty JSON object updates nothing and returns the current
// (or default) values unchanged. Locks down the behavior so a future
// refactor doesn't accidentally start treating nil pointers as "zero the
// field".
func TestUpdateUserSettings_EmptyBodyIsNoop(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "erin")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	// All fields are at their defaults — no prior row existed.
	assert.Equal(t, 3, resp.PreloadCount)
	assert.Equal(t, models.FitModeHeight, resp.FitMode)
	assert.Equal(t, 100, resp.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeSystem, resp.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowPaginated, resp.ReflowableFlow)
}

// TestUpdateUserSettings_AcceptsSingleFieldPayload verifies that a payload
// containing only one field (omitting all others) updates just that field
// and leaves unrelated settings at their existing values.
func TestUpdateUserSettings_AcceptsSingleFieldPayload(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "dave")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	// Only send viewer_reflowable_theme. Everything else must keep its default.
	body := `{"viewer_reflowable_theme": "sepia"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.ReflowableThemeSepia, resp.ReflowableTheme)
	// Other fields stay at defaults
	assert.Equal(t, 3, resp.PreloadCount)
	assert.Equal(t, models.FitModeHeight, resp.FitMode)
	assert.Equal(t, 100, resp.ReflowableFontSize)
	assert.Equal(t, models.ReflowableFlowPaginated, resp.ReflowableFlow)
}

func TestUpdateUserSettings_AcceptsValidGallerySize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "gally-valid")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "l"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeLarge, resp.GallerySize)
}

func TestUpdateUserSettings_RejectsInvalidGallerySize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "gally-bad")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "huge"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gallery_size")
}

func TestGetUserSettings_DefaultsToMediumGallerySize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "gally-default")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodGet, "/settings/user", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.getUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeMedium, resp.GallerySize)
}

func TestUpdateUserSettings_AcceptsEveryReflowableTheme(t *testing.T) {
	t.Parallel()
	for _, theme := range []string{
		models.ReflowableThemeSystem,
		models.ReflowableThemeLight,
		models.ReflowableThemeSepia,
		models.ReflowableThemeDark,
		models.ReflowableThemeBlack,
	} {
		t.Run(theme, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			user := createTestUser(t, db, "reader")

			e := newTestEcho(t)
			h := &handler{settingsService: NewService(db)}

			body := `{"viewer_reflowable_theme": "` + theme + `"}`
			req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			auth.SetUser(c, user)

			require.NoError(t, h.updateUserSettings(c))
			var resp UserSettingsResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, theme, resp.ReflowableTheme)
		})
	}
}

func TestUpdateUserSettings_RejectsUnknownReflowableTheme(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "frank")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(`{"viewer_reflowable_theme": "purple"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	// The message names every theme the readers offer.
	assert.Contains(t, err.Error(), "viewer_reflowable_theme must be 'system', 'light', 'sepia', 'dark', or 'black'")
}
