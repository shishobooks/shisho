# Fetch Chapters from Audible Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users fetch chapter titles and timestamps from Audible (via Audnexus) on the M4B chapter edit page and stage them into the existing edit form for review before saving.

**Architecture:** New `pkg/audnexus/` package with an HTTP client, in-memory TTL cache, and a single read-only endpoint gated by `books:write`. Frontend adds a `FetchChaptersDialog` with pure-function utilities for trim detection and apply-mode transforms, wired into `FileChaptersTab` at three entry points.

**Tech Stack:** Go (Echo, standard `net/http`, `sync.RWMutex`), React 19 + TypeScript (Tanstack Query, Radix Dialog via shadcn, vitest + RTL).

**Spec:** `docs/superpowers/specs/2026-04-21-audible-chapter-fetch-design.md`

---

## Background notes

- Audnexus endpoint: `GET https://api.audnex.us/books/{ASIN}/chapters`. No auth.
- ASIN is a 10-character uppercase alphanumeric. Normalize input to uppercase before validation and cache lookup.
- Vite strips `/api` before proxying, so server routes register without the `/api` prefix. Frontend calls `/api/audnexus/books/:asin/chapters`; server sees `/audnexus/books/:asin/chapters`.
- Permission middleware pattern: `authMiddleware.Authenticate` + `authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite)`.
- Route registration lives in `pkg/server/server.go` around line 115–140. Services are constructed and wired there, not in `cmd/api/main.go`.
- Existing chapter edit state (`editedChapters`, `hasChanges`, play buttons, per-row validation) is in `app/components/files/FileChaptersTab.tsx`. Apply handlers mutate `editedChapters` via the existing `setEditedChapters` setter — no new state machine.
- `file.audiobook_duration_seconds` is available on the file model for duration comparison.
- `file.identifiers` includes ASIN when `IdentifierTypeASIN` is present. Use it to prefill the dialog input.
- Version string comes from `pkg/version.Version` (default `"dev"`, overridden at build time).

---

## Task 1: Scaffold `pkg/audnexus/` package with types and constants

**Files:**
- Create: `pkg/audnexus/types.go`
- Create: `pkg/audnexus/errors.go`

- [ ] **Step 1: Create `pkg/audnexus/types.go`**

```go
package audnexus

// Chapter is a single chapter from Audnexus.
type Chapter struct {
	Title         string `json:"title"`
	StartOffsetMs int64  `json:"start_offset_ms"`
	LengthMs      int64  `json:"length_ms"`
}

// Response is the normalized chapters response returned by the service and
// passed through to the frontend. Field names are snake_case per project
// API conventions; upstream Audnexus uses camelCase and is converted at the
// parse boundary.
type Response struct {
	ASIN                 string    `json:"asin"`
	IsAccurate           bool      `json:"is_accurate"`
	RuntimeLengthMs      int64     `json:"runtime_length_ms"`
	BrandIntroDurationMs int64     `json:"brand_intro_duration_ms"`
	BrandOutroDurationMs int64     `json:"brand_outro_duration_ms"`
	Chapters             []Chapter `json:"chapters"`
}

// audnexusUpstream matches the Audnexus API camelCase JSON shape for decoding.
type audnexusUpstream struct {
	ASIN                 string              `json:"asin"`
	IsAccurate           bool                `json:"isAccurate"`
	RuntimeLengthMs      int64               `json:"runtimeLengthMs"`
	BrandIntroDurationMs int64               `json:"brandIntroDurationMs"`
	BrandOutroDurationMs int64               `json:"brandOutroDurationMs"`
	Chapters             []audnexusUpChapter `json:"chapters"`
}

type audnexusUpChapter struct {
	Title         string `json:"title"`
	StartOffsetMs int64  `json:"startOffsetMs"`
	LengthMs      int64  `json:"lengthMs"`
}
```

- [ ] **Step 2: Create `pkg/audnexus/errors.go`**

```go
package audnexus

import "errors"

// ErrorCode is a stable string identifier used by both the service and the
// HTTP handler to map failures to user-facing responses.
type ErrorCode string

const (
	ErrCodeInvalidASIN   ErrorCode = "invalid_asin"
	ErrCodeNotFound      ErrorCode = "not_found"
	ErrCodeTimeout       ErrorCode = "timeout"
	ErrCodeUpstreamError ErrorCode = "upstream_error"
)

// Error is a typed error carrying an ErrorCode. Callers can use errors.As to
// inspect the code for mapping to HTTP status.
type Error struct {
	Code ErrorCode
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return string(e.Code) + ": " + e.Msg
	}
	return string(e.Code)
}

func newErr(code ErrorCode, msg string) error {
	return &Error{Code: code, Msg: msg}
}

// AsAudnexusError extracts an *Error if err wraps one, else returns nil.
func AsAudnexusError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./pkg/audnexus/...`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add pkg/audnexus/types.go pkg/audnexus/errors.go
git commit -m "[Backend] Scaffold audnexus package types and error codes"
```

---

## Task 2: Service skeleton with ASIN validation

**Files:**
- Create: `pkg/audnexus/service.go`
- Create: `pkg/audnexus/service_test.go`

- [ ] **Step 1: Write failing test for ASIN validation**

Create `pkg/audnexus/service_test.go`:

```go
package audnexus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_GetChapters_InvalidASIN(t *testing.T) {
	t.Parallel()
	svc := NewService(ServiceConfig{})

	cases := []string{
		"",
		"short",
		"TOO-LONG-ASIN",
		"1234567890X!",             // non-alphanumeric
		"B0036UC2L",                // 9 chars
		"B0036UC2LOX",              // 11 chars
	}
	for _, asin := range cases {
		asin := asin
		t.Run(asin, func(t *testing.T) {
			t.Parallel()
			_, err := svc.GetChapters(context.Background(), asin)
			require.Error(t, err)
			e := AsAudnexusError(err)
			require.NotNil(t, e, "expected *Error")
			assert.Equal(t, ErrCodeInvalidASIN, e.Code)
		})
	}
}

