package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShareLinkState_ExpiresAtBoundary(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	tests := []struct {
		name string
		link ShareLink
		want string
	}{
		{"no expiry", ShareLink{}, ShareLinkStateActive},
		{"expires a nanosecond from now", ShareLink{ExpiresAt: at(time.Nanosecond)}, ShareLinkStateActive},
		{"expires exactly now", ShareLink{ExpiresAt: at(0)}, ShareLinkStateExpired},
		{"expired a nanosecond ago", ShareLink{ExpiresAt: at(-time.Nanosecond)}, ShareLinkStateExpired},
		{"revoked before expiry", ShareLink{ExpiresAt: at(time.Hour), RevokedAt: at(-time.Hour)}, ShareLinkStateRevoked},
		{"revoked and expired", ShareLink{ExpiresAt: at(-time.Hour), RevokedAt: at(-2 * time.Hour)}, ShareLinkStateRevoked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.link.State(now))
		})
	}
}
