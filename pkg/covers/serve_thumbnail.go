package covers

import (
	"bytes"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

func serveThumbnail(c echo.Context, file *models.File, cacheControl, resource string, cache *ThumbnailCache) (bool, error) {
	value := c.QueryParam("size")
	if cache == nil || value == "" {
		return false, nil
	}
	params := CoverThumbnailQuery{Aspect: c.QueryParam("aspect"), RenderKey: c.QueryParam("r")}
	var err error
	params.Size, err = strconv.Atoi(value)
	if err != nil || !validThumbnailSize(params.Size) {
		return true, errcodes.ValidationError(ErrInvalidThumbnailSize.Error())
	}
	aspect := params.Aspect
	if aspect != "" && aspect != "book" && aspect != "square" {
		return true, errcodes.ValidationError(ErrInvalidThumbnailAspect.Error())
	}
	thumb, err := cache.GetForAspect(c.Request().Context(), file, params.Size, aspect)
	if err != nil {
		if errors.Is(err, ErrUnsupportedThumbnail) {
			return true, serveOriginalThumbnailFallback(c, file, resource)
		}
		if errors.Is(err, os.ErrNotExist) {
			return true, errcodes.NotFound(resource)
		}
		return true, err
	}
	if params.RenderKey != ThumbnailRenderKey {
		cacheControl = "private, no-store"
	}
	// Generation and cache reads succeed before any cacheable image headers are set.
	header := c.Response().Header()
	header.Set("Content-Type", "image/png")
	header.Set("Cache-Control", cacheControl)
	header.Set("ETag", strconv.Quote(thumb.Key))
	req := c.Request()
	if rng := req.Header.Get("Range"); rng != "" && !strings.HasPrefix(rng, "bytes=") {
		req.Header.Del("Range")
	}
	http.ServeContent(c.Response(), req, "cover.png", time.Time{}, bytes.NewReader(thumb.Data))
	return true, nil
}

// serveOriginalThumbnailFallback preserves formats the thumbnail decoder cannot handle.
func serveOriginalThumbnailFallback(c echo.Context, file *models.File, resource string) error {
	return errors.WithStack(httputil.ServeFile(c, FileCoverPath(file), errcodes.NotFound(resource), httputil.WithCacheControl("private, no-store")))
}
