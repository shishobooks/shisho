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
// full bcrypt cost. The trade-off: a deactivated user or rotated password
// remains usable for OPDS for up to this window.
const defaultBasicAuthCacheTTL = 60 * time.Second

type basicAuthCacheEntry struct {
	user      *models.User
	expiresAt time.Time
}

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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = basicAuthCacheEntry{
		user:      user,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// basicAuthCacheKey derives a cache key from the credentials without holding
// the plaintext password in memory. The username is kept verbatim and
// concatenated through ":" — already the Basic Auth separator, so it cannot
// appear in the username — guaranteeing distinct (user, password) pairs map
// to distinct keys.
func basicAuthCacheKey(username, password string) string {
	sum := sha256.Sum256([]byte(password))
	return username + ":" + base64.RawStdEncoding.EncodeToString(sum[:])
}
