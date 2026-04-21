package audnexus

import (
	"context"
	stderrors "errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type handler struct {
	service *Service
}

func (h *handler) getChapters(c echo.Context) error {
	asin := c.Param("asin")
	resp, err := h.service.GetChapters(c.Request().Context(), asin)
	if err != nil {
		// If the request was canceled (client disconnect), don't try to
		// produce a response — nobody is listening.
		if stderrors.Is(err, context.Canceled) {
			return nil
		}
		return mapServiceError(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// mapServiceError converts an audnexus *Error into an errcodes response that
// carries the audnexus-specific code as `Code` (so the frontend can map it to
// a user-facing message) alongside the appropriate HTTP status.
//
// The generic errcodes helpers (BadRequest, NotFound, etc.) would set Code to
// the HTTP family (e.g. "bad_request") and stuff the audnexus slug in
// Message, which broke the frontend's code-based switch. Building the
// errcodes.Error directly keeps the audnexus slug in Code end-to-end.
//
// Non-typed errors bubble up as 502 (upstream_error); they aren't expected in
// practice, so log when one shows up so future regressions surface.
func mapServiceError(err error) error {
	e := AsAudnexusError(err)
	if e == nil {
		slog.Warn("audnexus: unexpected non-typed service error", "err", err.Error())
		return &errcodes.Error{
			HTTPCode: http.StatusBadGateway,
			Message:  string(ErrCodeUpstreamError),
			Code:     string(ErrCodeUpstreamError),
		}
	}
	status := http.StatusBadGateway
	switch e.Code {
	case ErrCodeInvalidASIN:
		status = http.StatusBadRequest
	case ErrCodeNotFound:
		status = http.StatusNotFound
	case ErrCodeTimeout:
		status = http.StatusGatewayTimeout
	case ErrCodeUpstreamError:
		status = http.StatusBadGateway
	}
	return &errcodes.Error{
		HTTPCode: status,
		Message:  string(e.Code),
		Code:     string(e.Code),
	}
}
