package errcodes

import (
	"context"
	"net/http"
	"syscall"

	"github.com/iancoleman/strcase"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/echo/v4/middleware/logger"
	"github.com/robinjoseph08/golib/errutils"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

// Handle is an Echo error handler that uses HTTP errors accordingly, and any
// generic error will be interpreted as an internal server error.
func (h *Handler) Handle(err error, c echo.Context) {
	// Silently ignore broken pipe errors - these are expected when clients
	// disconnect during streaming (e.g., navigating away while audio plays).
	// IsIgnorableErr only matches an EPIPE or ECONNRESET inside an
	// os.SyscallError, so match the bare errno as well. Only a response
	// already under way, or a request whose client is gone, can be a
	// disconnect; the same errors on a live request (a truncated archive
	// entry's io.ErrUnexpectedEOF) are server faults, and ignoring them
	// would send an empty 200.
	disconnectLike := errutils.IsIgnorableErr(err) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
	if disconnectLike && (c.Response().Committed || c.Request().Context().Err() != nil) {
		return
	}

	// Silently ignore context canceled errors - these are expected when clients
	// disconnect before the request completes (e.g., navigating away quickly)
	if errors.Is(err, context.Canceled) {
		return
	}

	httpCode, payload := h.generatePayload(c, err)

	// Internal server errors
	if httpCode == http.StatusInternalServerError {
		logger.FromEchoContext(c).Err(err).Error("server error")
	}

	// A handler that fails after the response started, such as a stream cut
	// off partway, can no longer change the status. Writing the JSON error
	// would append it to the partial body the client is reading.
	if c.Response().Committed {
		return
	}

	if err := c.JSON(httpCode, payload); err != nil {
		logger.FromEchoContext(c).Err(errors.WithStack(err)).Error("error handler json error")
	}
}

func (h *Handler) generatePayload(c echo.Context, err error) (int, map[string]interface{}) {
	return h.generateIndividualPayload(c, err)
}

func (h *Handler) generateIndividualPayload(_ echo.Context, err error) (int, map[string]interface{}) {
	code := ""
	msg := ""
	httpCode := http.StatusInternalServerError

	// Echo errors
	var he *echo.HTTPError
	if ok := errors.As(err, &he); ok {
		httpCode = he.Code
		msg = he.Message.(string)
		code = strcase.ToSnake(msg)
	}

	// Custom errors
	var e *Error
	if ok := errors.As(err, &e); ok {
		httpCode = e.HTTPCode
		code = e.Code
		msg = e.Message
	}

	// Internal server errors that aren't Echo errors or custom errors
	if httpCode == http.StatusInternalServerError && msg == "" {
		code = "internal_server_error"
		msg = "Internal Server Error"
	}

	return httpCode, map[string]interface{}{
		"error": map[string]interface{}{
			"code":        code,
			"message":     msg,
			"status_code": httpCode,
		},
	}
}
