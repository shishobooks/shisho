package devtool

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// OSProcesses runs development commands as child process groups.
type OSProcesses struct{}

// Start runs the servers for a start mode on the reserved ports. A zero port
// means the mode does not run that server.
func (p *OSProcesses) Start(ctx context.Context, env Environment, mode string, apiPort, webPort int) error {
	values := developmentEnvironment(os.Environ(), env, apiPort)
	if apiPort != 0 {
		// The API removes its port file only on a graceful shutdown. Once this
		// launcher exits it no longer holds the port, so a file left by a
		// killed API could send a later mise start:web to another worktree.
		defer func() { _ = os.Remove(apiPortPath(env)) }()
	}

	switch mode {
	case "api":
		return runSingleProcess(ctx, env.CurrentRoot, values, "go", "run", "./cmd/api")
	case "air":
		return runSingleProcess(ctx, env.CurrentRoot, values, "air")
	case "web":
		return runWebAlone(ctx, env, values, webPort)
	case "all":
		return runDevelopmentProcesses(ctx, env.CurrentRoot, values, apiPort, webPort)
	default:
		return errors.Errorf("unknown process mode %q", mode)
	}
}

// Docs runs the documentation dev server.
func (p *OSProcesses) Docs(ctx context.Context, env Environment, port int) error {
	return runSingleProcess(ctx, filepath.Join(env.CurrentRoot, "website"), os.Environ(), "pnpm", "start", "--port", strconv.Itoa(port))
}

// developmentEnvironment adds this worktree's settings to base. SERVER_PORT
// moves the API, API_PORT points Vite's proxy at it, SHISHO_DEV_PORT_FILE has
// the API record its port for a separate mise start:web, and
// SHISHO_COOKIE_NAMESPACE keeps its session cookie apart from other worktrees
// on localhost. Later entries win in a child process, so these override
// anything exported in the shell.
func developmentEnvironment(base []string, env Environment, apiPort int) []string {
	values := append([]string{}, base...)
	values = append(values, "SHISHO_COOKIE_NAMESPACE="+env.CookieNamespace())
	if apiPort != 0 {
		values = append(values,
			"SERVER_PORT="+strconv.Itoa(apiPort),
			"API_PORT="+strconv.Itoa(apiPort),
			"SHISHO_DEV_PORT_FILE="+apiPortPath(env),
		)
	}
	return values
}

func webCommandArgs(webPort int) []string {
	return []string{"start", "--port", strconv.Itoa(webPort), "--strictPort"}
}

func runDevelopmentProcesses(ctx context.Context, root string, values []string, apiPort, webPort int) error {
	api := newProcess(root, values, "air")
	if err := api.Start(); err != nil {
		return errors.Wrap(err, "start API")
	}
	apiDone := waitForProcess(api)
	apiExited, err := waitForAPI(ctx, apiDone, func() (int, error) { return apiPort, nil })
	if err != nil {
		if !apiExited {
			terminateProcess(api)
			awaitTermination(api, apiDone)
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	web := newProcess(root, values, "pnpm", webCommandArgs(webPort)...)
	if err := web.Start(); err != nil {
		terminateProcess(api)
		awaitTermination(api, apiDone)
		return errors.Wrap(err, "start web server")
	}
	webDone := waitForProcess(web)

	select {
	case err := <-apiDone:
		terminateProcess(web)
		awaitTermination(web, webDone)
		return processExitError("API", err)
	case err := <-webDone:
		terminateProcess(api)
		awaitTermination(api, apiDone)
		return processExitError("web server", err)
	case <-ctx.Done():
		terminateProcess(api)
		terminateProcess(web)
		awaitTermination(api, apiDone)
		awaitTermination(web, webDone)
		return nil
	}
}

// runWebAlone serves the frontend against an API that this worktree started
// separately, such as with mise start:air in another terminal. An API the
// launcher starts records its port in tmp/api.port, so that file names the
// right API even when another worktree holds the default port.
func runWebAlone(ctx context.Context, env Environment, values []string, webPort int) error {
	fmt.Println("Waiting for this worktree's API (start it with mise start:air)...")
	never := make(chan error)
	if _, err := waitForAPI(ctx, never, func() (int, error) { return recordedAPIPort(env) }); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	apiPort, err := recordedAPIPort(env)
	if err != nil {
		return err
	}
	values = append(values, "API_PORT="+strconv.Itoa(apiPort))
	return runSingleProcess(ctx, env.CurrentRoot, values, "pnpm", webCommandArgs(webPort)...)
}

func apiPortPath(env Environment) string {
	return filepath.Join(env.CurrentRoot, "tmp", "api.port")
}

func recordedAPIPort(env Environment) (int, error) {
	contents, err := os.ReadFile(apiPortPath(env))
	if err != nil {
		return 0, errors.Wrap(err, "read API port")
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		return 0, errors.Wrap(err, "read API port")
	}
	return port, nil
}

func runSingleProcess(ctx context.Context, dir string, values []string, name string, args ...string) error {
	command := newProcess(dir, values, name, args...)
	if err := command.Start(); err != nil {
		return errors.Wrapf(err, "start %s", name)
	}
	done := waitForProcess(command)
	select {
	case err := <-done:
		return processExitError(name, err)
	case <-ctx.Done():
		terminateProcess(command)
		awaitTermination(command, done)
		return nil
	}
}

// newProcess puts each child in its own process group so that stopping it
// also stops what it started, such as pnpm's Vite process. Air starts the API
// binary in a group of its own and stops it itself. It does not take a
// context because canceling one would kill only the group's leader.
//
// Children get no stdin. A process outside the terminal's foreground group
// that reads the terminal is stopped with SIGTTIN, and Vite reads stdin for
// its keyboard shortcuts.
func newProcess(dir string, values []string, name string, args ...string) *exec.Cmd {
	command := exec.Command(name, args...) //nolint:noctx // stopped through terminateProcess
	command.Dir = dir
	command.Env = values
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return command
}

func waitForProcess(command *exec.Cmd) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	return done
}

func terminateProcess(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
	}
}

// awaitTermination gives the API time for its graceful shutdown before
// killing the group.
func awaitTermination(command *exec.Cmd, done <-chan error) {
	select {
	case <-done:
		return
	case <-time.After(15 * time.Second):
		if command != nil && command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
		<-done
	}
}

// waitForAPI polls /health on the port that port returns until the API
// answers. It has no deadline: when the first build fails, air keeps running
// and rebuilds once the code is fixed, as the shell loop it replaces did. It
// reports whether the API process exited while waiting.
func waitForAPI(ctx context.Context, exited <-chan error, port func() (int, error)) (bool, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}

	for {
		select {
		case err := <-exited:
			if err == nil {
				return true, errors.New("API exited before becoming ready")
			}
			return true, processExitError("API", err)
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			p, err := port()
			if err != nil {
				continue
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/health", p)
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return false, errors.Wrap(err, "create API health request")
			}
			response, err := client.Do(request)
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					return false, nil
				}
			}
		}
	}
}

func processExitError(name string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return errors.Wrapf(err, "%s exited", name)
}
