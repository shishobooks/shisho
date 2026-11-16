package updatecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGitHub serves a releases list and counts the requests it gets.
type fakeGitHub struct {
	server   *httptest.Server
	requests atomic.Int32

	mu        sync.Mutex
	status    int
	releases  []githubRelease
	userAgent string
}

func newFakeGitHub(t *testing.T, releases ...githubRelease) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{status: http.StatusOK, releases: releases}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.userAgent = r.Header.Get("User-Agent")
		if f.status != http.StatusOK {
			w.WriteHeader(f.status)
			return
		}
		_ = json.NewEncoder(w).Encode(f.releases)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) setStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

func (f *fakeGitHub) setReleases(releases ...githubRelease) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases = releases
}

func release(tag string) githubRelease {
	return githubRelease{TagName: tag}
}

// fakeClock is a settable clock for the cache's freshness checks.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestService(gh *fakeGitHub, current string, clock *fakeClock) *Service {
	return NewService(ServiceConfig{
		Enabled:        true,
		CurrentVersion: current,
		ReleasesURL:    gh.server.URL,
		UserAgent:      "Shisho/test",
		now:            clock.Now,
	})
}

func TestService_Status_CountsNewerPublishedReleases(t *testing.T) {
	t.Parallel()
	draft := release("v0.10.0")
	draft.Draft = true
	prerelease := release("v0.9.0-rc1")
	prerelease.Prerelease = true
	gh := newFakeGitHub(t, draft, prerelease, release("v0.8.0"), release("v0.7.0"), release("v0.6.0"), release("v0.5.0"))
	svc := newTestService(gh, "0.6.0", &fakeClock{now: time.Now()})

	status := svc.Status(t.Context())

	assert.Equal(t, UpdateCheckResponse{
		CurrentVersion:  "0.6.0",
		LatestVersion:   "0.8.0",
		ReleasesBehind:  2,
		UpdateAvailable: true,
	}, status)
	assert.Equal(t, "Shisho/test", gh.userAgent)
}

func TestService_Status_UpToDate(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.8.0"), release("v0.7.0"))
	svc := newTestService(gh, "0.8.0", &fakeClock{now: time.Now()})

	status := svc.Status(t.Context())

	assert.Equal(t, UpdateCheckResponse{CurrentVersion: "0.8.0", LatestVersion: "0.8.0"}, status)
}

// Release images are built with the tag as the version ("v0.6.0"), and both
// versions in the response are shown side by side, so they share one form.
func TestService_Status_DropsTheVFromTheRunningVersion(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.8.0"), release("v0.7.0"), release("v0.6.0"))
	svc := newTestService(gh, "v0.6.0", &fakeClock{now: time.Now()})

	assert.Equal(t, UpdateCheckResponse{
		CurrentVersion:  "0.6.0",
		LatestVersion:   "0.8.0",
		ReleasesBehind:  2,
		UpdateAvailable: true,
	}, svc.Status(t.Context()))
}

func TestService_Status_ReusesResultUntilItGoesStale(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.7.0"))
	clock := &fakeClock{now: time.Now()}
	svc := newTestService(gh, "0.7.0", clock)

	assert.False(t, svc.Status(t.Context()).UpdateAvailable)
	gh.setReleases(release("v0.8.0"), release("v0.7.0"))
	clock.Advance(checkInterval - time.Minute)
	assert.False(t, svc.Status(t.Context()).UpdateAvailable)
	assert.Equal(t, int32(1), gh.requests.Load())

	clock.Advance(2 * time.Minute)
	status := svc.Status(t.Context())
	assert.True(t, status.UpdateAvailable)
	assert.Equal(t, "0.8.0", status.LatestVersion)
	assert.Equal(t, int32(2), gh.requests.Load())
}

func TestService_Status_KeepsLastResultAndBacksOffWhenGitHubFails(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.8.0"), release("v0.7.0"))
	clock := &fakeClock{now: time.Now()}
	svc := newTestService(gh, "0.7.0", clock)
	require.True(t, svc.Status(t.Context()).UpdateAvailable)

	gh.setStatus(http.StatusInternalServerError)
	clock.Advance(checkInterval + time.Minute)
	status := svc.Status(t.Context())
	assert.True(t, status.UpdateAvailable, "a failed check keeps the last result")
	assert.Equal(t, "0.8.0", status.LatestVersion)
	assert.Equal(t, int32(2), gh.requests.Load())

	clock.Advance(retryInterval - time.Minute)
	svc.Status(t.Context())
	assert.Equal(t, int32(2), gh.requests.Load(), "no retry before the backoff ends")

	clock.Advance(2 * time.Minute)
	svc.Status(t.Context())
	assert.Equal(t, int32(3), gh.requests.Load())
}

func TestService_Status_FirstCheckFailing(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t)
	gh.setStatus(http.StatusForbidden)
	svc := newTestService(gh, "0.7.0", &fakeClock{now: time.Now()})

	assert.Equal(t, UpdateCheckResponse{CurrentVersion: "0.7.0"}, svc.Status(t.Context()))
}

func TestService_Status_SkipsCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		enabled bool
		current string
	}{
		{"disabled", false, "0.7.0"},
		{"development build", true, "dev"},
		{"unparseable version", true, "nightly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gh := newFakeGitHub(t, release("v0.8.0"))
			svc := NewService(ServiceConfig{
				Enabled:        tc.enabled,
				CurrentVersion: tc.current,
				ReleasesURL:    gh.server.URL,
			})

			assert.Equal(t, UpdateCheckResponse{CurrentVersion: tc.current}, svc.Status(t.Context()))
			assert.Equal(t, int32(0), gh.requests.Load())
		})
	}
}

// A check runs to completion even when the request that started it goes
// away, so the result is cached for the next one.
func TestService_Status_FinishesCheckAfterCallerCancels(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.8.0"), release("v0.7.0"))
	svc := newTestService(gh, "0.7.0", &fakeClock{now: time.Now()})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.True(t, svc.Status(ctx).UpdateAvailable)
}

func TestService_Status_ConcurrentCallersShareOneCheck(t *testing.T) {
	t.Parallel()
	gh := newFakeGitHub(t, release("v0.8.0"), release("v0.7.0"))
	svc := newTestService(gh, "0.7.0", &fakeClock{now: time.Now()})

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.True(t, svc.Status(t.Context()).UpdateAvailable)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), gh.requests.Load())
}
