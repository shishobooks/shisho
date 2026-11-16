package updatecheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/version"
)

const (
	// defaultReleasesURL lists Shisho's GitHub releases, newest first. One
	// page of 100 covers any server worth reminding.
	defaultReleasesURL = "https://api.github.com/repos/shishobooks/shisho/releases?per_page=100"

	// checkInterval is how long a successful check is reused. GitHub allows
	// 60 unauthenticated requests an hour per IP, and a release a day is
	// already frequent.
	checkInterval = 12 * time.Hour
	// retryInterval is how long a failed check waits before trying again, so
	// an offline or rate-limited server doesn't ask on every page load.
	retryInterval = time.Hour
	// requestTimeout bounds one request to GitHub.
	requestTimeout = 10 * time.Second
	// maxResponseBytes caps the release list read into memory. A page of 100
	// releases with their notes and asset lists is a few megabytes at most.
	maxResponseBytes = 16 << 20
)

// ServiceConfig holds options for constructing a Service.
type ServiceConfig struct {
	// Enabled turns checks on. When false, Status never contacts GitHub.
	Enabled bool
	// CurrentVersion is the running server's version. Checks are skipped
	// for a development build or any version that isn't semver.
	CurrentVersion string
	// ReleasesURL overrides the GitHub releases endpoint for tests.
	ReleasesURL string
	// UserAgent is sent with every request; GitHub rejects requests without
	// one.
	UserAgent string
	// HTTPClient is optional; if nil, a client with requestTimeout is used.
	HTTPClient *http.Client

	// now is the clock, settable by tests.
	now func() time.Time
}

// Service compares the running version with Shisho's published releases. It
// asks GitHub only when Status is called and the last answer has gone stale,
// so a server nobody opens never contacts GitHub.
type Service struct {
	enabled     bool
	current     string
	releasesURL string
	userAgent   string
	http        *http.Client
	now         func() time.Time

	// mu is held for the whole check, so concurrent callers wait for one
	// request instead of each sending their own.
	mu        sync.Mutex
	latest    string
	behind    int
	nextCheck time.Time
	// failing is true while checks keep failing, so only the first failure
	// of a run is logged as a warning. A server that can't reach GitHub
	// would otherwise warn every hour.
	failing bool
}

// NewService constructs a Service with the given config.
func NewService(cfg ServiceConfig) *Service {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	url := cfg.ReleasesURL
	if url == "" {
		url = defaultReleasesURL
	}
	now := cfg.now
	if now == nil {
		now = time.Now
	}
	// Release images are built with the tag as their version ("v0.8.0"),
	// while the latest release is reported without the "v", and the two are
	// shown side by side.
	current := strings.TrimPrefix(cfg.CurrentVersion, "v")
	return &Service{
		enabled:     cfg.Enabled && version.IsValid(current),
		current:     current,
		releasesURL: url,
		userAgent:   cfg.UserAgent,
		http:        client,
		now:         now,
	}
}

// Status returns how the running version compares with the latest release,
// checking GitHub first when the last result is stale. A failed check is
// logged and keeps the last result, since a missing reminder is harmless.
func (s *Service) Status(ctx context.Context) UpdateCheckResponse {
	if !s.enabled {
		return UpdateCheckResponse{CurrentVersion: s.current}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.now().Before(s.nextCheck) {
		// Finish the check even if the caller leaves, so its result is
		// cached for the next one.
		checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
		latest, behind, err := s.fetch(checkCtx)
		cancel()
		if err != nil {
			log := logger.FromContext(ctx)
			data := logger.Data{"error": err.Error()}
			if s.failing {
				log.Debug("failed to check for a newer Shisho release again", data)
			} else {
				log.Warn("failed to check for a newer Shisho release; retrying hourly", data)
			}
			s.failing = true
			s.nextCheck = s.now().Add(retryInterval)
		} else {
			s.latest, s.behind = latest, behind
			s.failing = false
			s.nextCheck = s.now().Add(checkInterval)
		}
	}

	return UpdateCheckResponse{
		CurrentVersion:  s.current,
		LatestVersion:   s.latest,
		ReleasesBehind:  s.behind,
		UpdateAvailable: s.behind > 0,
	}
}

// fetch returns the newest published release and how many published
// releases are newer than the running version. Drafts and prereleases are
// skipped. Compare ignores prerelease suffixes, so a server running
// "0.9.0-rc1" counts "0.9.0" as the same version and is told only about the
// release after it.
func (s *Service) fetch(ctx context.Context) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.releasesURL, nil)
	if err != nil {
		return "", 0, errors.WithStack(err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.http.Do(req)
	if err != nil {
		return "", 0, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, errors.Errorf("GitHub returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&releases); err != nil {
		return "", 0, errors.Wrap(err, "failed to decode GitHub releases")
	}

	latest := ""
	behind := 0
	for _, r := range releases {
		tag := strings.TrimPrefix(r.TagName, "v")
		if r.Draft || r.Prerelease || !version.IsValid(tag) {
			continue
		}
		if version.Compare(tag, s.current) > 0 {
			behind++
		}
		if latest == "" || version.Compare(tag, latest) > 0 {
			latest = tag
		}
	}
	return latest, behind, nil
}

// githubRelease holds the fields read from GitHub's release list.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
