package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

func runIOS(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("ios", flag.ExitOnError)
	app := flags.String("app", "mobile/ios/build/DerivedData/Build/Products/Debug-iphonesimulator/Shisho.app", "the app built for the simulator")
	flowDir := flags.String("flows", "mobile/harness/flows", "folder of Maestro flows to run")
	keep := flags.Bool("keep", false, "keep the run's folder (server log, database, Maestro output) for debugging")
	out := flags.String("out", "", "make the run's folder here, and keep it")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	appPath, err := filepath.Abs(*app)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := os.Stat(appPath); err != nil {
		return errors.Errorf("no app at %s; build it first with `mise ios:build`", appPath)
	}
	// The bundle id the flows launch, which Signing.local.xcconfig may
	// have changed.
	bundleID, err := exec.CommandContext(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(appPath, "Info.plist")).Output() //nolint:gosec // The app this run built.
	if err != nil {
		return errors.Wrap(err, "read the app's bundle id")
	}
	appID := strings.TrimSpace(string(bundleID))
	flows, err := filepath.Abs(*flowDir)
	if err != nil {
		return errors.WithStack(err)
	}

	var dir string
	if *out != "" {
		dir, err = filepath.Abs(*out)
		if err == nil {
			err = os.MkdirAll(dir, 0o755)
		}
		*keep = true
	} else {
		dir, err = os.MkdirTemp("", "shisho-harness-")
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if *keep {
		fmt.Fprintln(os.Stderr, "appharness: keeping", dir)
	} else {
		defer os.RemoveAll(dir)
	}

	library := filepath.Join(dir, "library")
	if err := writeFixtures(ctx, library); err != nil {
		return errors.Wrap(err, "make fixtures")
	}
	srv, err := startServer(ctx, root, dir, "127.0.0.1", 0)
	if err != nil {
		return err
	}
	defer srv.Stop()
	if err := srv.seed(ctx, library, harnessPassword); err != nil {
		return err
	}

	control, err := startControl(ctx, srv)
	if err != nil {
		return err
	}
	defer control.Close()

	clone, err := cloneSimulator(ctx)
	if err != nil {
		return err
	}
	defer clone.Remove()
	if _, err := simctl(ctx, "install", clone.UDID, appPath); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "appharness: server %s, simulator %s\n", srv.URL, clone.UDID)
	//nolint:gosec // The harness's own flags and paths.
	maestro := exec.CommandContext(ctx, "maestro", "--device", clone.UDID, "test",
		"--debug-output", filepath.Join(dir, "maestro"),
		"-e", "APP_ID="+appID,
		"-e", "SERVER="+srv.URL,
		"-e", "USERNAME="+harnessUsername,
		"-e", "PASSWORD="+harnessPassword,
		"-e", "CONTROL="+control.URL,
		flows)
	maestro.Stdout, maestro.Stderr = os.Stdout, os.Stderr
	// Maestro installs and starts its driver on the simulator first, which
	// takes minutes on a CI runner, far longer than Maestro's default.
	if os.Getenv("MAESTRO_DRIVER_STARTUP_TIMEOUT") == "" {
		maestro.Env = append(os.Environ(), "MAESTRO_DRIVER_STARTUP_TIMEOUT=600000")
	}
	if err := maestro.Run(); err != nil {
		if !*keep {
			fmt.Fprintln(os.Stderr, "appharness: a flow failed; run with -keep to keep the server log and Maestro's screenshots")
		}
		return errors.Wrap(err, "maestro")
	}
	return nil
}

// control is an HTTP endpoint the flows call to change the world around the
// app, such as taking the server offline, since a flow can't reach the
// host's processes itself.
type control struct {
	URL      string
	listener net.Listener
}

func startControl(ctx context.Context, srv *server) (*control, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var mu sync.Mutex
	var offlineAt, onlineAt time.Time
	mux := http.NewServeMux()
	// The app can't reach a stopped server, as it can't in airplane mode.
	mux.HandleFunc("POST /offline", func(w http.ResponseWriter, _ *http.Request) {
		srv.Stop()
		mu.Lock()
		offlineAt = time.Now()
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	// The server comes back on the same address and data. A flow that needs
	// the server calls it first, in case an earlier flow left it offline.
	mux.HandleFunc("POST /online", func(w http.ResponseWriter, r *http.Request) {
		if err := srv.Start(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		onlineAt = time.Now()
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	// What the server holds of the harness user's listening to the Book
	// titled ?title=, read from its database, which works offline too.
	mux.HandleFunc("GET /reading", func(w http.ResponseWriter, r *http.Request) {
		state, err := readReading(r.Context(), filepath.Join(srv.dir, "data.sqlite"), r.URL.Query().Get("title"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		state.OfflineAt, state.OnlineAt = unixOrZero(offlineAt), unixOrZero(onlineAt)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	})
	go func() { _ = http.Serve(listener, mux) }()
	return &control{URL: "http://" + strings.TrimPrefix(listener.Addr().String(), "http://"), listener: listener}, nil
}

func (c *control) Close() {
	_ = c.listener.Close()
}
