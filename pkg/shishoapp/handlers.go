package shishoapp

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/bookmarks"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

const (
	// defaultFeedLimit is the page size when the app names none.
	defaultFeedLimit = 500
	// feedBudget is how long a page's documents may take to build before
	// the page ends early, well inside the app's 30-second request timeout.
	// Opening every CBZ of a large comics Library on a slow disk can take
	// longer, and a page that always timed out would never move the sync on.
	feedBudget = 10 * time.Second
	// feedChunk is how many entries are built between checks of the budget,
	// small enough that a first chunk of CBZs read from a slow disk still
	// ends well inside the app's timeout.
	feedChunk = 25
)

// Options configures the Device surface.
type Options struct {
	Contract *Contract
	// ServerVersion is the Shisho release, told only to an app that signed
	// in, as the KOReader plugin is told it.
	ServerVersion string
	DemoMode      bool
	// InstanceID is the server's instance id (pkg/serverinstance).
	InstanceID string
	// Thumbnails is the server's one cover thumbnail cache.
	Thumbnails *covers.ThumbnailCache
	// Books loads a Book with everything its Generated File is built from,
	// and Downloads is the server's one download cache, which builds it.
	Books     *books.Service
	Downloads *downloadcache.Cache
	// Pages serves a CBZ's or PDF's pages, as the web's page route does.
	Pages books.PageImages
}

type handler struct {
	db        *bun.DB
	books     *books.Service
	downloads *downloadcache.Cache
	auth      *auth.Service
	devices   *devices.Service
	feed      *changefeed.Service
	reading   *reading.Service
	bookmarks *bookmarks.Service
	contract  *Contract
	opts      Options
	// feedBudget and feedChunk replace the package's defaults in tests; a
	// negative budget ends every page after its first chunk.
	feedBudget time.Duration
	feedChunk  int
}

func (h *handler) info(c echo.Context) ServerInfo {
	return h.contract.info(requestProtocol(c), h.opts.DemoMode, covers.ThumbnailRenderKey, h.opts.InstanceID)
}

// requireInstance refuses a sync for an account made on another instance
// of the server (HeaderInstance), and tells every sync this one's. An app
// that names no instance, as one from before instance ids, is let through.
func (h *handler) requireInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set(HeaderInstance, h.opts.InstanceID)
		if named := c.Request().Header.Get(HeaderInstance); named != "" && named != h.opts.InstanceID {
			return errcodes.ServerReplaced()
		}
		return next(c)
	}
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
		access, err := h.feed.Access(ctx, tx, user)
		if err != nil {
			return err
		}
		var page *changefeed.Page
		behind := []int{}
		if query.LibraryID > 0 || query.ListID > 0 {
			scope := changefeed.Scope{LibraryID: query.LibraryID, ListID: query.ListID}
			page, err = h.feed.ScopedPage(ctx, tx, access, scope, query.Cursor, limit)
		} else {
			page, err = h.feed.Page(ctx, tx, user, query.Cursor, query.ExpiredThrough, limit)
			if err == nil {
				behind, err = h.feed.Behind(ctx, tx, access.LibraryIDs, query.Cursor)
			}
		}
		if errors.Is(err, changefeed.ErrOutOfReach) {
			if query.LibraryID > 0 {
				return errcodes.NotFound("Library")
			}
			return errcodes.NotFound("List")
		}
		if err != nil {
			return err
		}
		entries, built, err := h.buildWithin(ctx, tx, user, page.Entries)
		if err != nil {
			return err
		}
		if built < len(page.Entries) {
			page.Cursor = page.Entries[built-1].Seq
			page.HasMore = true
		}
		resp = FeedResponse{
			Entries: entries, Cursor: page.Cursor, HasMore: page.HasMore,
			StartOver: page.StartOver, ExpiredThrough: page.ExpiredThrough,
			Access: FeedAccess{LibraryIDs: access.LibraryIDs, ListIDs: access.ListIDs, BehindLibraryIDs: behind},
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

// buildWithin builds the documents of entries a chunk at a time, stopping
// after the chunk that runs past the budget. It returns how many entries it
// built, always at least the first chunk, so every page moves the cursor.
func (h *handler) buildWithin(ctx context.Context, tx bun.IDB, user *models.User, entries []*changefeed.Entry) ([]FeedEntry, int, error) {
	budget, chunk := feedBudget, feedChunk
	if h.feedBudget != 0 {
		budget = h.feedBudget
	}
	if h.feedChunk > 0 {
		chunk = h.feedChunk
	}
	start := time.Now()
	out := make([]FeedEntry, 0, len(entries))
	for i := 0; i < len(entries); i += chunk {
		if i > 0 && time.Since(start) > budget {
			return out, i, nil
		}
		part, err := buildEntries(ctx, tx, user, entries[i:min(i+chunk, len(entries))])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, part...)
	}
	return out, len(entries), nil
}