func TestService_GetChapters_NormalizesASINToUppercase(t *testing.T) {
	t.Parallel()
	// Valid lowercase ASIN should pass validation (and would reach upstream,
	// which we're not testing here — but it must not return invalid_asin).
	svc := NewService(ServiceConfig{})
	_, err := svc.GetChapters(context.Background(), "b0036uc2lo")
	if e := AsAudnexusError(err); e != nil {
		assert.NotEqual(t, ErrCodeInvalidASIN, e.Code, "lowercase ASIN should normalize to valid")
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_InvalidASIN -v`
Expected: FAIL (`NewService` and `GetChapters` not defined).

- [ ] **Step 3: Create `pkg/audnexus/service.go` with skeleton**

```go
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
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_InvalidASIN -v -count=1`
Expected: PASS.

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_NormalizesASINToUppercase -v -count=1`
Expected: PASS (error code is `upstream_error`, not `invalid_asin`).

- [ ] **Step 5: Commit**

```bash
git add pkg/audnexus/service.go pkg/audnexus/service_test.go
git commit -m "[Backend] Add audnexus service skeleton with ASIN validation"
```

---

## Task 3: Service upstream fetch (happy path)

**Files:**
- Modify: `pkg/audnexus/service.go`
- Modify: `pkg/audnexus/service_test.go`

- [ ] **Step 1: Add failing test for happy-path upstream fetch**

Append to `pkg/audnexus/service_test.go`:

```go
import (
	"net/http"
	"net/http/httptest"
	"time"
)

func TestService_GetChapters_HappyPath(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/books/B0036UC2LO/chapters", r.URL.Path)
		assert.Equal(t, "Shisho/test", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"asin": "B0036UC2LO",
			"isAccurate": true,
			"runtimeLengthMs": 163938000,
			"brandIntroDurationMs": 38000,
			"brandOutroDurationMs": 62000,
			"chapters": [
				{"title": "Prelude", "startOffsetMs": 0, "lengthMs": 272000},
				{"title": "Prologue", "startOffsetMs": 272000, "lengthMs": 1063000}
			]
		}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{
		BaseURL:   upstream.URL,
		UserAgent: "Shisho/test",
	})

	resp, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "B0036UC2LO", resp.ASIN)
	assert.True(t, resp.IsAccurate)
	assert.Equal(t, int64(163938000), resp.RuntimeLengthMs)
	assert.Equal(t, int64(38000), resp.BrandIntroDurationMs)
	assert.Equal(t, int64(62000), resp.BrandOutroDurationMs)
	require.Len(t, resp.Chapters, 2)
	assert.Equal(t, "Prelude", resp.Chapters[0].Title)
	assert.Equal(t, int64(0), resp.Chapters[0].StartOffsetMs)
	assert.Equal(t, int64(272000), resp.Chapters[0].LengthMs)
}
```

- [ ] **Step 2: Run and verify it fails**

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_HappyPath -v`
Expected: FAIL (still returns "not implemented").

- [ ] **Step 3: Replace `GetChapters` body with the real upstream call**

In `pkg/audnexus/service.go`, replace the body of `GetChapters`:

```go
func (s *Service) GetChapters(ctx context.Context, asin string) (*Response, error) {
	normalized := normalizeASIN(asin)
	if normalized == "" {
		return nil, newErr(ErrCodeInvalidASIN, "ASIN must be 10 alphanumeric characters")
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
```

Update the imports at the top of `pkg/audnexus/service.go` to:

```go
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
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_HappyPath -v -count=1`
Expected: PASS.

Run the whole package to check the earlier tests still pass:

Run: `go test ./pkg/audnexus/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/audnexus/service.go pkg/audnexus/service_test.go
git commit -m "[Backend] Implement audnexus upstream fetch"
```

---

## Task 4: Service error mapping (404, timeout, 5xx, invalid JSON)

**Files:**
- Modify: `pkg/audnexus/service_test.go`

- [ ] **Step 1: Add failing tests for each error case**

Append to `pkg/audnexus/service_test.go`:

```go
func TestService_GetChapters_NotFound(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeNotFound, AsAudnexusError(err).Code)
}

func TestService_GetChapters_UpstreamError_5xx(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_InvalidJSON(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_Timeout(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{
		BaseURL:    upstream.URL,
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeTimeout, AsAudnexusError(err).Code)
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./pkg/audnexus/ -run TestService_GetChapters_ -v -count=1`
Expected: PASS for all four (the error mapping was already implemented in Task 3; these tests verify that mapping).

- [ ] **Step 3: Commit**

```bash
git add pkg/audnexus/service_test.go
git commit -m "[Test] Cover audnexus error mapping for 404/5xx/timeout/invalid JSON"
```

---

## Task 5: Service in-memory TTL cache

**Files:**
- Modify: `pkg/audnexus/service.go`
- Modify: `pkg/audnexus/service_test.go`

- [ ] **Step 1: Add failing test for cache hit and TTL expiry**

Append to `pkg/audnexus/service_test.go`:

```go
import (
	"sync/atomic"
)

func TestService_GetChapters_CacheHit(t *testing.T) {
	t.Parallel()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"asin":"B0036UC2LO","chapters":[]}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: time.Hour})

	// First call hits upstream.
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	// Second call with same ASIN hits cache.
	_, err = svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	// Lowercase should normalize to same cache key.
	_, err = svc.GetChapters(context.Background(), "b0036uc2lo")
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "expected exactly one upstream call")
}

func TestService_GetChapters_CacheExpires(t *testing.T) {
	t.Parallel()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"asin":"B0036UC2LO","chapters":[]}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: 10 * time.Millisecond})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	time.Sleep(20 * time.Millisecond)
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "expected cache to expire and re-fetch")
}

func TestService_GetChapters_ErrorsNotCached(t *testing.T) {
	t.Parallel()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: time.Hour})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "errors must not be cached")
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./pkg/audnexus/ -run "TestService_GetChapters_Cache|TestService_GetChapters_ErrorsNotCached" -v`
Expected: FAIL — cache is not yet implemented, so `hits` will be 2 or 3 instead of 1.

- [ ] **Step 3: Wire the cache into `GetChapters`**

In `pkg/audnexus/service.go`, update `GetChapters` to check the cache before upstream and to store successful responses:

```go
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
```

- [ ] **Step 4: Run the full package tests**

Run: `go test ./pkg/audnexus/ -count=1`
Expected: PASS for all tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/audnexus/service.go pkg/audnexus/service_test.go
git commit -m "[Backend] Add 24h in-memory cache to audnexus service"
```

---

## Task 6: HTTP handler with ASIN route

**Files:**
- Create: `pkg/audnexus/handlers.go`
- Create: `pkg/audnexus/handlers_test.go`
- Create: `pkg/audnexus/routes.go`

- [ ] **Step 1: Create `pkg/audnexus/handlers.go`**

```go
package audnexus

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type handler struct {
	service *Service
}

func (h *handler) getChapters(c echo.Context) error {
	asin := c.Param("asin")
	resp, err := h.service.GetChapters(c.Request().Context(), asin)
	if err != nil {
		return mapServiceError(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// mapServiceError converts an audnexus *Error into an errcodes response with
// the right HTTP status. Non-typed errors bubble up as 502 (upstream_error).
func mapServiceError(err error) error {
	e := AsAudnexusError(err)
	if e == nil {
		return errcodes.BadGateway("upstream_error")
	}
	switch e.Code {
	case ErrCodeInvalidASIN:
		return errcodes.BadRequest(string(e.Code))
	case ErrCodeNotFound:
		return errcodes.NotFound(string(e.Code))
	case ErrCodeTimeout:
		return errcodes.GatewayTimeout(string(e.Code))
	default:
		return errcodes.BadGateway(string(e.Code))
	}
}
```

If `errcodes.BadGateway` or `errcodes.GatewayTimeout` don't exist in `pkg/errcodes/`, add them. Check with:

Run: `grep -n "^func " pkg/errcodes/*.go | head -20`

If they're missing, add to `pkg/errcodes/errcodes.go` (mirror the shape of existing helpers):

```go
func BadGateway(message string) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadGateway, message)
}

func GatewayTimeout(message string) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusGatewayTimeout, message)
}
```

- [ ] **Step 2: Create `pkg/audnexus/routes.go`**

```go
package audnexus

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes wires the audnexus endpoint into the Echo instance. The
// endpoint is scoped to authenticated users with books:write (the only
// legitimate use is staging data into an editable chapter form).
func RegisterRoutes(e *echo.Echo, svc *Service, authMiddleware *auth.Middleware) {
	g := e.Group("/audnexus")
	g.Use(authMiddleware.Authenticate)
	g.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))

	h := &handler{service: svc}
	g.GET("/books/:asin/chapters", h.getChapters)
}
```

- [ ] **Step 3: Write handler tests**

Create `pkg/audnexus/handlers_test.go`:

```go
package audnexus

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler(t *testing.T, upstreamStatus int, upstreamBody string) *handler {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(upstreamStatus)
		_, _ = io.WriteString(w, upstreamBody)
	}))
	t.Cleanup(upstream.Close)

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	return &handler{service: svc}
}

func invokeHandler(t *testing.T, h *handler, asin string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/audnexus/books/"+asin+"/chapters", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("asin")
	c.SetParamValues(asin)

	err := h.getChapters(c)
	if err != nil {
		// Echo HTTPError handling — set status from the error.
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, strings.TrimSpace(he.Message.(string))
		}
		return http.StatusInternalServerError, err.Error()
	}
	return rec.Code, rec.Body.String()
}

func TestHandler_GetChapters_Success(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusOK, `{"asin":"B0036UC2LO","chapters":[{"title":"C1","startOffsetMs":0,"lengthMs":1000}]}`)
	code, body := invokeHandler(t, h, "B0036UC2LO")

	assert.Equal(t, http.StatusOK, code)
	var resp Response
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, "B0036UC2LO", resp.ASIN)
	require.Len(t, resp.Chapters, 1)
	assert.Equal(t, "C1", resp.Chapters[0].Title)
}

func TestHandler_GetChapters_InvalidASIN(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusOK, `{}`)
	code, body := invokeHandler(t, h, "short")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, "invalid_asin")
}

func TestHandler_GetChapters_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusNotFound, ``)
	code, body := invokeHandler(t, h, "B0036UC2LO")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Contains(t, body, "not_found")
}

func TestHandler_GetChapters_UpstreamError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusInternalServerError, ``)
	code, body := invokeHandler(t, h, "B0036UC2LO")
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, body, "upstream_error")
}
```

