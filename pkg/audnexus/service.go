package audnexus

import (
	"context"
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
	// Upstream call will be implemented in the next task.
	return nil, newErr(ErrCodeUpstreamError, "not implemented")
}
