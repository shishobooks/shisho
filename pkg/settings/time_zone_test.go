package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func putZonedSettings(t *testing.T, db *bun.DB, h *handler, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	user := createTestUser(t, db, "zoned")
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)
	return rec, h.updateUserSettings(c)
}

func TestUpdateUserSettings_TimeZone(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := &handler{settingsService: NewService(db)}

	rec, err := putZonedSettings(t, db, h, `{"time_zone": "America/Chicago"}`)
	require.NoError(t, err)
	var resp UserSettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.TimeZone)
	assert.Equal(t, "America/Chicago", *resp.TimeZone)
}

func TestUpdateUserSettings_UnsetTimeZoneIsNull(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := &handler{settingsService: NewService(db)}

	rec, err := putZonedSettings(t, db, h, `{"gallery_size": "l"}`)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), `"time_zone":null`)
}

func TestUpdateUserSettings_RejectsUnknownTimeZones(t *testing.T) {
	t.Parallel()
	for _, zone := range []string{"Mars/Olympus_Mons", "Local", ""} {
		db := testdb.New(t)
		h := &handler{settingsService: NewService(db)}
		_, err := putZonedSettings(t, db, h, `{"time_zone": "`+zone+`"}`)
		require.Error(t, err, zone)
		assert.Contains(t, err.Error(), "time_zone", zone)
	}
}