- [ ] **Step 4: Run handler tests**

Run: `go test ./pkg/audnexus/ -run TestHandler -v -count=1`
Expected: PASS.

Run the whole package:

Run: `go test ./pkg/audnexus/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/audnexus/handlers.go pkg/audnexus/routes.go pkg/audnexus/handlers_test.go pkg/errcodes/
git commit -m "[Backend] Add audnexus HTTP handler with ASIN route"
```

---

## Task 7: Wire audnexus service into server.go

**Files:**
- Modify: `pkg/server/server.go`

- [ ] **Step 1: Inspect where similar services are constructed in `pkg/server/server.go`**

Run: `grep -n "authMiddleware\|RegisterRoutes" pkg/server/server.go | head -30`

Pick a spot near the other top-level `RegisterRoutes` calls (around line 115–120), where the server has access to `e`, `authMiddleware`, and version info.

- [ ] **Step 2: Add the audnexus construction and route registration**

Add the import at the top of `pkg/server/server.go`:

```go
"github.com/shishobooks/shisho/pkg/audnexus"
"github.com/shishobooks/shisho/pkg/version"
```

(If `version` is already imported, skip.)

In the function that wires routes, immediately after the `logs.RegisterRoutes(e, logBuffer, authMiddleware)` line (near line 117), add:

```go
audnexusService := audnexus.NewService(audnexus.ServiceConfig{
	UserAgent: "Shisho/" + version.Version,
})
audnexus.RegisterRoutes(e, audnexusService, authMiddleware)
```

- [ ] **Step 3: Build the binary to confirm wiring compiles**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Run all backend tests for regressions**

Run: `go test ./pkg/audnexus/ ./pkg/server/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/server.go
git commit -m "[Backend] Register audnexus routes on the Echo server"
```

---

## Task 8: pkg/audnexus/CLAUDE.md documentation

**Files:**
- Create: `pkg/audnexus/CLAUDE.md`

- [ ] **Step 1: Create `pkg/audnexus/CLAUDE.md`**

```markdown
# Audnexus API Integration

This package fetches chapter data for audiobooks from [Audnexus](https://audnex.us), a community-run proxy over Audible's chapter catalog. It's consumed by the M4B chapter edit UI via `GET /audnexus/books/:asin/chapters`.

## Architecture

- `service.go` — `Service` with a 5s-timeout HTTP client and a 24h in-memory TTL cache keyed by normalized (uppercase) ASIN. Only successes are cached; errors pass through so retries work.
- `handlers.go` — Echo handler that calls the service and maps typed errors to HTTP statuses.
- `routes.go` — `RegisterRoutes` wires the endpoint with `Authenticate` + `books:write` middleware.
- `types.go` — Response types with snake_case JSON tags. The upstream Audnexus camelCase shape is decoded into `audnexusUpstream` and converted at the parse boundary.
- `errors.go` — `ErrorCode` string identifiers and an `*Error` type. Use `AsAudnexusError(err)` to inspect.

## Endpoint

| Method | Path | Permission |
|--------|------|------------|
| GET | `/audnexus/books/:asin/chapters` | `books:write` |

### Error codes

| Service code | HTTP status |
|--------------|-------------|
| `invalid_asin` | 400 |
| `not_found` | 404 |
| `timeout` | 504 |
| `upstream_error` | 502 |

ASINs are validated before upstream calls: 10 alphanumeric characters, normalized to uppercase.

## Permissions

Uses `books:write` rather than `books:read` because the only legitimate use is staging data into an editable chapter form. Read-only users can't save what they fetch, so exposing the endpoint to them is pointless and widens the surface area. If the UI hides the button, the endpoint must also reject the request.

## Caching

In-memory `map[string]*cacheEntry` protected by `sync.RWMutex`, TTL 24h. No persistence across restarts. The cache is small (one entry per ASIN), and Audnexus data rarely changes, so there's no eviction beyond TTL.
```

- [ ] **Step 2: Commit**

```bash
git add pkg/audnexus/CLAUDE.md
git commit -m "[Docs] Add pkg/audnexus/CLAUDE.md"
```

---

## Task 9: Frontend API client + query hook

**Files:**
- Create: `app/hooks/queries/audnexus.ts`

- [ ] **Step 1: Create `app/hooks/queries/audnexus.ts`**

```typescript
import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  AudnexusChapters = "AudnexusChapters",
}

export interface AudnexusChapter {
  title: string;
  start_offset_ms: number;
  length_ms: number;
}

export interface AudnexusChaptersResponse {
  asin: string;
  is_accurate: boolean;
  runtime_length_ms: number;
  brand_intro_duration_ms: number;
  brand_outro_duration_ms: number;
  chapters: AudnexusChapter[];
}

/**
 * Fetch chapter data for an Audible ASIN. Disabled by default — enable only
 * when the user explicitly triggers a lookup from the fetch dialog.
 */
export const useAudnexusChapters = (
  asin: string | null,
  options: Omit<
    UseQueryOptions<AudnexusChaptersResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<AudnexusChaptersResponse, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : false,
    retry: false,
    ...options,
    queryKey: [QueryKey.AudnexusChapters, asin],
    queryFn: async ({ signal }) => {
      if (!asin) {
        throw new Error("ASIN is required");
      }
      return API.request<AudnexusChaptersResponse>(
        "GET",
        `/audnexus/books/${encodeURIComponent(asin)}/chapters`,
        null,
        null,
        signal,
      );
    },
  });
};
```

- [ ] **Step 2: Verify it type-checks**

Run: `pnpm lint:types`
Expected: PASS (no new errors in this file).

- [ ] **Step 3: Commit**

```bash
git add app/hooks/queries/audnexus.ts
git commit -m "[Frontend] Add useAudnexusChapters query hook"
```

---

## Task 10: Frontend utility — trim offset detection

**Files:**
- Create: `app/components/files/audnexusChapterUtils.ts`
- Create: `app/components/files/audnexusChapterUtils.test.ts`

- [ ] **Step 1: Write failing tests for trim detection**

Create `app/components/files/audnexusChapterUtils.test.ts`:

```typescript
import { describe, expect, it } from "vitest";

import { detectIntroOffset } from "./audnexusChapterUtils";

describe("detectIntroOffset", () => {
  it("defaults to false when file matches full runtime", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_938_000,
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("returns true when file duration matches runtime minus intro", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_900_000, // runtime - intro
    });
    expect(result).toEqual({ applyOffset: true, withinTolerance: true });
  });

  it("returns true when file duration matches runtime minus intro minus outro (Libation)", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_838_000, // runtime - intro - outro
    });
    expect(result).toEqual({ applyOffset: true, withinTolerance: true });
  });

  it("returns false when file duration matches runtime minus outro only (no intro offset needed)", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_876_000, // runtime - outro
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("accepts ±2000ms tolerance on matches", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_939_500, // +1.5s off full runtime
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("falls back to false and withinTolerance=false when nothing matches", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 160_000_000, // way off
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: false });
  });
});
```

- [ ] **Step 2: Run and verify they fail**

Run: `pnpm vitest run app/components/files/audnexusChapterUtils.test.ts`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Create `app/components/files/audnexusChapterUtils.ts` with `detectIntroOffset`**

