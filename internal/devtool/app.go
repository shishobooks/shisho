// Package devtool runs the development servers so that several worktrees can
// run them at once without colliding on ports.
package devtool

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
)

// Each server starts looking at its usual port and takes the first one that
// no other worktree holds.
const (
	defaultAPIPort  = 3689
	defaultWebPort  = 5173
	defaultDocsPort = 3000
)

// PortLease reserves a port until Close is called.
type PortLease interface {
	Port() int
	Close() error
}

// PortAllocator reserves the first available port at or above a preferred port.
type PortAllocator interface {
	Acquire(preferred int) (PortLease, error)
}

// ProcessManager runs the development servers.
type ProcessManager interface {
	Start(ctx context.Context, env Environment, mode string, apiPort, webPort int) error
	Docs(ctx context.Context, env Environment, port int) error
}

// App implements the commands behind the mise start and docs tasks.
type App struct {
	Stdout    io.Writer
	Resolve   func(context.Context) (Environment, error)
	Ports     PortAllocator
	Processes ProcessManager
}

// NewApp returns an App that runs real processes from the current worktree.
func NewApp() *App {
	return &App{
		Stdout:    os.Stdout,
		Resolve:   ResolveEnvironment,
		Processes: &OSProcesses{},
	}
}

// Run dispatches a command: "start [all|air|api|web]" or "docs".
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}

	env, err := a.Resolve(ctx)
	if err != nil {
		return errors.Wrap(err, "resolve development environment")
	}
	ports := a.Ports
	if ports == nil {
		ports = &FilePortAllocator{LockRoot: env.PortLockRoot()}
	}

	switch args[0] {
	case "start":
		return a.start(ctx, env, ports, args[1:])
	case "docs":
		if len(args) > 1 {
			return errors.New("docs takes no arguments")
		}
		return a.docs(ctx, env, ports)
	default:
		return errors.Errorf("unknown command %q", args[0])
	}
}

func (a *App) start(ctx context.Context, env Environment, ports PortAllocator, args []string) error {
	mode := "all"
	if len(args) > 1 {
		return errors.New("start accepts at most one mode")
	}
	if len(args) == 1 {
		mode = args[0]
	}
	if mode != "all" && mode != "air" && mode != "api" && mode != "web" {
		return errors.Errorf("unknown start mode %q", mode)
	}

	apiPort, webPort := 0, 0
	if mode != "web" {
		lease, err := ports.Acquire(defaultAPIPort)
		if err != nil {
			return errors.Wrap(err, "reserve API port")
		}
		defer func() { _ = lease.Close() }()
		apiPort = lease.Port()
		a.printf("API: http://localhost:%d\n", apiPort)
	}
	if mode == "all" || mode == "web" {
		lease, err := ports.Acquire(defaultWebPort)
		if err != nil {
			return errors.Wrap(err, "reserve web port")
		}
		defer func() { _ = lease.Close() }()
		webPort = lease.Port()
		a.printf("Web: http://localhost:%d\n", webPort)
	}
	return a.Processes.Start(ctx, env, mode, apiPort, webPort)
}

func (a *App) docs(ctx context.Context, env Environment, ports PortAllocator) error {
	lease, err := ports.Acquire(defaultDocsPort)
	if err != nil {
		return errors.Wrap(err, "reserve docs port")
	}
	defer func() { _ = lease.Close() }()
	a.printf("Docs: http://localhost:%d\n", lease.Port())
	return a.Processes.Docs(ctx, env, lease.Port())
}

func (a *App) printf(format string, values ...any) {
	_, _ = fmt.Fprintf(a.Stdout, format, values...)
}
