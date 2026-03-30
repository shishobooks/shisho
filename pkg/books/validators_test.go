package books

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveScanMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		payload          ResyncPayload
		wantForceRefresh bool
		wantSkipPlugins  bool
	}{
		{
			name:             "scan mode",
			payload:          ResyncPayload{Mode: "scan"},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
		{
			name:             "refresh mode",
			payload:          ResyncPayload{Mode: "refresh"},
			wantForceRefresh: true,
			wantSkipPlugins:  false,
		},
		{
			name:             "reset mode",
			payload:          ResyncPayload{Mode: "reset"},
			wantForceRefresh: true,
			wantSkipPlugins:  true,
		},
		{
			name:             "empty mode without refresh",
			payload:          ResyncPayload{},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
		{
			name:             "empty mode with refresh true (backwards compat)",
			payload:          ResyncPayload{Refresh: true},
			wantForceRefresh: true,
			wantSkipPlugins:  false,
		},
		{
			name:             "unknown mode defaults to scan",
			payload:          ResyncPayload{Mode: "unknown"},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			forceRefresh, skipPlugins := tt.payload.resolveScanMode()
			assert.Equal(t, tt.wantForceRefresh, forceRefresh, "forceRefresh")
			assert.Equal(t, tt.wantSkipPlugins, skipPlugins, "skipPlugins")
		})
	}
}