```typescript
import type { AudnexusChaptersResponse } from "@/hooks/queries/audnexus";
import type { ChapterInput } from "@/types";

export const TRIM_DETECT_TOLERANCE_MS = 2000;

interface DetectIntroOffsetParams {
  runtimeMs: number;
  introMs: number;
  outroMs: number;
  fileDurationMs: number;
}

interface DetectIntroOffsetResult {
  applyOffset: boolean;
  withinTolerance: boolean;
}

/**
 * Picks whether to apply an intro offset based on which Audnexus duration
 * candidate is closest to the file's actual duration. Candidates are:
 *   1. runtime (intact)
 *   2. runtime - intro
 *   3. runtime - outro
 *   4. runtime - intro - outro
 *
 * If the closest candidate subtracts intro, applyOffset=true. Otherwise false.
 * If no candidate is within TRIM_DETECT_TOLERANCE_MS, withinTolerance=false
 * (UI shows a mismatch warning) and applyOffset defaults to the closest match.
 */
export const detectIntroOffset = (
  params: DetectIntroOffsetParams,
): DetectIntroOffsetResult => {
  const { runtimeMs, introMs, outroMs, fileDurationMs } = params;
  const candidates: { ms: number; subtractsIntro: boolean }[] = [
    { ms: runtimeMs, subtractsIntro: false },
    { ms: runtimeMs - introMs, subtractsIntro: true },
    { ms: runtimeMs - outroMs, subtractsIntro: false },
    { ms: runtimeMs - introMs - outroMs, subtractsIntro: true },
  ];

  let best = candidates[0];
  let bestDiff = Math.abs(candidates[0].ms - fileDurationMs);
  for (let i = 1; i < candidates.length; i++) {
    const diff = Math.abs(candidates[i].ms - fileDurationMs);
    if (diff < bestDiff) {
      best = candidates[i];
      bestDiff = diff;
    }
  }

  return {
    applyOffset: best.subtractsIntro,
    withinTolerance: bestDiff <= TRIM_DETECT_TOLERANCE_MS,
  };
};
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `pnpm vitest run app/components/files/audnexusChapterUtils.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/files/audnexusChapterUtils.ts app/components/files/audnexusChapterUtils.test.ts
git commit -m "[Frontend] Add trim offset detection utility"
```

---

## Task 11: Frontend utility — apply mode transforms

**Files:**
- Modify: `app/components/files/audnexusChapterUtils.ts`
- Modify: `app/components/files/audnexusChapterUtils.test.ts`

- [ ] **Step 1: Write failing tests for the two apply functions**

Append to `app/components/files/audnexusChapterUtils.test.ts`:

```typescript
import {
  applyTitlesOnly,
  applyTitlesAndTimestamps,
} from "./audnexusChapterUtils";

