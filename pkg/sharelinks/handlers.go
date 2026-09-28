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

// bookAccess is the book named in the path and the user who reached it.
type bookAccess struct {
	bookID    int
	libraryID int
	user      *models.User
}

// requireBookAccess resolves the book in the path after checking that it
// exists and the user can reach its library.
func (h *handler) requireBookAccess(c echo.Context) (bookAccess, error) {
	bookID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return bookAccess{}, errcodes.NotFound("Book")
	}
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return bookAccess{}, errcodes.Unauthorized("User not found in context")
	}
	libraryID, err := h.service.BookLibraryID(c.Request().Context(), bookID)
	if err != nil {
		return bookAccess{}, err
	}
	if !user.HasLibraryAccess(libraryID) {
		return bookAccess{}, errcodes.Forbidden("You don't have access to this library")
	}
	return bookAccess{bookID: bookID, libraryID: libraryID, user: user}, nil
}

func (h *handler) list(c echo.Context) error {
	access, err := h.requireBookAccess(c)
	if err != nil {
		return err
	}
	links, err := h.service.ListForBook(c.Request().Context(), access.bookID)
	if err != nil {
		return err
	}
	now := time.Now()
	resp := make([]ShareLinkResponse, 0, len(links))
	for _, link := range links {
		resp = append(resp, newShareLinkResponse(link, access.libraryID, now))
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()
	access, err := h.requireBookAccess(c)
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
		BookID:          access.bookID,
		CreatedByUserID: access.user.ID,
		Label:           label,
		ExpiresAt:       payload.ExpiresAt,
	})
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusCreated, newShareLinkResponse(link, access.libraryID, now)))
}

// requireWritableLink checks book access, then returns the link named in the
// path, which must belong to the book. It does not require sharing to be on:
// revoking and deleting must stay available while sharing is off, or an admin
// would have to turn sharing back on, and so briefly restore every link, just
// to pull one that leaked.
func (h *handler) requireWritableLink(c echo.Context) (*models.ShareLink, bookAccess, error) {
	ctx := c.Request().Context()
	access, err := h.requireBookAccess(c)
	if err != nil {
		return nil, access, err
	}
	linkID, err := strconv.Atoi(c.Param("linkId"))
	if err != nil {
		return nil, access, errcodes.NotFound("Share Link")
	}
	link, err := h.service.RetrieveForBook(ctx, access.bookID, linkID)
	return link, access, err
}

// requireSharingEnabled refuses creating a link while sharing is off and
// returns the settings for callers that need the rest of the policy.
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
	link, access, err := h.requireWritableLink(c)
	if err != nil {
		return err
	}
	revoked, err := h.service.Revoke(c.Request().Context(), link)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, newShareLinkResponse(revoked, access.libraryID, time.Now())))
}

func (h *handler) delete(c echo.Context) error {
	link, _, err := h.requireWritableLink(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.Request().Context(), link); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// newShareLinkResponse derives the link's state and, for an active link, why
// its creator has paused it. libraryID is the library of the link's book.
func newShareLinkResponse(link *models.ShareLink, libraryID int, now time.Time) ShareLinkResponse {
	resp := ShareLinkResponse{ShareLink: *link, State: link.State(now)}
	if resp.State == models.ShareLinkStateActive {
		resp.PausedReason = link.PausedReason(libraryID)
	}
	if link.CreatedByUser != nil {
		resp.CreatedByUsername = link.CreatedByUser.Username
	}
	return resp
}
