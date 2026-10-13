// Package lrufiles keeps a directory of cached files within a byte budget by
// removing the least recently used files first.
package lrufiles

import (
	"container/list"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Unlimited is the budget of a cache that never evicts.
const Unlimited int64 = math.MaxInt64

// recentWindow protects files used this recently from eviction. A cache hands
// its caller a path, and the caller opens the file only after the cache
// returns, so evicting a file that was just handed out would fail the request
// that asked for it. The cache can run over its budget by what was used in
// this window until the next eviction.
const recentWindow = time.Minute

// touchInterval bounds how often a hit rewrites a file's mtime, which carries
// recency across restarts, so a reader paging back and forth does not write
// metadata on every page.
const touchInterval = time.Minute

type entry struct {
	path    string
	bytes   int64
	used    time.Time
	stamped time.Time
	element *list.Element
}

// Index tracks the files under one cache directory in least recently used
// order. It loads by walking the directory when a budget is first set or
// used, so an unlimited cache never pays for the walk. All methods are safe
// for concurrent use.
type Index struct {
	root     string
	now      func() time.Time
	mu       sync.Mutex
	maxBytes int64
	loaded   bool
	entries  map[string]*entry
	lru      list.List
	total    int64
}

// New returns an unlimited index of the files under root.
func New(root string) *Index {
	return &Index{root: root, now: time.Now, maxBytes: Unlimited, entries: make(map[string]*entry)}
}

// SetMaxBytes changes the budget and evicts down to it now.
func (x *Index) SetMaxBytes(maxBytes int64) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.maxBytes = max(0, maxBytes)
	if !x.loaded && x.maxBytes == Unlimited {
		return nil
	}
	if err := x.loadLocked(); err != nil {
		return err
	}
	return x.evictLocked()
}

// Used records a cache hit on path and reports whether the file is still
// there. A concurrent publish can evict it between the caller's existence
// check and this call; on false the caller regenerates it. It is best effort:
// a failure to index the directory never fails the hit.
func (x *Index) Used(path string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	now := x.now()
	// Load before the lookup: a walk this call triggers may index the path,
	// and adding it again would count it twice. An unlimited cache tracks
	// nothing until a limit is set, so the default never walks a large cache,
	// but the mtime still carries recency for when it is.
	tracking := x.tracking()
	e, ok := x.entries[path]
	if !ok {
		info, err := os.Stat(path)
		if err != nil {
			return false
		}
		e = &entry{path: path, bytes: info.Size(), stamped: info.ModTime()}
		if tracking {
			x.addLocked(e)
		}
	} else {
		x.lru.MoveToBack(e.element)
	}
	e.used = now
	if now.Sub(e.stamped) >= touchInterval {
		if err := os.Chtimes(path, now, now); err == nil {
			e.stamped = now
		}
	}
	return true
}

// Added records a file the cache just published at path and evicts down to
// the budget. Like Used it is best effort, so the request that produced the
// file is still served when eviction fails.
func (x *Index) Added(path string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.tracking() {
		return
	}
	// Stat rather than trust the caller: a Clear or Invalidate between the
	// publish and this call already removed the file.
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if e, ok := x.entries[path]; ok {
		// A concurrent request published the same file again.
		x.forgetLocked(e)
	}
	now := x.now()
	x.addLocked(&entry{path: path, bytes: info.Size(), used: now, stamped: now})
	_ = x.evictLocked()
}

// Forget drops every entry under dir after the caller removed it from disk.
// A file published into dir after the removal is indexed again, so a page an
// in-flight extraction wrote back still counts toward the budget.
func (x *Index) Forget(dir string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.loaded {
		return
	}
	dir = filepath.Clean(dir)
	prefix := dir + string(filepath.Separator)
	for path, e := range x.entries {
		if strings.HasPrefix(path, prefix) {
			x.forgetLocked(e)
		}
	}
	for _, e := range walk(dir, x.now()) {
		x.addLocked(e)
	}
}

// Reset empties the index after the caller cleared the whole directory. The
// next use walks it again, which is cheap right after a clear and picks up
// any page published while the clear ran.
func (x *Index) Reset() {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.entries = make(map[string]*entry)
	x.lru.Init()
	x.total = 0
	x.loaded = false
}

// tracking loads the index when the cache has a budget and reports whether
// it is in use. It holds x.mu.
func (x *Index) tracking() bool {
	if x.maxBytes == Unlimited && !x.loaded {
		return false
	}
	return x.loadLocked() == nil
}

func (x *Index) loadLocked() error {
	if x.loaded {
		return nil
	}
	if _, err := os.Stat(x.root); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to index %s", x.root)
	}
	found := walk(x.root, x.now())
	sort.Slice(found, func(i, j int) bool { return found[i].used.Before(found[j].used) })
	x.entries = make(map[string]*entry, len(found))
	x.lru.Init()
	x.total = 0
	for _, e := range found {
		x.addLocked(e)
	}
	x.loaded = true
	return nil
}

// walk lists the published files under dir. It skips what it cannot read
// instead of failing, so one unreadable directory does not leave the rest of
// the cache unbounded or rewalked on every request. A modification time in
// the future, from a clock set back, counts as now, or it would hold off
// eviction for the whole gap.
func walk(dir string, now time.Time) []*entry {
	var found []*entry
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || isTemp(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		used := info.ModTime()
		if used.After(now) {
			used = now
		}
		found = append(found, &entry{path: path, bytes: info.Size(), used: used, stamped: info.ModTime()})
		return nil
	})
	return found
}

// isTemp reports a file still being written: a page cache's ".extracting-"
// file or a fileutils.WriteFileAtomic temp file.
func isTemp(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp")
}

func (x *Index) addLocked(e *entry) {
	e.element = x.lru.PushBack(e)
	x.entries[e.path] = e
	x.total += e.bytes
}

func (x *Index) forgetLocked(e *entry) {
	x.lru.Remove(e.element)
	delete(x.entries, e.path)
	x.total -= e.bytes
}

func (x *Index) evictLocked() error {
	now := x.now()
	for x.total > x.maxBytes && x.lru.Len() > 0 {
		e := x.lru.Front().Value.(*entry)
		if now.Sub(e.used) < recentWindow {
			// Everything behind it was used even more recently.
			return nil
		}
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			return errors.WithStack(err)
		}
		x.forgetLocked(e)
	}
	return nil
}
