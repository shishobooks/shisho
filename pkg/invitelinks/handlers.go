package invitelinks

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	service     *Service
	authService *auth.Service
}

func buildInviteLinkResponse(link *models.InviteLink) InviteLinkResponse {
	resp := InviteLinkResponse{InviteLink: *link, LibraryIDs: link.LibraryIDs()}
	if link.CreatedByUser != nil {
		resp.CreatedByUsername = link.CreatedByUser.Username
	}
	return resp
}

func (h *handler) create(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	payload := CreateInviteLinkPayload{}
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	link, err := h.service.Create(c.Request().Context(), CreateOptions{
		CreatedByUserID:  user.ID,
		RoleID:           payload.RoleID,
		AllLibraryAccess: payload.AllLibraryAccess,
		LibraryIDs:       payload.LibraryIDs,
		ExpiresAt:        time.Now().AddDate(0, 0, payload.ExpiresInDays),
	})
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusCreated, buildInviteLinkResponse(link)))
}

func (h *handler) list(c echo.Context) error {
	links, err := h.service.ListPending(c.Request().Context())
	if err != nil {
		return err
	}
	resp := make([]InviteLinkResponse, 0, len(links))
	for _, link := range links {
		resp = append(resp, buildInviteLinkResponse(link))
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) revoke(c echo.Context) error {
	id, err := httputil.ParamID(c, "id", "Invite Link")
	if err != nil {
		return err
	}
	if err := h.service.Revoke(c.Request().Context(), id); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// lookup tells whoever opened an invite whether they can accept it and who
// sent it. It needs no account.
func (h *handler) lookup(c echo.Context) error {
	resp, err := h.service.Lookup(c.Request().Context(), c.Param("token"))
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// accept creates the invite's user and signs them in on this browser,
// replacing any session it had.
func (h *handler) accept(c echo.Context) error {
	ctx := c.Request().Context()
	payload := AcceptInvitePayload{}
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	var email *string
	if payload.Email != "" {
		email = &payload.Email
	}
	userID, err := h.service.Accept(ctx, c.Param("token"), AcceptOptions{
		Username: payload.Username,
		Email:    email,
		Password: payload.Password,
	})
	if err != nil {
		return err
	}
	user, err := h.authService.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := h.authService.StartSession(c, user); err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusCreated, auth.BuildMeResponse(user)))
}
