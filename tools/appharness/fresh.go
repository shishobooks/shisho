package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// The server's database files, which a fresh database replaces and the
// seeded one's come back over.
var databaseFiles = []string{"data.sqlite", "data.sqlite-wal", "data.sqlite-shm"}

// freshDatabase restarts the server on an empty database at the same
// address, as a server set up again from scratch, with the harness account
// made again as its first admin, so it has the same user id. The seeded
// database is set aside for seededDatabase to bring back. It returns where
// the fresh server's log starts in server.log.
func (s *server) freshDatabase(ctx context.Context) (int64, error) {
	s.swap.Lock()
	defer s.swap.Unlock()
	s.Stop()
	aside := filepath.Join(s.dir, "seeded-database")
	if _, err := os.Stat(aside); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(aside, 0o755); err != nil {
			return 0, errors.WithStack(err)
		}
		if err := moveDatabase(s.dir, aside); err != nil {
			return 0, err
		}
	} else if err := removeDatabase(s.dir); err != nil {
		return 0, err
	}
	var logStart int64
	if info, err := os.Stat(filepath.Join(s.dir, "server.log")); err == nil {
		logStart = info.Size()
	}
	if err := s.Start(ctx); err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if err := s.post(ctx, client, "/api/auth/setup", map[string]string{"username": harnessUsername, "password": harnessPassword}); err != nil {
		return 0, errors.Wrap(err, "set up the fresh server")
	}
	if err := cheapenPasswords(ctx, filepath.Join(s.dir, "data.sqlite"), map[string]string{harnessUsername: harnessPassword}); err != nil {
		return 0, err
	}
	return logStart, nil
}

// seededDatabase brings back the seeded database a fresh one replaced, if
// one did, and leaves the server running on it.
func (s *server) seededDatabase(ctx context.Context) error {
	s.swap.Lock()
	defer s.swap.Unlock()
	aside := filepath.Join(s.dir, "seeded-database")
	if _, err := os.Stat(aside); err == nil {
		s.Stop()
		if err := removeDatabase(s.dir); err != nil {
			return err
		}
		if err := moveDatabase(aside, s.dir); err != nil {
			return err
		}
		if err := os.Remove(aside); err != nil {
			return errors.WithStack(err)
		}
	}
	return s.Start(ctx)
}

func moveDatabase(from, to string) error {
	for _, name := range databaseFiles {
		err := os.Rename(filepath.Join(from, name), filepath.Join(to, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func removeDatabase(dir string) error {
	for _, name := range databaseFiles {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.WithStack(err)
		}
	}
	return nil
}

// journalRequestCount is the control endpoint's answer to how many journal
// requests reached the fresh server.
type journalRequestCount struct {
	Count int `json:"count"`
}

// journalRequests counts the journal requests in the server's log from
// offset on, which is where a fresh server's log starts.
func journalRequests(logPath string, offset int64) (int, error) {
	file, err := os.Open(logPath) //nolint:gosec // The run's own server log.
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, errors.WithStack(err)
	}
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "/mobile/journal") {
			count++
		}
	}
	return count, errors.WithStack(scanner.Err())
}
