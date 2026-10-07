package devtool

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

// FilePortAllocator coordinates ports between worktrees with advisory locks.
// A port is only free if no other worktree holds its lock and nothing is
// listening on it. The lock closes the gap between choosing a port and the
// server binding it, and the kernel drops it if the process dies.
type FilePortAllocator struct {
	LockRoot string
}

type filePortLease struct {
	port int
	file *os.File
}

// Acquire locks the first available port at or above preferred.
func (a *FilePortAllocator) Acquire(preferred int) (PortLease, error) {
	if err := os.MkdirAll(a.LockRoot, 0o755); err != nil {
		return nil, errors.Wrap(err, "create port lock directory")
	}

	for port := preferred; port <= 65535; port++ {
		path := filepath.Join(a.LockRoot, strconv.Itoa(port)+".lock")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, errors.Wrap(err, "open port lock")
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = file.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				continue
			}
			return nil, errors.Wrapf(err, "lock port %d", port)
		}
		if portAvailable(port) {
			return &filePortLease{port: port, file: file}, nil
		}
		_ = file.Close()
	}
	return nil, errors.Errorf("no available port at or above %d", preferred)
}

func (l *filePortLease) Port() int {
	return l.port
}

// Close releases the lock. Closing the file drops the flock.
func (l *filePortLease) Close() error {
	return errors.WithStack(l.file.Close())
}

// portAvailable checks every address another server might hold the port on.
// The API and Vite bind all interfaces, so a port held on any one of them is
// unusable. A port held only on ::1 is unusable too: our servers could still
// bind, but browsers resolve localhost to ::1 first and would load the other
// server instead.
//
// Each probe sticks to one address family. A "tcp" listener on 0.0.0.0 opens
// a dual-stack socket, which macOS lets bind beside a server holding only
// IPv4 0.0.0.0, so it would report that port as free.
func portAvailable(port int) bool {
	probes := []struct{ network, host string }{
		{"tcp4", "127.0.0.1"},
		{"tcp4", "0.0.0.0"},
		{"tcp6", "::1"},
		{"tcp6", "::"},
	}
	for _, probe := range probes {
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), probe.network, net.JoinHostPort(probe.host, strconv.Itoa(port)))
		if err != nil {
			// A machine without IPv6 can't refuse the port over it.
			if probe.network == "tcp6" && isAddressUnavailable(err) {
				continue
			}
			return false
		}
		_ = listener.Close()
	}
	return true
}

func isAddressUnavailable(err error) bool {
	return errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EAFNOSUPPORT)
}
