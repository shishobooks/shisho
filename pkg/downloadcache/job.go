package downloadcache

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
)

// partialPrefix names the files generation writes into before they are
// complete, and stagingPrefix the scratch directories of generators that
// build a whole file before streaming it. Neither matches a cache-hit check,
// so a partial file is never served as a hit, and RemoveLeftovers deletes
// both at startup.
const (
	partialPrefix = ".partial-"
	stagingPrefix = ".staging-"
)

// job is one generation running in the background, detached from the
// request that started it. It writes into a partial file in the cache and
// renames it into place once complete. Downloads read the partial file as
// it grows, through readers that share the job's file handle, so the handle
// stays valid across the rename and is closed when the last user releases
// it.
type job struct {
	// prepared is closed once the preparation step has finished; prepErr
	// and size are set before it is closed.
	prepared chan struct{}
	prepErr  error
	size     int64 // -1 when not known up front

	// finished is closed once the job has ended; err is set before.
	finished chan struct{}

	f           *os.File
	partialPath string
	finalPath   string

	mu      sync.Mutex
	written int64
	done    bool
	err     error
	changed chan struct{} // closed and replaced whenever written or done changes
	refs    int           // the writer plus every open reader
}

func newJob(f *os.File, finalPath string) *job {
	return &job{
		prepared:    make(chan struct{}),
		finished:    make(chan struct{}),
		f:           f,
		partialPath: f.Name(),
		finalPath:   finalPath,
		size:        -1,
		changed:     make(chan struct{}),
		refs:        1,
	}
}

// Write appends to the partial file and wakes the readers.
func (j *job) Write(p []byte) (int, error) {
	n, err := j.f.Write(p)
	if n > 0 {
		j.mu.Lock()
		j.written += int64(n)
		j.notifyLocked()
		j.mu.Unlock()
	}
	return n, err
}

func (j *job) notifyLocked() {
	close(j.changed)
	j.changed = make(chan struct{})
}

// finish ends the job and drops the writer's hold on the file.
func (j *job) finish(err error) {
	j.mu.Lock()
	j.done = true
	j.err = err
	j.notifyLocked()
	j.releaseLocked()
	j.mu.Unlock()
	close(j.finished)
}

func (j *job) releaseLocked() {
	j.refs--
	if j.refs == 0 {
		j.f.Close()
	}
}

// adopt makes a file built in full the job's output: it replaces the empty
// partial file, and is complete before any reader attaches.
func (j *job) adopt(built *builtFile) error {
	defer os.RemoveAll(built.dir)
	if err := os.Rename(built.path, j.partialPath); err != nil {
		return errors.Wrap(err, "failed to stage generated file")
	}
	f, err := os.OpenFile(j.partialPath, os.O_RDWR, 0)
	if err != nil {
		return errors.WithStack(err)
	}
	j.f.Close()
	j.f = f
	j.written = built.size
	j.size = built.size
	return nil
}

// reader returns a reader of the job's output that waits for more while the
// job runs. ok is false when the job has already ended and its file handle
// may be closed; the caller then reads the published file or reports
// j.err.
func (j *job) reader(ctx context.Context) (r io.ReadCloser, ok bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return nil, false
	}
	j.refs++
	return &jobReader{j: j, ctx: ctx}, true
}

// jobReader reads a job's output from the start. Read blocks while it has
// caught up with the writer, and ends with io.EOF once the job succeeds or
// with the job's error once it fails. ctx is the request's, so a client
// that leaves stops waiting without affecting the job.
type jobReader struct {
	j      *job
	ctx    context.Context
	off    int64
	closed bool
}

func (r *jobReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		j := r.j
		j.mu.Lock()
		written, done, jobErr, changed := j.written, j.done, j.err, j.changed
		j.mu.Unlock()

		if r.off < written {
			n := int64(len(p))
			if avail := written - r.off; n > avail {
				n = avail
			}
			read, err := j.f.ReadAt(p[:n], r.off)
			r.off += int64(read)
			if errors.Is(err, io.EOF) && read > 0 {
				err = nil
			}
			return read, err
		}
		if done {
			if jobErr != nil {
				return 0, jobErr
			}
			return 0, io.EOF
		}
		select {
		case <-changed:
		case <-r.ctx.Done():
			return 0, errors.WithStack(r.ctx.Err())
		}
	}
}

func (r *jobReader) Close() error {
	r.j.mu.Lock()
	defer r.j.mu.Unlock()
	if !r.closed {
		r.closed = true
		r.j.releaseLocked()
	}
	return nil
}

// pathGenerator writes a whole file to a path: the built-in generators that
// do not stream yet.
type pathGenerator interface {
	Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error
}

// fullBuild adapts a pathGenerator to filegen.StreamGenerator. Its
// preparation step builds the whole file into a scratch directory in the
// cache, so a failure still comes before the first byte and the size is
// known; only the copy into the download streams. Because generation runs
// detached from the request, a client that disconnects still leaves a
// cached file behind.
type fullBuild struct {
	gen pathGenerator
	dir string
	// name is the staged file's name. Some generators read the output's
	// extension, so it is the cached file's own name.
	name string
	// what describes the generation in errors.
	what string
}

func (a fullBuild) Prepare(ctx context.Context, _ filegen.Source, book *models.Book, file *models.File) (filegen.Prepared, error) {
	stagingDir, err := os.MkdirTemp(a.dir, stagingPrefix)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create staging directory")
	}
	stagedPath := filepath.Join(stagingDir, a.name)
	if err := a.gen.Generate(ctx, file.Filepath, stagedPath, book, file); err != nil {
		os.RemoveAll(stagingDir)
		return nil, errors.Wrap(err, a.what)
	}
	info, err := os.Stat(stagedPath)
	if err != nil {
		os.RemoveAll(stagingDir)
		return nil, errors.Wrap(err, "failed to stat generated file")
	}
	return &builtFile{dir: stagingDir, path: stagedPath, size: info.Size()}, nil
}

// builtFile is a fullBuild's complete output, waiting to be copied.
type builtFile struct {
	dir  string
	path string
	size int64
}

func (b *builtFile) Size() (int64, bool) {
	return b.size, true
}

func (b *builtFile) Write(ctx context.Context, w io.Writer) error {
	defer os.RemoveAll(b.dir)
	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}
	f, err := os.Open(b.path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return errors.WithStack(err)
}

// createPartial creates the file a job writes into. Downloads are read by
// other host accounts, so it is world-readable from the start.
func createPartial(dir string) (*os.File, error) {
	f, err := fileutils.CreateTemp(dir, partialPrefix+"*", 0o644)
	return f, errors.Wrap(err, "failed to create partial file")
}

// RemoveLeftovers deletes the partial files and scratch directories a crash
// or a killed process left in the cache. Call it at startup, before any
// download runs.
func (c *Cache) RemoveLeftovers() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "failed to read cache directory")
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, partialPrefix) && !strings.HasPrefix(name, stagingPrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.dir, name)); err != nil {
			return errors.Wrapf(err, "failed to remove %s", name)
		}
	}
	return nil
}
