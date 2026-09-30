package errcodes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A handler that fails after the response has started, such as a stream cut
// off partway, cannot change the status any more. Writing the JSON error then
// would append it to the partial body the client is already reading.
func TestHandle_CommittedResponseGetsNoBody(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.Response().Header().Set(echo.HeaderContentType, "audio/mp4")
	c.Response().WriteHeader(http.StatusPartialContent)
	_, err := c.Response().Write([]byte("first bytes"))
	require.NoError(t, err)

	NewHandler().Handle(errors.New("read failed"), c)
	NewHandler().Handle(errors.WithStack(io.ErrUnexpectedEOF), c)

	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "first bytes", rec.Body.String())
}

// A client that goes away (a seek in the audio player, a closed tab) is not a
// server fault, however the write error is wrapped: its request context is
// done, and there is no one left to answer.
func TestHandle_ClientDisconnectIsIgnored(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"bare EPIPE", errors.WithStack(syscall.EPIPE)},
		{"bare ECONNRESET", errors.WithStack(syscall.ECONNRESET)},
		{"syscall error", &os.SyscallError{Syscall: "write", Err: syscall.EPIPE}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), rec)

			NewHandler().Handle(tt.err, c)

			assert.False(t, c.Response().Committed)
			assert.Empty(t, rec.Body.String())
		})
	}
}

// An error that looks like a disconnect but reaches a live, uncommitted
// request is a server fault, such as a truncated CBZ entry failing with
// io.ErrUnexpectedEOF. Swallowing it would send an empty 200.
func TestHandle_DisconnectLikeErrorOnLiveRequestIsServerError(t *testing.T) {
	t.Parallel()
	for _, err := range []error{errors.WithStack(io.ErrUnexpectedEOF), errors.WithStack(syscall.EPIPE)} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

		NewHandler().Handle(err, c)

		assert.Equal(t, http.StatusInternalServerError, rec.Code, err.Error())
		assert.Contains(t, rec.Body.String(), `"internal_server_error"`, err.Error())
	}
}

func TestHandle_WritesJSONError(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

	NewHandler().Handle(NotFound("File"), c)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"not_found","message":"File not found.","status_code":404}}`, rec.Body.String())
}
