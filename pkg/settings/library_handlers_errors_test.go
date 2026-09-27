package settings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A library_id that does not parse, or is below 1, names no library, so both
// library settings handlers return the Library 404.
func TestLibrarySettingsHandlers_InvalidLibraryIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	h := &libraryHandler{}

	for _, id := range []string{"abc", "0"} {
		for name, fn := range map[string]echo.HandlerFunc{"get": h.getLibrarySettings, "update": h.updateLibrarySettings} {
			t.Run(name+" "+id, func(t *testing.T) {
				t.Parallel()
				req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				c := echo.New().NewContext(req, httptest.NewRecorder())
				c.Set("user", &models.User{})
				c.SetParamNames("library_id")
				c.SetParamValues(id)

				err := fn(c)

				var ecErr *errcodes.Error
				require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
				assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
				assert.Equal(t, "not_found", ecErr.Code)
				assert.Equal(t, "Library not found.", ecErr.Message)
			})
		}
	}
}