describe("applyTitlesOnly", () => {
  it("replaces titles by position, keeps timestamps", () => {
    const existing = [
      { title: "Old 1", start_timestamp_ms: 0, children: [] },
      { title: "Old 2", start_timestamp_ms: 60_000, children: [] },
    ];
    const fromAudible = [
      { title: "New 1", start_offset_ms: 100, length_ms: 1 },
      { title: "New 2", start_offset_ms: 200, length_ms: 1 },
    ];
    const result = applyTitlesOnly(existing, fromAudible);
    expect(result).toEqual([
      { title: "New 1", start_timestamp_ms: 0, children: [] },
      { title: "New 2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("returns existing unchanged when counts differ", () => {
    const existing = [{ title: "A", start_timestamp_ms: 0, children: [] }];
    const fromAudible = [
      { title: "X", start_offset_ms: 0, length_ms: 1 },
      { title: "Y", start_offset_ms: 100, length_ms: 1 },
    ];
    const result = applyTitlesOnly(existing, fromAudible);
    expect(result).toEqual(existing);
  });
});

describe("applyTitlesAndTimestamps", () => {
  it("replaces wholesale with no offset when applyIntroOffset=false", () => {
    const fromAudible = [
      { title: "C1", start_offset_ms: 0, length_ms: 1000 },
      { title: "C2", start_offset_ms: 60_000, length_ms: 1000 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: false,
      introMs: 38_000,
    });
    expect(result).toEqual([
      { title: "C1", start_timestamp_ms: 0, children: [] },
      { title: "C2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("subtracts introMs from every start when applyIntroOffset=true", () => {
    const fromAudible = [
      { title: "C1", start_offset_ms: 38_000, length_ms: 1000 },
      { title: "C2", start_offset_ms: 98_000, length_ms: 1000 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: true,
      introMs: 38_000,
    });
    expect(result).toEqual([
      { title: "C1", start_timestamp_ms: 0, children: [] },
      { title: "C2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("clamps negative timestamps to 0 after offset", () => {
    const fromAudible = [
      { title: "Pre", start_offset_ms: 0, length_ms: 1 },
      { title: "Main", start_offset_ms: 40_000, length_ms: 1 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: true,
      introMs: 38_000,
    });
    expect(result[0].start_timestamp_ms).toBe(0);
    expect(result[1].start_timestamp_ms).toBe(2_000);
  });
});
```

- [ ] **Step 2: Run and verify they fail**

Run: `pnpm vitest run app/components/files/audnexusChapterUtils.test.ts`
Expected: FAIL — functions don't exist.

- [ ] **Step 3: Add the two functions to `app/components/files/audnexusChapterUtils.ts`**

Append:

```typescript
import type { AudnexusChapter } from "@/hooks/queries/audnexus";

/**
 * Replace titles in-place by position; leave timestamps and children alone.
 * Returns the original array unchanged when lengths differ so the caller
 * doesn't silently lose data.
 */
export const applyTitlesOnly = (
  existing: ChapterInput[],
  fromAudible: AudnexusChapter[],
): ChapterInput[] => {
  if (existing.length !== fromAudible.length) {
    return existing;
  }
  return existing.map((chapter, i) => ({
    ...chapter,
    title: fromAudible[i].title,
  }));
};

interface ApplyTitlesAndTimestampsOpts {
  applyIntroOffset: boolean;
  introMs: number;
}

/**
 * Build a fresh chapter array from Audnexus data, optionally subtracting the
 * intro duration from every start timestamp. Negative results are clamped
 * to 0 (only possible for the first chapter).
 */
export const applyTitlesAndTimestamps = (
  fromAudible: AudnexusChapter[],
  opts: ApplyTitlesAndTimestampsOpts,
): ChapterInput[] => {
  const offset = opts.applyIntroOffset ? opts.introMs : 0;
  return fromAudible.map((c) => ({
    title: c.title,
    start_timestamp_ms: Math.max(0, c.start_offset_ms - offset),
    children: [],
  }));
};
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `pnpm vitest run app/components/files/audnexusChapterUtils.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/files/audnexusChapterUtils.ts app/components/files/audnexusChapterUtils.test.ts
git commit -m "[Frontend] Add apply-mode transforms for audnexus chapters"
```

---

## Task 12: FetchChaptersDialog — ASIN entry + loading + error states

**Files:**
- Create: `app/components/files/FetchChaptersDialog.tsx`

- [ ] **Step 1: Create the dialog with the initial + loading + error states**

Create `app/components/files/FetchChaptersDialog.tsx`:

```tsx
import { useEffect, useMemo, useState } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useAudnexusChapters,
  type AudnexusChaptersResponse,
} from "@/hooks/queries/audnexus";
import { cn } from "@/libraries/utils";
import type { ChapterInput } from "@/types";

export interface FetchChaptersDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** ASIN prefilled from the file's existing identifiers, if present. */
  initialAsin?: string;
  /** Called when the user clicks an Apply button. Closes the dialog. */
  onApply: (chapters: ChapterInput[]) => void;
  /** Current edited chapters; used for the counts-match check. */
  editedChapters: ChapterInput[];
  /** File duration in ms; used for trim offset detection. */
  fileDurationMs: number;
  /** True if the parent edit form has unsaved edits. */
  hasChanges: boolean;
}

const ASIN_PATTERN = /^[A-Z0-9]{10}$/;

type Stage = "entry" | "result";

export const FetchChaptersDialog = ({
  open,
  onOpenChange,
  initialAsin,
  onApply,
  editedChapters,
  fileDurationMs,
  hasChanges,
}: FetchChaptersDialogProps) => {
  const [stage, setStage] = useState<Stage>("entry");
  const [asinInput, setAsinInput] = useState("");
  const [submittedAsin, setSubmittedAsin] = useState<string | null>(null);

  // Reset internal state when the dialog opens so reopening starts fresh.
  useEffect(() => {
    if (open) {
      setAsinInput((initialAsin ?? "").toUpperCase());
      setSubmittedAsin(null);
      setStage("entry");
    }
  }, [open, initialAsin]);

  const normalizedAsin = useMemo(
    () => asinInput.trim().toUpperCase(),
    [asinInput],
  );
  const isValidAsin = ASIN_PATTERN.test(normalizedAsin);
  const prefilledActive =
    initialAsin !== undefined &&
    normalizedAsin === initialAsin.trim().toUpperCase();

  const query = useAudnexusChapters(submittedAsin, {
    enabled: Boolean(submittedAsin),
  });

  const handleFetch = () => {
    if (!isValidAsin) return;
    setSubmittedAsin(normalizedAsin);
    setStage("result");
  };

  const handleRetry = () => {
    void query.refetch();
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg overflow-x-hidden">
        {stage === "entry" && (
          <EntryStage
            asinInput={asinInput}
            setAsinInput={setAsinInput}
            isValidAsin={isValidAsin}
            prefilledActive={prefilledActive}
            onCancel={() => onOpenChange(false)}
            onFetch={handleFetch}
          />
        )}
        {stage === "result" && query.isLoading && <LoadingStage />}
        {stage === "result" && query.isError && (
          <ErrorStage
            error={query.error}
            asinInput={asinInput}
            setAsinInput={setAsinInput}
            onRetry={handleRetry}
            onCancel={() => onOpenChange(false)}
          />
        )}
        {/* Result stage body is added in the next task */}
        {stage === "result" &&
          query.isSuccess &&
          query.data && (
            <ResultStagePlaceholder
              data={query.data}
              editedChapters={editedChapters}
              fileDurationMs={fileDurationMs}
              hasChanges={hasChanges}
              onApply={(chapters) => {
                onApply(chapters);
                onOpenChange(false);
              }}
              onCancel={() => onOpenChange(false)}
            />
          )}
      </DialogContent>
    </Dialog>
  );
};

// --- Sub-stages ---

interface EntryStageProps {
  asinInput: string;
  setAsinInput: (value: string) => void;
  isValidAsin: boolean;
  prefilledActive: boolean;
  onCancel: () => void;
  onFetch: () => void;
}

const EntryStage = ({
  asinInput,
  setAsinInput,
  isValidAsin,
  prefilledActive,
  onCancel,
  onFetch,
}: EntryStageProps) => (
  <>
    <DialogHeader className="pr-8">
      <DialogTitle>Fetch chapters from Audible</DialogTitle>
      <DialogDescription>
        Look up chapter titles and timestamps by Audible ID.
      </DialogDescription>
    </DialogHeader>

    <div className="space-y-4">
      <div>
        <Label className="mb-1.5 block" htmlFor="audnexus-asin">
          Audible ID (ASIN)
        </Label>
        <Input
          autoComplete="off"
          className="font-mono"
          id="audnexus-asin"
          onChange={(e) => setAsinInput(e.target.value)}
          placeholder="B0036UC2LO"
          value={asinInput}
        />
        <p className="mt-1.5 text-xs text-muted-foreground">
          {prefilledActive
            ? "Using this file's existing Audible ID."
            : "10 characters, found on the Audible book page URL."}
        </p>
      </div>

      <div className="flex items-center justify-between pt-2">
        <span className="text-xs text-muted-foreground">
          Powered by{" "}
          <a
            className="underline hover:no-underline"
            href="https://audnex.us"
            rel="noreferrer"
            target="_blank"
          >
            Audnexus
          </a>
        </span>
        <div className="flex items-center gap-2">
          <Button onClick={onCancel} variant="ghost">
            Cancel
          </Button>
          <Button disabled={!isValidAsin} onClick={onFetch}>
            Fetch
          </Button>
        </div>
      </div>
    </div>
  </>
);

const LoadingStage = () => (
  <>
    <DialogHeader className="pr-8">
      <DialogTitle>Fetch chapters from Audible</DialogTitle>
    </DialogHeader>
    <div className="flex flex-col items-center gap-3 py-8">
      <div
        aria-label="Loading"
        className="h-5 w-5 animate-spin rounded-full border-2 border-border border-t-primary"
      />
      <p className="text-sm text-muted-foreground">
        Looking up chapters on Audible…
      </p>
    </div>
  </>
);

interface ErrorStageProps {
  error: unknown;
  asinInput: string;
  setAsinInput: (value: string) => void;
  onRetry: () => void;
  onCancel: () => void;
}

const ErrorStage = ({
  error,
  asinInput,
  setAsinInput,
  onRetry,
  onCancel,
}: ErrorStageProps) => {
  const message = errorMessage(error);
  return (
    <>
      <DialogHeader className="pr-8">
        <DialogTitle>Fetch chapters from Audible</DialogTitle>
      </DialogHeader>
      <div className="space-y-4">
        <div
          className={cn(
            "rounded-md border p-3 text-sm",
            "border-destructive/40 bg-destructive/10 text-destructive",
          )}
          role="alert"
        >
          {message}
        </div>
        <div>
          <Label className="mb-1.5 block" htmlFor="audnexus-asin-retry">
            Audible ID (ASIN)
          </Label>
          <Input
            autoComplete="off"
            className="font-mono"
            id="audnexus-asin-retry"
            onChange={(e) => setAsinInput(e.target.value)}
            value={asinInput}
          />
        </div>
        <div className="flex items-center justify-end gap-2 pt-2">
          <Button onClick={onCancel} variant="ghost">
            Cancel
          </Button>
          <Button onClick={onRetry}>Retry</Button>
        </div>
      </div>
    </>
  );
};

const errorMessage = (error: unknown): string => {
  // ShishoAPIError shape has a `code` field for our error envelope.
  const code = (error as { code?: string } | null)?.code;
  switch (code) {
    case "not_found":
      return "We couldn't find this ASIN on Audible. Double-check the ID on the Audible book page.";
    case "timeout":
      return "Request timed out. Try again.";
    case "invalid_asin":
      return "Check the ASIN format. It should be 10 alphanumeric characters.";
    default:
      return "Couldn't reach Audible. Try again in a moment.";
  }
};

// Placeholder until the next task wires up the real result body.
interface ResultStagePlaceholderProps {
  data: AudnexusChaptersResponse;
  editedChapters: ChapterInput[];
  fileDurationMs: number;
  hasChanges: boolean;
  onApply: (chapters: ChapterInput[]) => void;
  onCancel: () => void;
}

const ResultStagePlaceholder = (props: ResultStagePlaceholderProps) => {
  // Suppress unused-var lint warnings; the next task replaces this.
  void props;
  return (
    <div className="p-4 text-sm text-muted-foreground">
      Result view is implemented in the next task.
    </div>
  );
};
```

- [ ] **Step 2: Type-check**

Run: `pnpm lint:types`
Expected: PASS (no new errors).

- [ ] **Step 3: Commit**

```bash
git add app/components/files/FetchChaptersDialog.tsx
git commit -m "[Frontend] Scaffold FetchChaptersDialog with entry/loading/error stages"
```

---

## Task 13: FetchChaptersDialog — result stage

**Files:**
- Modify: `app/components/files/FetchChaptersDialog.tsx`

- [ ] **Step 1: Replace `ResultStagePlaceholder` with the real result view**

In `app/components/files/FetchChaptersDialog.tsx`, delete the placeholder and replace with:

```tsx
import { Checkbox } from "@/components/ui/checkbox";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

import {
  applyTitlesAndTimestamps,
  applyTitlesOnly,
  detectIntroOffset,
} from "./audnexusChapterUtils";

interface ResultStageProps {
  data: AudnexusChaptersResponse;
  editedChapters: ChapterInput[];
  fileDurationMs: number;
  hasChanges: boolean;
  onApply: (chapters: ChapterInput[]) => void;
  onCancel: () => void;
}

const ResultStage = ({
  data,
  editedChapters,
  fileDurationMs,
  hasChanges,
  onApply,
  onCancel,
}: ResultStageProps) => {
  const detection = useMemo(
    () =>
      detectIntroOffset({
        runtimeMs: data.runtime_length_ms,
        introMs: data.brand_intro_duration_ms,
        outroMs: data.brand_outro_duration_ms,
        fileDurationMs,
      }),
    [data, fileDurationMs],
  );

  const [applyOffset, setApplyOffset] = useState(detection.applyOffset);

  // Re-sync when a new fetch result lands (different ASIN, say).
  useEffect(() => {
    setApplyOffset(detection.applyOffset);
  }, [detection.applyOffset]);

  const countsMatch = data.chapters.length === editedChapters.length;
  const durationDiffMs = Math.abs(data.runtime_length_ms - fileDurationMs);

  const handleApplyTitlesOnly = () => {
    onApply(applyTitlesOnly(editedChapters, data.chapters));
  };

  const handleApplyBoth = () => {
    onApply(
      applyTitlesAndTimestamps(data.chapters, {
        applyIntroOffset: applyOffset,
        introMs: data.brand_intro_duration_ms,
      }),
    );
  };

  return (
    <>
      <DialogHeader className="pr-8">
        <DialogTitle>Chapters from Audible</DialogTitle>
        <DialogDescription>Audible ID: {data.asin}</DialogDescription>
      </DialogHeader>

      <div className="space-y-4">
        <DurationComparison
          audibleMs={data.runtime_length_ms}
          fileMs={fileDurationMs}
          introMs={data.brand_intro_duration_ms}
          outroMs={data.brand_outro_duration_ms}
          audibleCount={data.chapters.length}
          fileCount={editedChapters.length}
        />

        {detection.withinTolerance ? (
          <InfoCallout
            heading={
              detection.applyOffset
                ? `Intro removed. Chapters offset by −${Math.round(
                    data.brand_intro_duration_ms / 1000,
                  )}s.`
                : "Durations match. File is intact."
            }
            body={
              detection.applyOffset
                ? "Matches a trimmed file (e.g. Libation rip). Chapters will be shifted to align."
                : "Chapter timestamps will be used as-is."
            }
          />
        ) : (
          <WarnCallout
            heading={`Duration differs by ${formatDurationDiff(durationDiffMs)}. May be a different edition.`}
            body="None of the trim modes match within 2 seconds. Timestamps may be off."
          />
        )}

        {hasChanges && (
          <WarnCallout
            heading="Unsaved changes will be overwritten"
            body="You have unsaved edits to the current chapters. Applying will replace them. You can still Cancel afterward to discard everything."
          />
        )}

        <label className="flex cursor-pointer items-start gap-2 text-sm">
          <Checkbox
            checked={applyOffset}
            className="mt-0.5"
            onCheckedChange={(v) => setApplyOffset(v === true)}
          />
          <div>
            <div>
              Offset chapters by intro duration (−
              {Math.round(data.brand_intro_duration_ms / 1000)}s)
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">
              Enable for files with the Audible intro stripped out (e.g.
              Libation rips).
            </div>
          </div>
        </label>

        <div className="flex flex-wrap items-center justify-end gap-2 pt-2">
          <Button onClick={onCancel} variant="ghost">
            Cancel
          </Button>
          <Button
            onClick={handleApplyBoth}
            variant={countsMatch ? "outline" : "default"}
          >
            Apply titles + timestamps
          </Button>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    disabled={!countsMatch}
                    onClick={handleApplyTitlesOnly}
                  >
                    Apply titles only
                  </Button>
                </span>
              </TooltipTrigger>
              {!countsMatch && (
                <TooltipContent>
                  Chapter counts don't match ({editedChapters.length} vs{" "}
                  {data.chapters.length}).
                </TooltipContent>
              )}
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
    </>
  );
};

interface DurationComparisonProps {
  audibleMs: number;
  fileMs: number;
  introMs: number;
  outroMs: number;
  audibleCount: number;
  fileCount: number;
}

const DurationComparison = ({
  audibleMs,
  fileMs,
  introMs,
  outroMs,
  audibleCount,
  fileCount,
}: DurationComparisonProps) => (
  <div className="rounded-md border border-border">
    <KvRow label="Audible runtime">
      <span>{formatHms(audibleMs)}</span>
      {(introMs > 0 || outroMs > 0) && (
        <span className="text-muted-foreground">
          {" · "}
          {introMs > 0 && `intro ${formatSeconds(introMs)}`}
          {introMs > 0 && outroMs > 0 && " · "}
          {outroMs > 0 && `outro ${formatSeconds(outroMs)}`}
        </span>
      )}
    </KvRow>
    <KvRow label="Your file">{formatHms(fileMs)}</KvRow>
    <KvRow label="Chapters">
      <span>
        {audibleCount} from Audible <span className="text-muted-foreground">·</span>{" "}
        <span
          className={cn(audibleCount !== fileCount && "text-amber-500")}
        >
          {fileCount} in your file
        </span>
      </span>
    </KvRow>
  </div>
);

const KvRow = ({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) => (
  <div className="flex items-baseline justify-between gap-2 px-3 py-2 [&+&]:border-t [&+&]:border-border">
    <span className="text-sm text-muted-foreground">{label}</span>
    <span className="text-sm font-medium">{children}</span>
  </div>
);

const InfoCallout = ({ heading, body }: { heading: string; body: string }) => (
  <div className="rounded-md border border-primary/40 bg-primary/10 p-3 text-sm">
    <div className="font-medium text-primary">{heading}</div>
    <div className="mt-0.5 text-xs text-muted-foreground">{body}</div>
  </div>
);

const WarnCallout = ({ heading, body }: { heading: string; body: string }) => (
  <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm">
    <div className="font-medium text-destructive">{heading}</div>
    <div className="mt-0.5 text-xs text-muted-foreground">{body}</div>
  </div>
);

const formatHms = (ms: number): string => {
  const total = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
};

const formatSeconds = (ms: number): string => {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m ${s % 60}s`;
};

const formatDurationDiff = (ms: number): string => formatHms(ms);
```

And replace the `<ResultStagePlaceholder ... />` usage in the main component body with:

```tsx
<ResultStage
  data={query.data}
  editedChapters={editedChapters}
  fileDurationMs={fileDurationMs}
  hasChanges={hasChanges}
  onApply={(chapters) => {
    onApply(chapters);
    onOpenChange(false);
  }}
  onCancel={() => onOpenChange(false)}
/>
```

Remove the `ResultStagePlaceholder` function entirely.

- [ ] **Step 2: Type-check and lint**

Run: `pnpm lint:types && pnpm lint:eslint`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/components/files/FetchChaptersDialog.tsx
git commit -m "[Frontend] Implement result stage in FetchChaptersDialog"
```

---

## Task 14: FetchChaptersDialog component tests

**Files:**
- Create: `app/components/files/FetchChaptersDialog.test.tsx`

- [ ] **Step 1: Create the test file**

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { FetchChaptersDialog } from "./FetchChaptersDialog";

const renderWithClient = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

describe("FetchChaptersDialog", () => {
  it("prefills ASIN and enables Fetch when valid", () => {
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[]}
        fileDurationMs={0}
        hasChanges={false}
        initialAsin="B0036UC2LO"
        onApply={vi.fn()}
        onOpenChange={vi.fn()}
        open
      />,
    );
    expect(screen.getByText("Using this file's existing Audible ID.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Fetch" })).not.toBeDisabled();
  });

  it("disables Fetch when ASIN is invalid", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[]}
        fileDurationMs={0}
        hasChanges={false}
        onApply={vi.fn()}
        onOpenChange={vi.fn()}
        open
      />,
    );
    expect(screen.getByRole("button", { name: "Fetch" })).toBeDisabled();
    await user.type(screen.getByLabelText("Audible ID (ASIN)"), "short");
    expect(screen.getByRole("button", { name: "Fetch" })).toBeDisabled();
    await user.clear(screen.getByLabelText("Audible ID (ASIN)"));
    await user.type(screen.getByLabelText("Audible ID (ASIN)"), "B0036UC2LO");
    expect(screen.getByRole("button", { name: "Fetch" })).not.toBeDisabled();
  });

  it("shows loading, then result, and calls onApply with titles-only chapters when counts match", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const apiSpy = vi.spyOn(API, "request").mockResolvedValue({
      asin: "B0036UC2LO",
      is_accurate: true,
      runtime_length_ms: 60_000,
      brand_intro_duration_ms: 0,
      brand_outro_duration_ms: 0,
      chapters: [
        { title: "New A", start_offset_ms: 0, length_ms: 30_000 },
        { title: "New B", start_offset_ms: 30_000, length_ms: 30_000 },
      ],
    });
    const onApply = vi.fn();
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[
          { title: "Old A", start_timestamp_ms: 0, children: [] },
          { title: "Old B", start_timestamp_ms: 20_000, children: [] },
        ]}
        fileDurationMs={60_000}
        hasChanges={false}
        initialAsin="B0036UC2LO"
        onApply={onApply}
        onOpenChange={vi.fn()}
        open
      />,
    );
    await user.click(screen.getByRole("button", { name: "Fetch" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Apply titles only" }),
      ).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Apply titles only" }));

    expect(onApply).toHaveBeenCalledWith([
      { title: "New A", start_timestamp_ms: 0, children: [] },
      { title: "New B", start_timestamp_ms: 20_000, children: [] },
    ]);
    expect(apiSpy).toHaveBeenCalled();
  });

  it("disables Apply titles only when counts differ", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.spyOn(API, "request").mockResolvedValue({
      asin: "B0036UC2LO",
      is_accurate: true,
      runtime_length_ms: 60_000,
      brand_intro_duration_ms: 0,
      brand_outro_duration_ms: 0,
      chapters: [
        { title: "A", start_offset_ms: 0, length_ms: 30_000 },
        { title: "B", start_offset_ms: 30_000, length_ms: 30_000 },
      ],
    });
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[
          { title: "Only one", start_timestamp_ms: 0, children: [] },
        ]}
        fileDurationMs={60_000}
        hasChanges={false}
        initialAsin="B0036UC2LO"
        onApply={vi.fn()}
        onOpenChange={vi.fn()}
        open
      />,
    );
    await user.click(screen.getByRole("button", { name: "Fetch" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Apply titles only" }),
      ).toBeDisabled(),
    );
  });

  it("shows overwrite warning when hasChanges=true", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.spyOn(API, "request").mockResolvedValue({
      asin: "B0036UC2LO",
      is_accurate: true,
      runtime_length_ms: 60_000,
      brand_intro_duration_ms: 0,
      brand_outro_duration_ms: 0,
      chapters: [{ title: "A", start_offset_ms: 0, length_ms: 60_000 }],
    });
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[{ title: "Old", start_timestamp_ms: 0, children: [] }]}
        fileDurationMs={60_000}
        hasChanges
        initialAsin="B0036UC2LO"
        onApply={vi.fn()}
        onOpenChange={vi.fn()}
        open
      />,
    );
    await user.click(screen.getByRole("button", { name: "Fetch" }));
    await waitFor(() =>
      expect(
        screen.getByText(/Unsaved changes will be overwritten/i),
      ).toBeInTheDocument(),
    );
  });

  it("shows error message for not_found", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.spyOn(API, "request").mockRejectedValue({
      code: "not_found",
      message: "ASIN not found",
    });
    renderWithClient(
      <FetchChaptersDialog
        editedChapters={[]}
        fileDurationMs={0}
        hasChanges={false}
        initialAsin="B0036UC2LO"
        onApply={vi.fn()}
        onOpenChange={vi.fn()}
        open
      />,
    );
    await user.click(screen.getByRole("button", { name: "Fetch" }));
    await waitFor(() =>
      expect(
        screen.getByText(/couldn't find this ASIN/i),
      ).toBeInTheDocument(),
    );
  });
});
```

- [ ] **Step 2: Run component tests**

Run: `pnpm vitest run app/components/files/FetchChaptersDialog.test.tsx`
Expected: PASS for all cases.

- [ ] **Step 3: Commit**

```bash
git add app/components/files/FetchChaptersDialog.test.tsx
git commit -m "[Frontend] Test FetchChaptersDialog states and apply behavior"
```

---

## Task 15: Integrate into FileChaptersTab (button, dialog, apply wiring)

**Files:**
- Modify: `app/components/files/FileChaptersTab.tsx`

- [ ] **Step 1: Read the current file to locate insertion points**

Run: `cat app/components/files/FileChaptersTab.tsx | head -40`

You'll be adding:
- A new `useState` for dialog open.
- A computed `audibleAsin` from `file.identifiers`.
- A permission check via `useCurrentUser()` (there's likely an existing hook; search for it).
- A `handleApplyAudnexus` that replaces `editedChapters` and enters edit mode.
- Three button mounts: edit-mode toolbar, view-mode header, empty-state row.
- The `<FetchChaptersDialog />` rendered at the bottom.

- [ ] **Step 2: Find the existing permission check pattern**

Run: `grep -rn "HasPermission\|hasPermission\|books:write\|OperationWrite" app/components | head -10`

Typical pattern (verify in the repo):
```tsx
const { user } = useCurrentUser();
const canEdit = user?.has_permission?.("books", "write") ?? false;
```

Use whatever pattern the surrounding code already uses for the "Edit" button in this file.

- [ ] **Step 3: Add imports, state, and handlers**

In `app/components/files/FileChaptersTab.tsx`, add the imports:

```tsx
import { FetchChaptersDialog } from "@/components/files/FetchChaptersDialog";
import { IdentifierTypeASIN } from "@/types";
```

Inside the component body (near the other `useState` calls), add:

```tsx
const [isFetchDialogOpen, setIsFetchDialogOpen] = useState(false);

