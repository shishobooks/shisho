package sharelinks

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// handler serves the authenticated management routes under
// /books/:id/share-links.
type handler struct {
	service            *Service
	appSettingsService *appsettings.Service
}

// requireBookAccess returns the book ID from the path after checking that
// the book exists and the user can reach its library.
func (h *handler) requireBookAccess(c echo.Context) (int, *models.User, error) {
	bookID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return 0, nil, errcodes.NotFound("Book")
	}
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return 0, nil, errcodes.Unauthorized("User not found in context")
	}
	libraryID, err := h.service.BookLibraryID(c.Request().Context(), bookID)
	if err != nil {
		return 0, nil, err
	}
	if !user.HasLibraryAccess(libraryID) {
		return 0, nil, errcodes.Forbidden("You don't have access to this library")
	}
	return bookID, user, nil
}

func (h *handler) list(c echo.Context) error {
	bookID, _, err := h.requireBookAccess(c)
	if err != nil {
		return err
	}
	links, err := h.service.ListForBook(c.Request().Context(), bookID)
	if err != nil {
		return err
	}
	now := time.Now()
	resp := make([]ShareLinkResponse, 0, len(links))
	for _, link := range links {
		resp = append(resp, newShareLinkResponse(link, now))
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()
	bookID, user, err := h.requireBookAccess(c)
	if err != nil {
		return err
	}

	var payload CreateShareLinkPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	settings, err := h.requireSharingEnabled(ctx)
	if err != nil {
		return err
	}
	if payload.ExpiresAt == nil && settings.RequireExpiration {
		return errcodes.ValidationError("An expiration is required for new share links.")
	}
	now := time.Now()
	if payload.ExpiresAt != nil && !payload.ExpiresAt.After(now) {
		return errcodes.ValidationError("The expiration must be in the future.")
	}

	label := payload.Label
	if label != nil && *label == "" {
		label = nil
	}
	link, err := h.service.Create(ctx, CreateOptions{
		BookID:          bookID,
		CreatedByUserID: user.ID,
		Label:           label,
		ExpiresAt:       payload.ExpiresAt,
	})
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusCreated, newShareLinkResponse(link, now)))
}

// requireWritableLink checks book access and that sharing is enabled, then
// returns the link named in the path, which must belong to the book.
func (h *handler) requireWritableLink(c echo.Context) (*models.ShareLink, error) {
	ctx := c.Request().Context()
	bookID, _, err := h.requireBookAccess(c)
	if err != nil {
		return nil, err
	}
	if _, err := h.requireSharingEnabled(ctx); err != nil {
		return nil, err
	}
	linkID, err := strconv.Atoi(c.Param("linkId"))
	if err != nil {
		return nil, errcodes.NotFound("Share Link")
	}
	return h.service.RetrieveForBook(ctx, bookID, linkID)
}

// requireSharingEnabled refuses every management write while sharing is off
// and returns the settings for callers that need the rest of the policy.
func (h *handler) requireSharingEnabled(ctx context.Context) (Settings, error) {
	settings, err := LoadSettings(ctx, h.appSettingsService)
	if err != nil {
		return settings, err
	}
	if !settings.Enabled {
		return settings, errcodes.Forbidden("Sharing is turned off. An admin can turn it on in Settings > Sharing.")
	}
	return settings, nil
}

func (h *handler) revoke(c echo.Context) error {
	link, err := h.requireWritableLink(c)
	if err != nil {
		return err
	}
	revoked, err := h.service.Revoke(c.Request().Context(), link)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, newShareLinkResponse(revoked, time.Now())))
}

func (h *handler) delete(c echo.Context) error {
	link, err := h.requireWritableLink(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.Request().Context(), link); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

func newShareLinkResponse(link *models.ShareLink, now time.Time) ShareLinkResponse {
	resp := ShareLinkResponse{ShareLink: *link, State: link.State(now)}
	if link.CreatedByUser != nil {
		resp.CreatedByUsername = link.CreatedByUser.Username
	}
	return resp
}
