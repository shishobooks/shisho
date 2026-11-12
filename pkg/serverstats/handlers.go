package serverstats

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cache"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
)

// CacheSizer reports each cache's size, as Administration > Cache does.
type CacheSizer interface {
	Sizes() ([]cache.Info, error)
}

type handler struct {
	db           *bun.DB
	databasePath string
	cacheDir     string
	caches       CacheSizer
	// statVolume, statTimeout, now, and location are fields so tests can
	// fix them.
	statVolume func(path string, mustExist bool) (volumeStat, error)
	// statTimeout is how long one volume may take to answer.
	statTimeout time.Duration
	now         func() time.Time
	// location is the server's time zone, which "this year" and the
	// growth months are counted in.
	location *time.Location
}

func newHandler(db *bun.DB, cfg *config.Config, caches CacheSizer) *handler {
	return &handler{
		db:           db,
		databasePath: cfg.DatabaseFilePath,
		cacheDir:     cfg.CacheDir,
		caches:       caches,
		statVolume:   statVolume,
		statTimeout:  5 * time.Second,
		now:          time.Now,
		location:     time.Local,
	}
}

func (h *handler) retrieve(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	resp, err := h.compute(c.Request().Context(), user)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *handler) retrieveReading(c echo.Context) error {
	var params ServerReadingQuery
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.reading(c.Request().Context(), params.Year)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, resp)
}
