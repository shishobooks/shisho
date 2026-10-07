package devtool

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessDoesNotShareTheTerminal(t *testing.T) {
	t.Parallel()

	// Each child runs in its own process group, so reading a terminal on
	// stdin would get it stopped with SIGTTIN. Vite reads stdin for its
	// keyboard shortcuts and froze before serving anything.
	command := newProcess(t.TempDir(), nil, "true")
	assert.Nil(t, command.Stdin)
	assert.True(t, command.SysProcAttr.Setpgid)
}

func healthServer(t *testing.T, status int) int {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().(*net.TCPAddr).Port
}

func TestWaitForAPIReturnsOnceHealthAnswers(t *testing.T) {
	t.Parallel()

	port := healthServer(t, http.StatusOK)
	// The port file appears only after the API starts.
	calls := 0
	exited, err := waitForAPI(t.Context(), make(chan error), func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("not written yet")
		}
		return port, nil
	})
	require.NoError(t, err)
	assert.False(t, exited)
}

func TestWaitForAPIReportsAnAPIThatExits(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	done <- errors.New("exit status 1")
	exited, err := waitForAPI(t.Context(), done, func() (int, error) { return healthServer(t, http.StatusServiceUnavailable), nil })
	require.Error(t, err)
	assert.True(t, exited)
}

func TestWaitForAPIStopsWhenCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	exited, err := waitForAPI(ctx, make(chan error), func() (int, error) { return 0, errors.New("never written") })
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, exited)
}

func TestStartRemovesTheAPIPortFileWhenTheAPIExits(t *testing.T) {
	// Not parallel: it puts a substitute go executable on PATH.

	// An API that is killed never removes its own port file. Once this
	// launcher exits, another worktree may take the port, and a later
	// mise start:web here would proxy to that worktree's API.
	root := t.TempDir()
	bin := t.TempDir()
	script := "#!/bin/sh\nmkdir -p tmp && echo 3690 > tmp/api.port\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	env := Environment{MainRoot: root, CurrentRoot: root}
	require.NoError(t, (&OSProcesses{}).Start(t.Context(), env, "api", 3690, 0))

	assert.NoFileExists(t, apiPortPath(env))
}
