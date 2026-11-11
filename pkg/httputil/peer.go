package httputil

import "net/netip"

// TrustedPeer reports whether a direct peer may set forwarding headers:
// loopback, private, or link-local, as Caddy's private_ranges has it. The
// server strips X-Forwarded-* from any other peer.
func TrustedPeer(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
