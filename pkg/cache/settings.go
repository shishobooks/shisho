package cache

import (
	"context"
	stderrors "errors"
	"math"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/covers"
)

const settingsKey = "cache"

// defaultDownloadsMaxSizeGB is the download cache limit until an admin saves
// one.
const defaultDownloadsMaxSizeGB = 5

// defaultPageCacheMaxSizeGB is the size the page caches offer when their
// default of no limit is turned off.
const defaultPageCacheMaxSizeGB = 5

// Bytes returns the budget a cache enforces: math.MaxInt64 when unlimited,
// which no cache can exceed.
func (l Limit) Bytes() int64 {
	if l.Unlimited {
		return math.MaxInt64
	}
	return int64(max(0, l.MaxSizeGB) * (1 << 30))
}

// Limiter is a cache whose disk budget can change while the server runs.
type Limiter interface {
	SetMaxBytes(maxBytes int64) error
}

// Limiters are the caches the settings control, one per settings field. A nil
// limiter is skipped.
type Limiters struct {
	CoverThumbnails Limiter
	Downloads       Limiter
	CBZPages        Limiter
	PDFPages        Limiter
}

// Apply sets each named cache's limit from s, or every cache's when no id is
// given. It applies them all even when one fails and returns the failures
// together, so one cache that cannot be cleaned does not leave the others on
// their old limits.
func (l Limiters) Apply(s SettingsResponse, ids ...string) error {
	caches, limits := l.byID(), s.byID()
	if len(ids) == 0 {
		for id := range caches {
			ids = append(ids, id)
		}
	}
	var errs []error
	for _, id := range ids {
		if caches[id] == nil {
			continue
		}
		if err := caches[id].SetMaxBytes(limits[id].Bytes()); err != nil {
			errs = append(errs, errors.Wrapf(err, "apply %s limit", id))
		}
	}
	return stderrors.Join(errs...)
}

// savedSettings is the stored document. It holds only the limits an admin
// saved, so an unsaved limit follows a changed default after an upgrade.
type savedSettings struct {
	CoverThumbnails *Limit `json:"cover_thumbnails,omitempty"`
	Downloads       *Limit `json:"downloads,omitempty"`
	CBZPages        *Limit `json:"cbz_pages,omitempty"`
	PDFPages        *Limit `json:"pdf_pages,omitempty"`
}

// The byID methods key each shape by the cache ids in GET /cache, so code
// that walks the caches never depends on field order.
// TestSettingsShapesCoverEveryCache keeps them in step.

func (s *SettingsResponse) byID() map[string]*Limit {
	return map[string]*Limit{
		"cover_thumbnails": &s.CoverThumbnails,
		"downloads":        &s.Downloads,
		"cbz_pages":        &s.CBZPages,
		"pdf_pages":        &s.PDFPages,
	}
}

func (s *savedSettings) byID() map[string]**Limit {
	return map[string]**Limit{
		"cover_thumbnails": &s.CoverThumbnails,
		"downloads":        &s.Downloads,
		"cbz_pages":        &s.CBZPages,
		"pdf_pages":        &s.PDFPages,
	}
}

func (p *UpdateSettingsPayload) byID() map[string]*LimitPayload {
	return map[string]*LimitPayload{
		"cover_thumbnails": p.CoverThumbnails,
		"downloads":        p.Downloads,
		"cbz_pages":        p.CBZPages,
		"pdf_pages":        p.PDFPages,
	}
}

func (l Limiters) byID() map[string]Limiter {
	return map[string]Limiter{
		"cover_thumbnails": l.CoverThumbnails,
		"downloads":        l.Downloads,
		"cbz_pages":        l.CBZPages,
		"pdf_pages":        l.PDFPages,
	}
}

// LoadSettings reads saved policy without persisting defaults on a GET.
func LoadSettings(ctx context.Context, store *appsettings.Service) (SettingsResponse, error) {
	var saved savedSettings
	if _, err := store.GetJSON(ctx, settingsKey, &saved); err != nil {
		return SettingsResponse{}, errors.WithStack(err)
	}
	return saved.merge(), nil
}

// merge fills the limits an admin has not saved with their defaults.
func (s *savedSettings) merge() SettingsResponse {
	out := SettingsResponse{
		CoverThumbnails: Limit{MaxSizeGB: float64(covers.DefaultThumbnailCacheMaxBytes) / (1 << 30)},
		Downloads:       Limit{MaxSizeGB: defaultDownloadsMaxSizeGB},
		CBZPages:        Limit{MaxSizeGB: defaultPageCacheMaxSizeGB, Unlimited: true},
		PDFPages:        Limit{MaxSizeGB: defaultPageCacheMaxSizeGB, Unlimited: true},
	}
	current := out.byID()
	for id, limit := range s.byID() {
		if *limit != nil {
			*current[id] = **limit
		}
	}
	return out
}

type settingsHandler struct {
	store  *appsettings.Service
	caches Limiters
	mu     sync.Mutex
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
	var saved savedSettings
	if _, err := h.store.GetJSON(ctx, settingsKey, &saved); err != nil {
		return errors.WithStack(err)
	}
	savedLimits := saved.byID()
	var changed []string
	for id, limit := range payload.byID() {
		if limit != nil {
			*savedLimits[id] = &Limit{MaxSizeGB: *limit.MaxSizeGB, Unlimited: *limit.Unlimited}
			changed = append(changed, id)
		}
	}
	if err := h.store.SetJSON(ctx, settingsKey, saved); err != nil {
		return errors.WithStack(err)
	}
	s := saved.merge()
	if err := h.caches.Apply(s, changed...); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, s)
}
