package kobo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scope ID that does not parse names no library or list, so the scope
// parser returns that resource's 404, the same as an ID with no row.
func TestScopeParser_NonNumericScopeIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	for scopeType, resource := range map[string]string{"library": "Library", "list": "List"} {
		t.Run(scopeType, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames("scopeId")
			c.SetParamValues("abc")

			err := ScopeParser(scopeType)(func(echo.Context) error { return nil })(c)

			var ecErr *errcodes.Error
			require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
			assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
			assert.Equal(t, "not_found", ecErr.Code)
			assert.Equal(t, resource+" not found.", ecErr.Message)
		})
	}
}
