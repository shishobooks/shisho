package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// The account the flows sign in with.
const (
	harnessUsername = "harness"
	harnessPassword = "harness-password"
)

// server is a Shisho server of the run's own, on a free port, with its
// database and caches in the run's folder. A flow can stop it and start it
// again on the same port and data, as a server coming back online.
type server struct {
	URL    string
	port   string
	binary string
	dir    string
	env    []string

	mu  sync.Mutex
	cmd *exec.Cmd
	log *os.File
}

// startServer builds the server from this checkout and starts it on dir's
// files, listening on host and port, or a free port when port is 0. env adds
// to the server's environment. The server's output goes to dir/server.log.
func startServer(ctx context.Context, root, dir, host string, port int, env ...string) (*server, error) {
	binary := filepath.Join(dir, "shisho")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/api")
	build.Dir = root
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return nil, errors.Wrap(err, "build the server")
	}
	if port == 0 {
		var err error
		if port, err = freePort(ctx); err != nil {
			return nil, err
		}
	}
	s := &server{URL: fmt.Sprintf("http://127.0.0.1:%d", port), port: strconv.Itoa(port), binary: binary, dir: dir}
	s.env = append([]string{
		"DATABASE_FILE_PATH=" + filepath.Join(dir, "data.sqlite"),
		"CACHE_DIR=" + filepath.Join(dir, "cache"),
		"PLUGIN_DIR=" + filepath.Join(dir, "plugins"),
		"PLUGIN_DATA_DIR=" + filepath.Join(dir, "plugin-data"),
		"JWT_SECRET=appharness-secret-not-for-production",
		"SERVER_HOST=" + host,
		"SERVER_PORT=" + strconv.Itoa(port),
		"LIBRARY_MONITOR_ENABLED=false",
		"UPDATE_CHECK_ENABLED=false",
		"SYNC_INTERVAL_MINUTES=0",
	}, env...)
	if err := s.Start(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Start runs the server, if it isn't running, and waits until it answers.
// Its output goes to the end of dir/server.log.
func (s *server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running() {
		return nil
	}
	log, err := os.OpenFile(filepath.Join(s.dir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.WithStack(err)
	}
	cmd := exec.Command(s.binary) //nolint:noctx,gosec // The server this run built; Stop ends it, with its process group.
	cmd.Dir = s.dir
	cmd.Env = append(os.Environ(), s.env...)
	cmd.Stdout, cmd.Stderr = log, log
	// Its own process group, so stopping the run stops it and nothing else.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return errors.Wrap(err, "start the server")
	}
	s.cmd, s.log = cmd, log
	if err := s.waitHealthy(ctx); err != nil {
		s.stopLocked()
		return err
	}
	return nil
}

func (s *server) running() bool {
	return s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil
}

func freePort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func (s *server) waitHealthy(ctx context.Context) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/health", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return errors.Errorf("the server at %s didn't start; see server.log", s.URL)
}

// Stop ends the server, if it's still running.
func (s *server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *server) stopLocked() {
	if s.running() {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = s.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}
	if s.log != nil {
		_ = s.log.Close()
	}
}

// seed makes the first admin, a Library on the fixtures, and waits for the
// scan to find every fixture.
func (s *server) seed(ctx context.Context, library, password string) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	post := func(path string, body any) error {
		data, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return errors.WithStack(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			text, _ := io.ReadAll(resp.Body)
			return errors.Errorf("POST %s: %s: %s", path, resp.Status, text)
		}
		return nil
	}
	if err := post("/api/auth/setup", map[string]string{"username": harnessUsername, "password": password}); err != nil {
		return err
	}
	if err := post("/api/auth/login", map[string]string{"username": harnessUsername, "password": password}); err != nil {
		return err
	}
	if err := post("/api/libraries", map[string]any{
		"name": "Harness Library", "cover_aspect_ratio": "book", "library_paths": []string{library},
		"organize_file_structure": false,
	}); err != nil {
		return err
	}

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/api/books?limit=50", nil)
		resp, err := client.Do(req)
		if err != nil {
			return errors.WithStack(err)
		}
		var page struct {
			Total int `json:"total"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err == nil && page.Total >= len(fixtures) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.Errorf("the scan didn't find all %d fixtures in time; see server.log", len(fixtures))
}
