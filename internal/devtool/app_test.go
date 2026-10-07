package devtool

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProcesses struct {
	mode     string
	apiPort  int
	webPort  int
	docsPort int
}

func (p *fakeProcesses) Start(_ context.Context, _ Environment, mode string, apiPort, webPort int) error {
	p.mode = mode
	p.apiPort = apiPort
	p.webPort = webPort
	return nil
}

func (p *fakeProcesses) Docs(_ context.Context, _ Environment, port int) error {
	p.mode = "docs"
	p.docsPort = port
	return nil
}

// fakePorts hands out the next port in its list and records which port each
// caller asked for.
type fakePorts struct {
	ports     []int
	preferred []int
}

func (p *fakePorts) Acquire(preferred int) (PortLease, error) {
	p.preferred = append(p.preferred, preferred)
	if len(p.ports) == 0 {
		return nil, errors.New("no port")
	}
	port := p.ports[0]
	p.ports = p.ports[1:]
	return fakeLease(port), nil
}

type fakeLease int

func (l fakeLease) Port() int  { return int(l) }
func (fakeLease) Close() error { return nil }

func testApp(ports *fakePorts, processes *fakeProcesses) *App {
	root := "/tmp/shisho"
	return &App{
		Stdout: &bytes.Buffer{},
		Resolve: func(context.Context) (Environment, error) {
			return Environment{MainRoot: root, CurrentRoot: root}, nil
		},
		Ports:     ports,
		Processes: processes,
	}
}

func TestStartReservesAPIAndWebPorts(t *testing.T) {
	t.Parallel()

	ports := &fakePorts{ports: []int{3690, 5174}}
	processes := &fakeProcesses{}
	app := testApp(ports, processes)

	require.NoError(t, app.Run(context.Background(), []string{"start"}))

	assert.Equal(t, []int{3689, 5173}, ports.preferred)
	assert.Equal(t, "all", processes.mode)
	assert.Equal(t, 3690, processes.apiPort)
	assert.Equal(t, 5174, processes.webPort)
}

func TestStartAPIModesReserveOnlyTheAPIPort(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"air", "api"} {
		ports := &fakePorts{ports: []int{3690}}
		processes := &fakeProcesses{}
		app := testApp(ports, processes)

		require.NoError(t, app.Run(context.Background(), []string{"start", mode}))

		assert.Equal(t, []int{3689}, ports.preferred, mode)
		assert.Equal(t, mode, processes.mode)
		assert.Equal(t, 3690, processes.apiPort, mode)
		assert.Zero(t, processes.webPort, mode)
	}
}

func TestStartWebReservesOnlyTheWebPort(t *testing.T) {
	t.Parallel()

	ports := &fakePorts{ports: []int{5174}}
	processes := &fakeProcesses{}
	app := testApp(ports, processes)

	require.NoError(t, app.Run(context.Background(), []string{"start", "web"}))

	assert.Equal(t, []int{5173}, ports.preferred)
	assert.Equal(t, "web", processes.mode)
	assert.Zero(t, processes.apiPort)
	assert.Equal(t, 5174, processes.webPort)
}

func TestDocsReservesTheDocsPort(t *testing.T) {
	t.Parallel()

	ports := &fakePorts{ports: []int{3001}}
	processes := &fakeProcesses{}
	app := testApp(ports, processes)

	require.NoError(t, app.Run(context.Background(), []string{"docs"}))

	assert.Equal(t, []int{3000}, ports.preferred)
	assert.Equal(t, 3001, processes.docsPort)
}

func TestRunRejectsUnknownCommands(t *testing.T) {
	t.Parallel()

	app := testApp(&fakePorts{}, &fakeProcesses{})

	require.Error(t, app.Run(context.Background(), nil))
	require.Error(t, app.Run(context.Background(), []string{"deploy"}))
	require.Error(t, app.Run(context.Background(), []string{"start", "everything"}))
	require.Error(t, app.Run(context.Background(), []string{"start", "air", "web"}))
}

func TestPortAllocatorSkipsAPortHeldByAnotherProject(t *testing.T) {
	t.Parallel()

	// Another project's dev server may hold the port on any of these. tcp4
	// and tcp6 keep each holder to one address family, since "tcp" on
	// 0.0.0.0 opens a dual-stack socket and would hide an IPv4-only holder.
	// One bound to only ::1 matters too: the browser resolves localhost to
	// ::1 first, so it would load that project instead of ours.
	holders := []struct{ network, host string }{
		{"tcp4", "127.0.0.1"},
		{"tcp4", "0.0.0.0"},
		{"tcp6", "::1"},
		{"tcp6", "::"},
	}
	for _, holder := range holders {
		listener, err := (&net.ListenConfig{}).Listen(t.Context(), holder.network, net.JoinHostPort(holder.host, "0"))
		if err != nil {
			t.Logf("skipping %s %s: %v", holder.network, holder.host, err)
			continue
		}
		occupied := listener.Addr().(*net.TCPAddr).Port

		lease, err := (&FilePortAllocator{LockRoot: t.TempDir()}).Acquire(occupied)
		require.NoError(t, err)
		assert.NotEqual(t, occupied, lease.Port(), "port held on %s %s", holder.network, holder.host)

		require.NoError(t, lease.Close())
		require.NoError(t, listener.Close())
	}
}