const audibleAsin = useMemo(() => {
  const asinIdentifier = file.identifiers?.find(
    (id) => id.type === IdentifierTypeASIN,
  );
  return asinIdentifier?.value;
}, [file.identifiers]);

// Only M4B files can use this feature.
const canFetchFromAudible =
  file.file_type === FileTypeM4B && userCanEditChapters;

const handleApplyAudnexus = useCallback(
  (chapters: ChapterInput[]) => {
    const newEdited = toEditedChapters(chapters);
    setEditedChapters(newEdited);
    if (!isEditing) {
      // Entering edit mode from view/empty state.
      setInitialChapters(
        toEditedChapters(chaptersToInputArray(chaptersQuery.data ?? [])),
      );
      editInitializedRef.current = true;
      onEditingChange(true);
    }
  },
  [isEditing, chaptersQuery.data, onEditingChange],
);
```

`userCanEditChapters` should be sourced from whatever pattern the current "Edit" button in this component uses (check lines where `onEditingChange(true)` is gated). If the Edit button is always visible in this component and gating happens at the parent, then mirror that: surface `canFetchFromAudible` as the M4B check only, and let the parent's permission gating handle the Edit gating naturally.

- [ ] **Step 4: Add the three button mounts**

Inside `renderEditedChapters` where "Add chapter" for M4B is rendered, add the button next to it:

```tsx
{isM4b && (
  <>
    <Button
      className="mt-2"
      onClick={handleAddChapterM4B}
      type="button"
      variant="outline"
    >
      Add Chapter
    </Button>
    {canFetchFromAudible && (
      <Button
        className="mt-2 ml-2"
        onClick={() => setIsFetchDialogOpen(true)}
        type="button"
        variant="outline"
      >
        Fetch from Audible
      </Button>
    )}
  </>
)}
```

In the empty-state branch where `Add Chapter` for M4B is rendered:

```tsx
{canAddChapters && (
  <div className="mt-4 flex items-center justify-center gap-2">
    <button
      className="cursor-pointer rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
      onClick={handleAddChapterFromEmpty}
      type="button"
    >
      Add Chapter
    </button>
    {file.file_type === FileTypeM4B && canFetchFromAudible && (
      <Button
        onClick={() => setIsFetchDialogOpen(true)}
        type="button"
        variant="outline"
      >
        Fetch from Audible
      </Button>
    )}
  </div>
)}
```

The view-mode header button lives in the parent of this component (`FileDetail` or similar). Trace where the Edit button is rendered for chapters and add "Fetch from Audible" next to it, also gated by `file.file_type === FileTypeM4B` and `books:write`. If that parent doesn't expose a slot for extra actions, skip the third entry point for v1 and note it in Task 17 docs.

- [ ] **Step 5: Render the dialog at the end of the component**

Add just before the closing `</div>` of the component's root return:

```tsx
{canFetchFromAudible && (
  <FetchChaptersDialog
    editedChapters={
      isEditing
        ? stripEditKeys(editedChapters)
        : chaptersToInputArray(chaptersQuery.data ?? [])
    }
    fileDurationMs={(file.audiobook_duration_seconds ?? 0) * 1000}
    hasChanges={hasChanges}
    initialAsin={audibleAsin}
    onApply={handleApplyAudnexus}
    onOpenChange={setIsFetchDialogOpen}
    open={isFetchDialogOpen}
  />
)}
```

- [ ] **Step 6: Type-check, lint, unit tests**

Run: `pnpm lint:types && pnpm lint:eslint && pnpm vitest run app/components/files`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add app/components/files/FileChaptersTab.tsx
git commit -m "[Frontend] Wire FetchChaptersDialog into FileChaptersTab"
```

