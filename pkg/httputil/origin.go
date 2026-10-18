package httputil

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// PublicOrigin returns the scheme://host[:port] a client used to reach r, for
// building absolute URLs that devices follow. The forwarded headers it reads
// are trusted because the server middleware strips them from untrusted peers.
//
// Some clients, Kobo firmware among them, leave the port out of Host even on
// a non-default port. A direct request then gets the port the connection
// arrived on. A proxied request never does: the listener's port is not the
// one the client used, so a proxy must send X-Forwarded-Port or put the port
// in the host it forwards.
func PublicOrigin(r *http.Request) string {
	scheme := PublicScheme(r)

	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}

	if (&url.URL{Host: host}).Port() == "" {
		// 80 and 443 are skipped under either scheme: a proxy behind an
		// upstream TLS terminator sends X-Forwarded-Port: 80 with https.
		if port := originPort(r); port != "" && port != "80" && port != "443" {
			host += ":" + port
		}
	}

	return scheme + "://" + host
}

// PublicScheme returns "https" or "http", whichever the client used to reach
// r. Session cookies are Secure exactly when this is "https", so they agree
// with the URLs PublicOrigin builds.
func PublicScheme(r *http.Request) string {
	proto := strings.ToLower(firstForwardedValue(r.Header.Get("X-Forwarded-Proto")))
	if proto == "http" || proto == "https" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// originPort returns the port to add to a host that has none: X-Forwarded-Port
// when sent, otherwise the local port of a request with no forwarded headers.
// Any of them marks a proxy, whose local port is not the one the client used.
func originPort(r *http.Request) string {
	if port := firstForwardedValue(r.Header.Get("X-Forwarded-Port")); port != "" {
		return port
	}
	for _, name := range []string{"X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Prefix", "X-Forwarded-For"} {
		if r.Header.Get(name) != "" {
			return ""
		}
	}
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return ""
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return port
}

// firstForwardedValue returns the client-facing entry of a header that a
// chain of proxies may have appended to.
func firstForwardedValue(value string) string {
	first, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(first)
}
