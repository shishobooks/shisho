package audnexus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ServiceConfig holds options for constructing a Service.
type ServiceConfig struct {
	// HTTPClient is optional; if nil, a client with a 5-second timeout is used.
	HTTPClient *http.Client
	// BaseURL overrides the Audnexus endpoint for tests. Defaults to the real
	// API when empty.
	BaseURL string
	// UserAgent sent on every upstream request. Defaults to "Shisho/unknown".
	UserAgent string
	// CacheTTL for successful responses. Defaults to 24 hours if zero.
	CacheTTL time.Duration
}

// Service fetches chapter data from Audnexus with in-memory caching.
type Service struct {
	http      *http.Client
	baseURL   string
	userAgent string
	cacheTTL  time.Duration
	mu        sync.RWMutex
	cache     map[string]*cacheEntry
}

type cacheEntry struct {
	value    *Response
	expireAt time.Time
}

const defaultBaseURL = "https://api.audnex.us"

// NewService constructs a Service with the given config.
func NewService(cfg ServiceConfig) *Service {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "Shisho/unknown"
	}
	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &Service{
		http:      client,
		baseURL:   strings.TrimRight(base, "/"),
		userAgent: ua,
		cacheTTL:  ttl,
		cache:     make(map[string]*cacheEntry),
	}
}

var asinPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// normalizeASIN uppercases and returns the ASIN if it passes format checks,
// otherwise returns the empty string.
func normalizeASIN(asin string) string {
	normalized := strings.ToUpper(strings.TrimSpace(asin))
	if !asinPattern.MatchString(normalized) {
		return ""
	}
	return normalized
}

// GetChapters fetches chapter data for an ASIN. Returns a typed *Error on
// failure (check with AsAudnexusError).
func (s *Service) GetChapters(ctx context.Context, asin string) (*Response, error) {
	normalized := normalizeASIN(asin)
	if normalized == "" {
		return nil, newErr(ErrCodeInvalidASIN, "ASIN must be 10 alphanumeric characters")
	}

	if cached := s.cacheGet(normalized); cached != nil {
		return cached, nil
	}

	url := s.baseURL + "/books/" + normalized + "/chapters"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, newErr(ErrCodeUpstreamError, err.Error())
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.http.Do(req)
	if err != nil {
		if ctx.Err() != nil || isTimeout(err) {
			return nil, newErr(ErrCodeTimeout, err.Error())
		}
		return nil, newErr(ErrCodeUpstreamError, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, newErr(ErrCodeNotFound, "ASIN not found on Audible")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newErr(ErrCodeUpstreamError, "upstream returned "+resp.Status)
	}

	var upstream audnexusUpstream
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		return nil, newErr(ErrCodeUpstreamError, "invalid JSON from upstream")
	}

	out := &Response{
		ASIN:                 upstream.ASIN,
		IsAccurate:           upstream.IsAccurate,
		RuntimeLengthMs:      upstream.RuntimeLengthMs,
		BrandIntroDurationMs: upstream.BrandIntroDurationMs,
		BrandOutroDurationMs: upstream.BrandOutroDurationMs,
		Chapters:             make([]Chapter, 0, len(upstream.Chapters)),
	}
	for _, c := range upstream.Chapters {
		out.Chapters = append(out.Chapters, Chapter{
			Title:         c.Title,
			StartOffsetMs: c.StartOffsetMs,
			LengthMs:      c.LengthMs,
		})
	}

	s.cachePut(normalized, out)
	return out, nil
}

// isTimeout reports whether err represents a net/http timeout.
func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	var t timeout
	if errors.As(err, &t) {
		return t.Timeout()
	}
	return false
}

func (s *Service) cacheGet(asin string) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.cache[asin]
	if !ok || time.Now().After(entry.expireAt) {
		return nil
	}
	return entry.value
}

func (s *Service) cachePut(asin string, value *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[asin] = &cacheEntry{
		value:    value,
		expireAt: time.Now().Add(s.cacheTTL),
	}
}