---

## Task 16: Run the full check suite and fix regressions

**Files:** (none modified by default)

- [ ] **Step 1: Run the full check**

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 2: If anything fails, inspect with full output**

Run: `mise check`

Fix the failing step, re-run `mise check:quiet` until green. Each fix gets its own commit scoped to the failure (e.g., `[Fix] Audnexus test cache race` or `[Frontend] Adjust tooltip role for RTL`).

- [ ] **Step 3: Verify M4B chapter edit UI manually**

Start the dev server:

Run: `mise start` (or `mise start:air` if the frontend is already running)

In the browser:
1. Navigate to an M4B file's chapter edit page.
2. Confirm "Fetch from Audible" appears in view mode, edit mode, and empty-state views.
3. Click it — confirm the ASIN is prefilled when the file has an ASIN identifier.
4. Fetch with a real Audible ASIN. Confirm result stage shows, durations compare, checkbox defaults to the right state.
5. Apply titles only — confirm the edit form shows the new titles, timestamps untouched, not saved.
6. Cancel — confirm the pre-apply chapters come back.
7. Apply titles + timestamps on a Libation rip (where the checkbox is pre-checked). Confirm chapter rows show sensible timestamps and use the play button to spot-check a couple.
8. Hit Save and confirm the chapters persist (reload to verify).
9. Try with a user that lacks `books:write` (Viewer role). Confirm the button isn't visible.

