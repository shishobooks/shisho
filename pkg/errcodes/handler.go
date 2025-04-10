package errcodes

import (
	"net/http"

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
	if errutils.IsIgnorableErr(err) {
		logger.FromEchoContext(c).Err(err).Warn("broken pipe")
		return
	}

	httpCode, payload := h.generatePayload(c, err)

	// Internal server errors
	if httpCode == http.StatusInternalServerError {
		logger.FromEchoContext(c).Err(err).Error("server error")
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
