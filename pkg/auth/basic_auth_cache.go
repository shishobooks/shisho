package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// defaultBasicAuthCacheTTL bounds how long a successful Basic Auth result is
// reused before re-running bcrypt + the user lookup. OPDS clients re-send
// credentials on every request, so without this cache each click pays the
// full bcrypt cost. Trade-off: a deactivated user, rotated password, or
// changed role/library-access stays in effect on OPDS for up to this window.
const defaultBasicAuthCacheTTL = 60 * time.Second

type basicAuthCacheEntry struct {
	user      *models.User
	expiresAt time.Time
}

// basicAuthCache caches successful Basic Auth results keyed by
// (username, sha256(password)). Eviction is lazy — entries are dropped on
// the next get() for that key. Memory growth is bounded by the number of
// distinct (user, password) pairs that successfully authenticate, so stale
// entries from password rotation linger at most until process restart but
// cannot grow unboundedly under attacker control (failed auths are not
// cached).
//
// The cached *models.User is shared across concurrent callers — treat it
// as read-only.
type basicAuthCache struct {
	mu      sync.Mutex
	entries map[string]basicAuthCacheEntry
	ttl     time.Duration
}

func newBasicAuthCache(ttl time.Duration) *basicAuthCache {
	return &basicAuthCache{
		entries: make(map[string]basicAuthCacheEntry),
		ttl:     ttl,
	}
}

func (c *basicAuthCache) get(key string) (*models.User, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.user, true
}

func (c *basicAuthCache) put(key string, user *models.User) {
	if user == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = basicAuthCacheEntry{
		user:      user,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// basicAuthCacheKey derives a cache key without retaining the plaintext
// password in memory. Distinct (username, password) pairs map to distinct
// keys: the username arrives here from strings.SplitN(decoded, ":", 2) in
// BasicAuth, so by construction it cannot contain a ":" — making
// `username + ":" + sha256(password)` an unambiguous concatenation.
func basicAuthCacheKey(username, password string) string {
	sum := sha256.Sum256([]byte(password))
	return username + ":" + base64.RawStdEncoding.EncodeToString(sum[:])
}