If anything is off, file a task back at the appropriate step and fix before committing.

---

## Task 17: Documentation — website user docs, pkg/mp4/CLAUDE.md

**Files:**
- Check existing website docs structure first: `ls website/docs/`
- Create or modify: `website/docs/chapters.md` (or add a section to the appropriate existing page — `supported-formats.md` or `metadata.md`, verify in the sidebar)
- Modify: `pkg/mp4/CLAUDE.md`

- [ ] **Step 1: Find the right docs location**

Run: `ls website/docs/` and `cat website/sidebars.ts 2>/dev/null || cat website/sidebars.js 2>/dev/null`

Choose one:
- If a "Chapters" page exists, add a "Fetch from Audible" section at the end.
- Otherwise create a new page `website/docs/chapter-editing.md` and add it to the sidebar in the appropriate section.

- [ ] **Step 2: Write the user docs**

The page must cover:

```markdown
## Fetching chapter data from Audible

For M4B audiobook files, Shisho can look up chapter titles and timestamps from Audible's catalog and stage them into the chapter editor. This is useful for files ripped from Audible that have missing or wrong chapter metadata.

### Requirements

- File must be an M4B audiobook.
- You need an Audible ID (ASIN) for the book. You can copy it from the URL of the book's Audible page (e.g., `https://www.audible.com/pd/Example-Audiobook/B0036UC2LO` — the 10-character code at the end).
- Your user must have **books:write** permission (Editor or Admin role).

### How it works

1. Open the chapter edit page for an M4B file and click **Fetch from Audible** (available in view mode, edit mode, and when there are no chapters yet).
2. Paste or confirm the ASIN and click **Fetch**.
3. Shisho shows the Audible runtime, your file's duration, and the chapter count on each side.
4. Shisho auto-detects whether your file has the Audible intro removed (as some ripping tools like Libation do by default). The resulting offset is reflected in a checkbox that you can flip if the detection is wrong.
5. Choose how to apply:
   - **Apply titles only** — replace chapter titles in place, keep your existing timestamps. Available only when the chapter counts match.
   - **Apply titles + timestamps** — replace everything with Audible data, respecting the offset checkbox.
6. The dialog closes and the new data is staged in the edit form. Use the per-chapter play buttons to spot-check timestamps, adjust anything that's off, and click **Save** to commit — or **Cancel** to discard everything, including the fetched data.

### Data source

Chapter data comes from [Audnexus](https://audnex.us), a community-run proxy of Audible chapter data. Shisho caches each ASIN response for 24 hours to minimize upstream calls.
```

Add to the sidebar if a new page was created.

- [ ] **Step 3: Update `pkg/mp4/CLAUDE.md` with a cross-reference**

Append a new section to `pkg/mp4/CLAUDE.md`:

```markdown
## External chapter source

For user-initiated chapter enrichment on M4B files, see `pkg/audnexus/` — a single-endpoint integration that fetches chapter titles and timestamps from Audible via the Audnexus public API. The M4B chapter edit UI exposes a "Fetch from Audible" button that stages the fetched data into the edit form without persisting until the user clicks Save.
```

- [ ] **Step 4: Run the docs build to catch sidebar or markdown issues**

Run: `mise docs` in a separate shell, confirm the new page renders. Stop the server with Ctrl-C when done.

- [ ] **Step 5: Commit**

```bash
git add website/ pkg/mp4/CLAUDE.md
git commit -m "[Docs] Document fetch chapters from Audible feature"
```

---

## Self-review checklist (for the implementer)

Before handing off, verify:

- [ ] `mise check:quiet` passes.
- [ ] New endpoint `/api/audnexus/books/:asin/chapters` returns 400/404/504/502 correctly.
- [ ] Dialog opens from all three entry points on M4B files and is hidden for non-M4B.
- [ ] Dialog is hidden for users without `books:write` (verify with Viewer role).
- [ ] Apply titles only is disabled with a tooltip when counts differ.
- [ ] Checkbox auto-state is correct for: intact file, intro-only trim, outro-only trim, both-removed trim, and mismatch.
- [ ] Cancel discards everything after apply.
- [ ] Out-of-bounds timestamps after offset are flagged by existing `ChapterRow` validation, not silently clamped.
- [ ] Overwrite warning appears only when `hasChanges` is true before fetching.
- [ ] No em dashes in any new user-facing copy.
