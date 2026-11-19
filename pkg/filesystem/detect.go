package filesystem

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
)

// detectLimits bounds a detection walk, so Setup never waits long on a huge
// library or a slow disk.
type detectLimits struct {
	// Duration is how long the request may take.
	Duration time.Duration
	// Files is how many recognized files the walk counts before it stops.
	Files int
	// foldersOnly resolves the paths and walks none of them.
	foldersOnly bool
	// now reads the clock and walk walks one root; tests replace them.
	now  func() time.Time
	walk func(root string, fn fs.WalkDirFunc) error
}

// defaultDetectLimits is what POST /filesystem/detect walks within.
var defaultDetectLimits = detectLimits{Duration: 5 * time.Second, Files: 5000}

// errBudgetSpent ends a walk once its budget runs out.
var errBudgetSpent = errors.New("detection budget spent")

// detectCounts is the walk's running tally, read by the request when the
// deadline passes before the walk returns.
type detectCounts struct {
	mu   sync.Mutex
	resp DetectFileTypesResponse
}

func (c *detectCounts) snapshot(stoppedEarly bool) *DetectFileTypesResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp := c.resp
	resp.StoppedEarly = resp.StoppedEarly || stoppedEarly
	resp.MissingPaths = slices.Clone(resp.MissingPaths)
	return &resp
}

// DetectFileTypes walks paths and counts the files the scanner would
// recognize by their extension, by category. It opens no files. Each path is
// resolved as the directory picker resolves it, symlinks included. A path
// that isn't a folder the server can open and read is named in MissingPaths,
// since a library made from it never scans. A folder inside one that can't
// be read is skipped, and the scan reports it itself. With foldersOnly it
// stops after checking the paths.
//
// The walk checks its budget between entries, but one folder's read can
// take any time on a hung network mount, so the walk runs on its own and the
// request returns what it has counted once the time is up. The walk then
// stops at its next entry.
func (s *Service) DetectFileTypes(ctx context.Context, paths []string, limits detectLimits) (*DetectFileTypesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	walkCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	counts := &detectCounts{}
	done := make(chan error, 1)
	go func() { done <- detectWalk(walkCtx, paths, limits, counts) }()

	timer := time.NewTimer(limits.Duration)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
		return counts.snapshot(false), nil
	case <-timer.C:
		return counts.snapshot(true), nil
	case <-ctx.Done():
		return nil, errors.WithStack(ctx.Err())
	}
}

func detectWalk(ctx context.Context, paths []string, limits detectLimits, counts *detectCounts) error {
	now := limits.now
	if now == nil {
		now = time.Now
	}
	walk := limits.walk
	if walk == nil {
		walk = fileutils.WalkDirFollowingRoot
	}
	deadline := now().Add(limits.Duration)
	counted := 0
	stop := func() error {
		counts.mu.Lock()
		counts.resp.StoppedEarly = true
		counts.mu.Unlock()
		return errBudgetSpent
	}

	var roots []string
	for _, path := range paths {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		root, ok := resolveDir(path)
		if !ok {
			counts.mu.Lock()
			counts.resp.MissingPaths = append(counts.resp.MissingPaths, path)
			counts.mu.Unlock()
			continue
		}
		roots = append(roots, root)
	}
	if limits.foldersOnly {
		return nil
	}

	for i, root := range roots {
		// A path inside another is walked with it.
		if coveredByAnother(roots, i) {
			continue
		}
		err := walk(root, func(path string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !now().Before(deadline) {
				return stop()
			}
			if walkErr != nil {
				// An unreadable folder is skipped; the rest is still counted.
				if d != nil && d.IsDir() && path != root {
					return fs.SkipDir
				}
				return nil
			}
			// macOS leaves "._" files beside others on network shares, which
			// the scan rejects.
			if d.IsDir() || strings.HasPrefix(d.Name(), "._") {
				return nil
			}
			fileType := models.FileTypeForPath(path)
			if !models.IsBuiltInFileType(fileType) {
				return nil
			}
			if counted >= limits.Files {
				return stop()
			}
			counted++
			counts.mu.Lock()
			defer counts.mu.Unlock()
			switch {
			case fileType == models.FileTypeCBZ:
				counts.resp.Comics++
			case models.IsAudiobookFileType(fileType):
				counts.resp.Audiobooks++
			default:
				counts.resp.Ebooks++
			}
			return nil
		})
		if errors.Is(err, errBudgetSpent) {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.WithStack(ctxErr)
		}
		// A root that turns unreadable mid-walk keeps what was found.
	}
	return nil
}

// resolveDir resolves a path the way Browse does and reports whether it is a
// directory the server can read. Reading one entry tells a folder the server
// may list from one it can only see, such as one owned by another user.
func resolveDir(path string) (string, bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.IsDir() {
		return "", false
	}
	dir, err := os.Open(realPath)
	if err != nil {
		return "", false
	}
	defer dir.Close()
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return "", false
	}
	return realPath, true
}

// coveredByAnother reports whether roots[i] is walked as part of another
// root: it lies inside one, or repeats an earlier one.
func coveredByAnother(roots []string, i int) bool {
	for j, other := range roots {
		if j == i {
			continue
		}
		if roots[i] == other {
			if j < i {
				return true
			}
			continue
		}
		if fileutils.IsWithin(roots[i], other) {
			return true
		}
	}
	return false
}
