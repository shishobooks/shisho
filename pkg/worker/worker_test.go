package worker

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
)

// The worker uses the plugin service handed to New, so cmd/api/main.go can
// share one instance with the plugin Manager and the server.
func TestNew_UsesInjectedPluginService(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	pluginService := plugins.NewService(tc.db)

	w := New(&config.Config{WorkerProcesses: 1}, tc.db, pluginService, nil, nil, nil, nil, nil)

	assert.Same(t, pluginService, w.pluginService)
}

// Tests may pass a nil plugin service. The worker then builds its own, so
// scans that consult plugin settings still work.
func TestNew_BuildsPluginServiceWhenNil(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	w := New(&config.Config{WorkerProcesses: 1}, tc.db, nil, nil, nil, nil, nil, nil)

	assert.NotNil(t, w.pluginService)
}
