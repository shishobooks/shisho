package gettingstarted

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/uptrace/bun"
)

type handler struct {
	service  *Service
	demoMode bool
}

func newHandler(db *bun.DB, demoMode bool) *handler {
	return &handler{service: NewService(db), demoMode: demoMode}
}

// respond sends the signed-in user's list, which every route but the admin's
// returns. Demo Mode reports it off without reading anything.
func (h *handler) respond(c echo.Context) error {
	if h.demoMode {
		return errors.WithStack(c.JSON(http.StatusOK, GettingStartedResponse{Steps: []GettingStartedStep{}}))
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	resp, err := h.service.Status(c.Request().Context(), user)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) retrieve(c echo.Context) error {
	return h.respond(c)
}

// update runs fn for the signed-in user, then responds with their list.
func (h *handler) update(c echo.Context, fn func(userID int) error) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := fn(user.ID); err != nil {
		return err
	}
	return h.respond(c)
}

func (h *handler) markNotForMe(c echo.Context) error {
	return h.update(c, func(userID int) error {
		return h.service.SetNotForMe(c.Request().Context(), userID, c.Param("step"), true)
	})
}

func (h *handler) unmarkNotForMe(c echo.Context) error {
	return h.update(c, func(userID int) error {
		return h.service.SetNotForMe(c.Request().Context(), userID, c.Param("step"), false)
	})
}

func (h *handler) dismiss(c echo.Context) error {
	return h.update(c, func(userID int) error {
		return h.service.Dismiss(c.Request().Context(), userID)
	})
}

func (h *handler) restore(c echo.Context) error {
	return h.update(c, func(userID int) error {
		return h.service.Restore(c.Request().Context(), userID)
	})
}

func (h *handler) clearAutoOpen(c echo.Context) error {
	return h.update(c, func(userID int) error {
		return h.service.ClearAutoOpen(c.Request().Context(), userID)
	})
}

// show is an admin's "Show Getting Started" for another user. It responds
// with no content: the admin never sees which of the user's steps are done.
func (h *handler) show(c echo.Context) error {
	id, err := httputil.ParamID(c, "id", "User")
	if err != nil {
		return err
	}
	if err := h.service.Show(c.Request().Context(), id); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}
