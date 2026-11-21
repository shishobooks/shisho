package shishoapp

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

const (
	// defaultFeedLimit is the page size when the app names none.
	defaultFeedLimit = 500
)

// Options configures the Device surface.
type Options struct {
	Contract *Contract
	// ServerVersion is the Shisho release, told only to an app that signed
	// in, as the KOReader plugin is told it.
	ServerVersion string
	DemoMode      bool
	// Thumbnails is the server's one cover thumbnail cache.
	Thumbnails *covers.ThumbnailCache
}

type handler struct {
	db       *bun.DB
	auth     *auth.Service
	devices  *devices.Service
	feed     *changefeed.Service
	reading  *reading.Service
	contract *Contract
	opts     Options
}

func (h *handler) info(c echo.Context) ServerInfo {
	return h.contract.info(requestProtocol(c), h.opts.DemoMode, covers.ThumbnailRenderKey)
}

// requireProtocol refuses an app whose protocol the server doesn't accept.
// It runs after signing in, so only an account's owner learns the server's
// version.
func (h *handler) requireProtocol(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if protocol := requestProtocol(c); !h.contract.accepts(protocol) {
			return h.contract.refuseProtocol(c, protocol, h.opts.ServerVersion)
		}
		return next(c)
	}
}

// recordAppVersion stores the app version a signed-in request reports on
// its Device, so the Device list shows the version in use.
func (h *handler) recordAppVersion(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		device, err := devices.RequireDevice(c)
		if err != nil {
			return err
		}
		version := strings.TrimSpace(c.Request().Header.Get(HeaderAppVersion))
		if runes := []rune(version); len(runes) > 64 {
			version = string(runes[:64])
		}
		if err := h.devices.RecordAppVersion(c.Request().Context(), device, version); err != nil {
			return err
		}
		return next(c)
	}
}

// server answers what the server accepts and offers. It needs no sign-in
// and accepts any protocol, so an app can learn it's too old or too new.
func (h *handler) server(c echo.Context) error {
	return errors.WithStack(c.JSON(http.StatusOK, h.info(c)))
}

// signIn checks a username and password and creates a Shisho app Device
// for the installation. A Demo Mode server refuses it before it runs, as
// it refuses every write. The protocol is checked after the password, so
// only the account's owner learns the server's version.
func (h *handler) signIn(c echo.Context) error {
	ctx := c.Request().Context()
	var payload SignInPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	user, err := h.auth.Authenticate(ctx, payload.Username, payload.Password)
	if err != nil {
		return err
	}
	if protocol := requestProtocol(c); !h.contract.accepts(protocol) {
		return h.contract.refuseProtocol(c, protocol, h.opts.ServerVersion)
	}
	// The password signs in only once it's changed, and a Device made now
	// would outlive the reset.
	if user.MustChangePassword {
		return errcodes.PasswordResetRequired()
	}
	if !user.HasPermission(models.ResourceBooks, models.OperationRead) {
		return errcodes.PermissionDenied(models.ResourceBooks, models.OperationRead)
	}
	device, err := h.devices.CreateAppDevice(ctx, user.ID, devices.AppDeviceOptions{
		Name:           payload.DeviceName,
		InstallationID: payload.InstallationID,
		AppVersion:     payload.AppVersion,
	})
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, SignInResponse{
		Credential:    device.Credential,
		DeviceID:      device.ID,
		DeviceName:    device.Name,
		User:          AppUser{ID: user.ID, Username: user.Username},
		ServerVersion: h.opts.ServerVersion,
		Server:        h.info(c),
	}))
}

// signOut deletes the signed-in Device, so its credential stops working.
// It accepts any protocol, so an app too old for the server can still sign
// out.
func (h *handler) signOut(c echo.Context) error {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	if err := h.devices.Delete(c.Request().Context(), device.UserID, device.ID); err != nil && !errors.Is(err, devices.ErrNotFound) {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// changes answers the catalog's changes after a cursor, as the signed-in
// user may see them (ADR 0023).
func (h *handler) changes(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var query FeedQuery
	if err := c.Bind(&query); err != nil {
		return errors.WithStack(err)
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultFeedLimit
	}
	if err := h.feed.ExpireIfDue(ctx); err != nil {
		return err
	}

	var resp FeedResponse
	err = h.feed.Read(ctx, func(ctx context.Context, tx bun.Tx) error {
		page, err := h.feed.Page(ctx, tx, user, query.Cursor, query.ExpiredThrough, limit)
		if err != nil {
			return err
		}
		entries, err := buildEntries(ctx, tx, user.ID, page.Entries)
		if err != nil {
			return err
		}
		resp = FeedResponse{
			Entries: entries, Cursor: page.Cursor, HasMore: page.HasMore,
			StartOver: page.StartOver, ExpiredThrough: page.ExpiredThrough,
		}
		return nil
	})
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// cover serves a Book's cover, resized when the app asks for a size. A
// URL keyed with the Book's cover version is cached as immutable. A Book
// outside the user's Libraries is a 404, never a 403, so its id reveals
// nothing.
func (h *handler) cover(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	id, err := httputil.ParamID(c, "id", "Cover")
	if err != nil {
		return err
	}
	book := new(models.Book)
	err = h.db.NewSelect().Model(book).Relation("Library").Relation("Files").Where("b.id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return errcodes.NotFound("Cover")
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.NotFound("Cover")
	}
	aspect := ""
	if book.Library != nil {
		aspect = book.Library.CoverAspectRatio
	}
	cacheControl := covers.CacheControlNoCache
	if c.QueryParam("v") != "" {
		cacheControl = covers.CacheControlImmutable
	}
	return covers.ServeBookCover(c, book.Files, aspect, cacheControl, "Cover", h.opts.Thumbnails)
}
