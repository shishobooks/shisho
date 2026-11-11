package koreaderplugin

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/shishobooks/shisho/pkg/httputil"
	"golang.org/x/time/rate"
)

const (
	// pairBurst is how many short codes one address may try at once, and
	// pairRefill how often it gets another try. A short code is one of
	// 36^6 and lives 30 minutes, so this leaves guessing hopeless while a
	// person mistyping a code a few times never notices.
	pairBurst  = 10
	pairRefill = 6 * time.Second
	// limiterIdle is how long an address keeps its bucket after its last
	// attempt. By then the bucket is full again, so forgetting it changes
	// nothing.
	limiterIdle = pairBurst * pairRefill
	// maxBuckets bounds how many addresses are tracked at once. Past it,
	// every new address shares one bucket, so a flood of addresses slows
	// pairing for the newcomers rather than growing memory.
	maxBuckets  = 10000
	overflowKey = "overflow"
)

// addressLimiter limits attempts per client address.
type addressLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	pruned  time.Time
}

type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}

func newAddressLimiter() *addressLimiter {
	return &addressLimiter{buckets: map[string]*bucket{}}
}

// allow reports whether address may make another attempt now, and counts
// it.
func (l *addressLimiter) allow(address string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.pruned) > limiterIdle {
		for key, b := range l.buckets {
			if now.Sub(b.seen) > limiterIdle {
				delete(l.buckets, key)
			}
		}
		l.pruned = now
	}
	b, ok := l.buckets[address]
	if !ok && len(l.buckets) >= maxBuckets {
		address = overflowKey
		b, ok = l.buckets[address]
	}
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(rate.Every(pairRefill), pairBurst)}
		l.buckets[address] = b
	}
	b.seen = now
	return b.limiter.AllowN(now, 1)
}

// clientAddress is the address to limit a request by. Echo's RealIP takes a
// client's own X-Real-IP or the first X-Forwarded-For entry, either of which
// a client sets to whatever it likes. So this takes the peer's address, or,
// from a trusted peer (a reverse proxy, as the server's forwarded-header
// handling decides), the last X-Forwarded-For entry, the one that proxy
// added. An IPv6 address counts by its /64, which one client holds whole.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	address := peer.Unmap()
	if httputil.TrustedPeer(peer) {
		if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
			entries := strings.Split(forwarded[len(forwarded)-1], ",")
			if last, err := netip.ParseAddr(strings.TrimSpace(entries[len(entries)-1])); err == nil {
				address = last.Unmap()
			}
		}
	}
	if address.Is6() {
		prefix, err := address.Prefix(64)
		if err == nil {
			return prefix.String()
		}
	}
	return address.String()
}