func TestPortAllocatorSkipsAPortLeasedByAnotherWorktree(t *testing.T) {
	t.Parallel()

	// A leased port is free until its server binds it, so only the lock tells
	// a second worktree to keep looking.
	lockRoot := t.TempDir()
	first, err := (&FilePortAllocator{LockRoot: lockRoot}).Acquire(freePort(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	second, err := (&FilePortAllocator{LockRoot: lockRoot}).Acquire(first.Port())
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	assert.Greater(t, second.Port(), first.Port())
}

func TestPortAllocatorReleasesTheLockOnClose(t *testing.T) {
	t.Parallel()

	// Checks the lock itself rather than acquiring the port again, since a
	// parallel test may bind the port as soon as the lease lets go of it.
	lockRoot := t.TempDir()
	lease, err := (&FilePortAllocator{LockRoot: lockRoot}).Acquire(freePort(t))
	require.NoError(t, err)

	lockPath := filepath.Join(lockRoot, strconv.Itoa(lease.Port())+".lock")
	file, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	require.ErrorIs(t, syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB), syscall.EWOULDBLOCK)
	require.NoError(t, lease.Close())
	// A process forked by a parallel test shares the lock until it execs.
	assert.Eventually(t, func() bool {
		return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestResolveEnvironmentFindsTheMainWorktree(t *testing.T) {
	t.Parallel()

	mainRoot := t.TempDir()
	git(t, mainRoot, "init", "--quiet")
	git(t, mainRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "--allow-empty", "-m", "init")
	linkedRoot := filepath.Join(t.TempDir(), "linked")
	git(t, mainRoot, "worktree", "add", "--quiet", linkedRoot)

	env, err := resolveEnvironmentAt(context.Background(), linkedRoot)
	require.NoError(t, err)

	expectedMain, err := filepath.EvalSymlinks(mainRoot)
	require.NoError(t, err)
	expectedLinked, err := filepath.EvalSymlinks(linkedRoot)
	require.NoError(t, err)
	assert.Equal(t, expectedMain, env.MainRoot)
	assert.Equal(t, expectedLinked, env.CurrentRoot)
	assert.Equal(t, filepath.Join(expectedMain, "tmp", "ports"), env.PortLockRoot())
}

func TestDevelopmentEnvironmentPassesTheReservedPorts(t *testing.T) {
	t.Parallel()

	env := Environment{MainRoot: "/code/shisho", CurrentRoot: "/code/shisho"}
	values := developmentEnvironment([]string{"SERVER_PORT=1", "PATH=/bin"}, env, 3690)

	assert.Equal(t, "3690", lastValue(values, "SERVER_PORT"))
	assert.Equal(t, "3690", lastValue(values, "API_PORT"))
	assert.Equal(t, "/code/shisho/tmp/api.port", lastValue(values, "SHISHO_DEV_PORT_FILE"))
	assert.Empty(t, lastValue(values, "WEB_PORT"))
	assert.Equal(t, "/bin", lastValue(values, "PATH"))
}

func TestDevelopmentEnvironmentNamespacesCookiesInLinkedWorktrees(t *testing.T) {
	t.Parallel()

	// Cookies ignore the port, so each worktree on localhost needs its own
	// session cookie name or signing in to one signs out of the others.
	main := Environment{MainRoot: "/code/shisho", CurrentRoot: "/code/shisho"}
	assert.Empty(t, lastValue(developmentEnvironment([]string{"SHISHO_COOKIE_NAMESPACE=stale"}, main, 3689), "SHISHO_COOKIE_NAMESPACE"))

	linked := Environment{MainRoot: "/code/shisho", CurrentRoot: "/worktrees/shisho/My Feature!"}
	namespace := lastValue(developmentEnvironment(nil, linked, 3690), "SHISHO_COOKIE_NAMESPACE")
	assert.Regexp(t, `^my_feature_[a-f0-9]{8}$`, namespace)

	other := Environment{MainRoot: "/code/shisho", CurrentRoot: "/elsewhere/My Feature!"}
	assert.NotEqual(t, namespace, lastValue(developmentEnvironment(nil, other, 3690), "SHISHO_COOKIE_NAMESPACE"))
}

func TestRecordedAPIPortReadsTheWorktreesPortFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := Environment{MainRoot: root, CurrentRoot: root}

	_, err := recordedAPIPort(env)
	require.Error(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "tmp"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tmp", "api.port"), []byte("3691\n"), 0o600))

	port, err := recordedAPIPort(env)
	require.NoError(t, err)
	assert.Equal(t, 3691, port)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func lastValue(values []string, key string) string {
	prefix := key + "="
	for _, value := range slices.Backward(values) {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return value[len(prefix):]
		}
	}
	return ""
}
