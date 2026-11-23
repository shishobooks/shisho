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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

func runIOS(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("ios", flag.ExitOnError)
	app := flags.String("app", defaultAppPath, "the app built for the simulator")
	flowDir := flags.String("flows", "mobile/harness/flows", "folder of Maestro flows to run")
	keep := flags.Bool("keep", false, "keep the run's folder (server log, database, Maestro output) for debugging")
	out := flags.String("out", "", "make the run's folder here, and keep it")
	shard := flags.String("shard", "", "run only this shard of the flows, as i/n (CI's parallel jobs)")
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
	appID, err := appBundleID(ctx, appPath)
	if err != nil {
		return err
	}
	flows, err := filepath.Abs(*flowDir)
	if err != nil {
		return errors.WithStack(err)
	}
	flowArgs := []string{flows}
	if *shard != "" {
		if flowArgs, err = shardFlowPaths(flows, *shard); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "appharness: shard %s runs %d flows\n", *shard, len(flowArgs))
	}

	sweepLeftoverRunDirs()
	var dir string
	if *out != "" {
		if dir, err = filepath.Abs(*out); err != nil {
			return errors.WithStack(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errors.WithStack(err)
		}
		*keep = true
	} else {
		run, err := makeRunDir(os.TempDir(), flowsRunPrefix, *keep)
		if err != nil {
			return err
		}
		defer run.Close()
		dir = run.Path
	}
	if *keep {
		fmt.Fprintln(os.Stderr, "appharness: keeping", dir)
	}

	library := filepath.Join(dir, "library")
	if err := writeFixtures(ctx, root, library); err != nil {
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
	if err := cheapenPasswords(ctx, filepath.Join(dir, "data.sqlite")); err != nil {
		return err
	}

	control, err := startControl(ctx, srv)
	if err != nil {
		return err
	}
	defer control.Close()

	sim, err := simulatorForRun(ctx, root, "flows", appID)
	if err != nil {
		return err
	}
	defer sim.Release()
	if _, err := simctl(ctx, "install", sim.UDID, appPath); err != nil {
		return err
	}
	// Only the font import flow needs it, which fails on its own without it.
	if err := seedFiles(ctx, root, sim.UDID); err != nil {
		fmt.Fprintln(os.Stderr, "appharness: couldn't put the font in Files, so the import flow will fail:", err)
	}

	fmt.Fprintf(os.Stderr, "appharness: server %s, simulator %s\n", srv.URL, sim.UDID)
	//nolint:gosec // The harness's own flags and paths.
	maestro := exec.CommandContext(ctx, "maestro", append([]string{"--device", sim.UDID, "test",
		"--debug-output", filepath.Join(dir, "maestro"),
		"-e", "APP_ID=" + appID,
		"-e", "SERVER=" + srv.URL,
		"-e", "USERNAME=" + harnessUsername,
		"-e", "PASSWORD=" + harnessPassword,
		"-e", "READER_USERNAME=" + readerUsername,
		"-e", "READER_PASSWORD=" + readerPassword,
		"-e", "CONTROL=" + control.URL,
	}, flowArgs...)...)
	maestro.Stdout, maestro.Stderr = os.Stdout, os.Stderr
	stopWithProcessGroup(maestro)
	maestro.Env = append(os.Environ(), "MAESTRO_CLI_NO_ANALYTICS=1")
	// Maestro installs and starts its driver on the simulator first, which
	// takes minutes on a CI runner, far longer than Maestro's default.
	if os.Getenv("MAESTRO_DRIVER_STARTUP_TIMEOUT") == "" {
		maestro.Env = append(maestro.Env, "MAESTRO_DRIVER_STARTUP_TIMEOUT=600000")
	}
	err = maestro.Run()
	killProcessGroup(ctx, maestro)
	if err != nil {
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
	// The second account loses, or gets back, access to the harness's
	// Library, as an admin would change it on the web.
	mux.HandleFunc("POST /reader-access", func(w http.ResponseWriter, r *http.Request) {
		if err := setReaderAccess(r.Context(), filepath.Join(srv.dir, "data.sqlite"), r.URL.Query().Get("access") != "none"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// What the server holds of the harness user's reading or listening to
	// the Book titled ?title=, read from its database, which works offline
	// too.
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
	// The harness user's Bookmarks in the Book titled ?title=, to check what
	// the app made offline reached the server.
	mux.HandleFunc("GET /bookmarks", func(w http.ResponseWriter, r *http.Request) {
		bookmarks, err := readBookmarks(r.Context(), filepath.Join(srv.dir, "data.sqlite"), r.URL.Query().Get("title"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bookmarks)
	})
	// Answers after ?ms= milliseconds, at most 5 s: Maestro's scripts have no
	// sleep, so the assert_*.js scripts wait here between polls.
	mux.HandleFunc("GET /wait", func(w http.ResponseWriter, r *http.Request) {
		ms, err := strconv.Atoi(r.URL.Query().Get("ms"))
		if err != nil || ms < 0 || ms > 5000 {
			http.Error(w, "ms must be 0 to 5000", http.StatusBadRequest)
			return
		}
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	})
	go func() { _ = http.Serve(listener, mux) }()
	return &control{URL: "http://" + strings.TrimPrefix(listener.Addr().String(), "http://"), listener: listener}, nil
}

func (c *control) Close() {
	_ = c.listener.Close()
}
