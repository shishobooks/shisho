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
		wantReset        bool
	}{
		{
			name:             "scan mode",
			payload:          ResyncPayload{Mode: ResyncModeScan},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
			wantReset:        false,
		},
		{
			name:             "refresh mode",
			payload:          ResyncPayload{Mode: ResyncModeRefresh},
			wantForceRefresh: true,
			wantSkipPlugins:  false,
			wantReset:        false,
		},
		{
			name:             "reset mode",
			payload:          ResyncPayload{Mode: ResyncModeReset},
			wantForceRefresh: true,
			wantSkipPlugins:  true,
			wantReset:        true,
		},
		{
			name:             "empty mode defaults to scan",
			payload:          ResyncPayload{},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
			wantReset:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			forceRefresh, skipPlugins, reset := tt.payload.resolveScanMode()
			assert.Equal(t, tt.wantForceRefresh, forceRefresh, "forceRefresh")
			assert.Equal(t, tt.wantSkipPlugins, skipPlugins, "skipPlugins")
			assert.Equal(t, tt.wantReset, reset, "reset")
		})
	}
}
