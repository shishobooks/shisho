package cache

import (
	"context"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/covers"
)

const settingsKey = "cache"

// LoadSettings reads saved policy without persisting defaults on a GET.
func LoadSettings(ctx context.Context, store *appsettings.Service) (SettingsResponse, error) {
	s := SettingsResponse{CoverThumbnailMaxSizeGB: float64(covers.DefaultThumbnailCacheMaxBytes) / (1 << 30)}
	_, err := store.GetJSON(ctx, settingsKey, &s)
	return s, errors.WithStack(err)
}

type settingsHandler struct {
	store      *appsettings.Service
	thumbnails *covers.ThumbnailCache
	mu         sync.Mutex
}

func (h *settingsHandler) get(c echo.Context) error {
	s, err := LoadSettings(c.Request().Context(), h.store)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, s)
}

func (h *settingsHandler) update(c echo.Context) error {
	var payload UpdateSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ctx := c.Request().Context()
	s, err := LoadSettings(ctx, h.store)
	if err != nil {
		return err
	}
	if payload.CoverThumbnailMaxSizeGB != nil {
		s.CoverThumbnailMaxSizeGB = *payload.CoverThumbnailMaxSizeGB
		if err := h.store.SetJSON(ctx, settingsKey, s); err != nil {
			return errors.WithStack(err)
		}
		if err := h.thumbnails.SetMaxBytes(int64(s.CoverThumbnailMaxSizeGB * (1 << 30))); err != nil {
			return err
		}
	}
	return c.JSON(http.StatusOK, s)
}
