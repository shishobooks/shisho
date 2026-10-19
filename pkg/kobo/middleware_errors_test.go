package kobo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scope ID that does not parse names no library or list, so a migrated
// Kobo's first request returns that resource's 404, the same as an ID with
// no row.
func TestLegacyURLScope_NonNumericScopeIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	for scopeType, resource := range map[string]string{"library": "Library", "list": "List"} {
		t.Run(scopeType, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames("scopeId")
			c.SetParamValues("abc")
			devices.SetDevice(c, &devices.Device{ID: "migrated", URLScopePending: true})

			err := legacyURLScope(nil, scopeType)(func(echo.Context) error { return nil })(c)

			var ecErr *errcodes.Error
			require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
			assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
			assert.Equal(t, "not_found", ecErr.Code)
			assert.Equal(t, resource+" not found.", ecErr.Message)
		})
	}
}
