package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

// stagingPrefix names the per-generation directories that hold output until it
// is complete. Cache listings skip directories, so they are never mistaken for
// entries.
const stagingPrefix = ".staging-"

// keyedLock serializes work per key while letting unrelated keys proceed. A
// waiter gives up when its context ends, so a canceled request never blocks
// behind a slow generation.
type keyedLock struct {
	mu    sync.Mutex
	slots map[string]*lockSlot
}

type lockSlot struct {
	ch   chan struct{}
	refs int
}

// acquire blocks until key is free or ctx ends. The returned release must be
// called exactly once.
func (l *keyedLock) acquire(ctx context.Context, key string) (release func(), err error) {
	l.mu.Lock()
	if l.slots == nil {
		l.slots = make(map[string]*lockSlot)
	}
	slot, ok := l.slots[key]
	if !ok {
		slot = &lockSlot{ch: make(chan struct{}, 1)}
		l.slots[key] = slot
	}
	slot.refs++
	l.mu.Unlock()

	drop := func() {
		l.mu.Lock()
		slot.refs--
		if slot.refs == 0 {
			delete(l.slots, key)
		}
		l.mu.Unlock()
	}

	select {
	case slot.ch <- struct{}{}:
		return func() {
			<-slot.ch
			drop()
		}, nil
	case <-ctx.Done():
		drop()
		return nil, errors.WithStack(ctx.Err())
	}
}

// generateInto runs generate against a path inside a fresh staging directory
// and, once it succeeds, renames the result onto destPath. Readers of destPath
// therefore see either the previous complete file or the new complete file,
// never a partial one, and concurrent generators cannot clobber each other's
// scratch files. It returns the size of the published file.
func generateInto(dir, destPath string, generate func(stagedPath string) error) (int64, error) {
	stagingDir, err := os.MkdirTemp(dir, stagingPrefix)
	if err != nil {
		return 0, errors.Wrap(err, "failed to create staging directory")
	}
	defer os.RemoveAll(stagingDir)

	stagedPath := filepath.Join(stagingDir, filepath.Base(destPath))
	if err := generate(stagedPath); err != nil {
		return 0, err
	}

	info, err := os.Stat(stagedPath)
	if err != nil {
		return 0, errors.Wrap(err, "failed to stat generated file")
	}

	if err := os.Rename(stagedPath, destPath); err != nil {
		return 0, errors.Wrap(err, "failed to publish generated file")
	}

	return info.Size(), nil
}
