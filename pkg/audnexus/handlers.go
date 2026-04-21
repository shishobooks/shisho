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

// mapServiceError converts an audnexus *Error into an errcodes response with
// the right HTTP status. Non-typed errors bubble up as 502 (upstream_error);
// they aren't expected in practice, so log when one shows up so future
// regressions surface.
func mapServiceError(err error) error {
	e := AsAudnexusError(err)
	if e == nil {
		slog.Warn("audnexus: unexpected non-typed service error", "err", err.Error())
		return errcodes.BadGateway(string(ErrCodeUpstreamError))
	}
	switch e.Code {
	case ErrCodeInvalidASIN:
		return errcodes.BadRequest(string(e.Code))
	case ErrCodeNotFound:
		return errcodes.NotFound(string(e.Code))
	case ErrCodeTimeout:
		return errcodes.GatewayTimeout(string(e.Code))
	case ErrCodeUpstreamError:
		return errcodes.BadGateway(string(e.Code))
	default:
		return errcodes.BadGateway(string(e.Code))
	}
}
