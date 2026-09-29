package events

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonFlushingWriter hides the recorder's Flush method.
type nonFlushingWriter struct {
	http.ResponseWriter
}

// A response writer that cannot flush is a server fault. The handler returns
// a plain error, which the error handler renders as internal_server_error,
// rather than an Echo HTTP error whose wire code is the snake-cased message.
func TestStream_NonFlushingWriterReturnsInternalError(t *testing.T) {
	t.Parallel()
	h := &handler{broker: NewBroker()}
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	c := echo.New().NewContext(req, nonFlushingWriter{httptest.NewRecorder()})
	auth.SetUser(c, &models.User{})

	err := h.stream(c)

	require.Error(t, err)
	var httpErr *echo.HTTPError
	assert.False(t, errors.As(err, &httpErr), "want a plain error, got %T", err)
	var ecErr *errcodes.Error
	assert.False(t, errors.As(err, &ecErr), "want a plain error, got %T", err)
	assert.Equal(t, "streaming not supported", err.Error())
}
