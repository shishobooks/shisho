package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test-only seeding routes return errcodes bodies like every other
// route, so a missing field is bad_request rather than a snake-cased copy of
// the message.
func TestSeedingRoutes_ReturnErrcodesBodies(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(e.Group("/api"), db, nil, nil)

	tests := []struct {
		path    string
		message string
	}{
		{"/api/test/users", "Username and password are required."},
		{"/api/test/libraries", "Name is required."},
		{"/api/test/books", "libraryId and title are required."},
		{"/api/test/persons", "libraryId and name are required."},
		{"/api/test/series", "libraryId and name are required."},
		{"/api/test/api-keys", "userId and name are required."},
		{"/api/test/plugins", "scope and id are required."},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, "response body: %s", rec.Body.String())
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "bad_request", body.Error.Code)
			assert.Equal(t, tt.message, body.Error.Message)
		})
	}
}
