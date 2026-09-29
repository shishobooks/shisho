package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test-only seeding routes return errcodes bodies like every other
// route. The server's binder validates the required fields, so an empty body
// is a validation_error rather than a snake-cased copy of a message.
func TestSeedingRoutes_ReturnErrcodesBodies(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(e.Group("/api"), db, nil, nil, "")

	paths := []string{
		"/api/test/users",
		"/api/test/libraries",
		"/api/test/books",
		"/api/test/persons",
		"/api/test/series",
		"/api/test/api-keys",
		"/api/test/plugins",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "response body: %s", rec.Body.String())
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "validation_error", body.Error.Code)
			assert.NotEmpty(t, body.Error.Message)
		})
	}
}

// A body that is not JSON reaches the seeding handler as the binder's own
// errcodes error, which the handler returns unchanged.
func TestSeedingRoutes_MalformedBodyReturnsBinderError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(e.Group("/api"), db, nil, nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/test/users", strings.NewReader(`{"username":`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "response body: %s", rec.Body.String())
	assert.Equal(t, http.StatusBadRequest, rec.Code, "response body: %s", rec.Body.String())
	assert.Equal(t, "malformed_payload", body.Error.Code)
	assert.Equal(t, "Malformed Payload", body.Error.Message)
}
