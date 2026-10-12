package downloadcache

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

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

// errNoReaders stops a job whose cache copy was dropped once no download is
// left to send its output to.
var errNoReaders = errors.New("no download is reading the generated file")

// job is one generation running in the background, detached from the
// request that started it. It writes into a partial file in the cache and
// renames it into place once complete. Downloads read the partial file as
// it grows, through readers that share the job's file handle, so the handle
// stays valid across the rename and is closed when the last user releases
// it.
//
// When writing the partial file fails partway (a full disk), the job drops
// its cache copy rather than failing the downloads: from then on each
// reader already attached gets the rest of the output through its own pipe,
// once it has read what reached the file, and the job runs at the pace of
// the slowest reader. Nothing is published, and the job stops when the last
// reader leaves.
type job struct {
	// prepared is closed once the preparation step has finished; prepErr
	// and size are set before it is closed.
	prepared chan struct{}
	prepErr  error
	size     int64 // -1 when not known up front

	// finished is closed once the job has ended; err is set before.
	finished chan struct{}

	// gen builds the file, so a download that cannot read this job can
	// build its own copy.
	gen filegen.StreamGenerator

	f           partialFile
	partialPath string
	finalPath   string

	// outs and readersGone belong to the job's goroutine. readersGone is
	// set once the cache copy is dropped and no download is left to read
	// the output, which stops the build without it being the file's fault.
	outs        []*io.PipeWriter
	readersGone bool

	// cacheErr is why the cache copy was dropped (a write that failed
	// partway, or a failed publish), or nil. Set by the job's goroutine
	// before finished is closed.
	cacheErr error
	// onDirect is called once when the cache copy is dropped partway.
	onDirect func(err error)

	mu      sync.Mutex
	written int64 // bytes in the partial file
	done    bool
	err     error
	direct  bool // the cache copy was dropped; output goes through pipes
	readers map[*jobReader]struct{}
	changed chan struct{} // closed and replaced whenever written, done, or direct changes
	refs    int           // the writer plus every open reader
}

func newJob(f partialFile, finalPath string, gen filegen.StreamGenerator) *job {
	return &job{
		gen:         gen,
		readers:     make(map[*jobReader]struct{}),
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

// Write appends to the partial file and wakes the readers. Once a write to
// the file fails, it sends the output to the readers' pipes instead.
func (j *job) Write(p []byte) (int, error) {
	if j.cacheErr != nil {
		return j.writeDirect(p)
	}
	n, err := j.f.Write(p)
	if n > 0 {
		j.mu.Lock()
		j.written += int64(n)
		j.notifyLocked()
		j.mu.Unlock()
	}
	if err == nil {
		return n, nil
	}
	j.dropCache(errors.Wrap(err, "failed to write download cache"))
	m, err := j.writeDirect(p[n:])
	return n + m, err
}

// dropCache gives every attached reader a pipe for the output the file did
// not get, and stops new readers from attaching.
func (j *job) dropCache(err error) {
	j.cacheErr = err
	j.mu.Lock()
	j.direct = true
	for r := range j.readers {
		pr, pw := io.Pipe()
		r.pipe = pr
		// A reader blocked on its pipe still stops when its request ends.
		r.stopPipe = context.AfterFunc(r.ctx, func() { pr.CloseWithError(r.ctx.Err()) })
		j.outs = append(j.outs, pw)
	}
	j.notifyLocked()
	j.mu.Unlock()
	if j.onDirect != nil {
		j.onDirect(err)
	}
}

// writeDirect sends p to every reader still reading, waiting for each to
// take it.
func (j *job) writeDirect(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	live := j.outs[:0]
	for _, pw := range j.outs {
		if _, err := pw.Write(p); err == nil {
			live = append(live, pw)
		}
	}
	j.outs = live
	if len(live) == 0 {
		j.readersGone = true
		return 0, errNoReaders
	}
	return len(p), nil
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
	for _, pw := range j.outs {
		pw.CloseWithError(err)
	}
	j.outs = nil
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

// attachment is what reader found.
type attachment int

const (
	// attached: the reader reads the job's output.
	attached attachment = iota
	// jobEnded: the job has ended and its file handle may be closed; read
	// the published file or report j.err.
	jobEnded
	// cacheDropped: the job dropped its cache copy partway, so the start of
	// the output is gone; build another copy.
	cacheDropped
)

// reader returns a reader of the job's output that waits for more while the
// job runs, when the state is attached.
func (j *job) reader(ctx context.Context) (io.ReadCloser, attachment) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return nil, jobEnded
	}
	if j.direct {
		return nil, cacheDropped
	}
	j.refs++
	r := &jobReader{j: j, ctx: ctx}
	j.readers[r] = struct{}{}
	return r, attached
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
	// pipe carries the output the partial file did not get, once the job
	// drops its cache copy. Set under j.mu.
	pipe     *io.PipeReader
	stopPipe func() bool
}

func (r *jobReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		j := r.j
		j.mu.Lock()
		written, done, jobErr, changed, pipe := j.written, j.done, j.err, j.changed, r.pipe
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
		if pipe != nil {
			n, err := pipe.Read(p)
			r.off += int64(n)
			return n, err
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
		delete(r.j.readers, r)
		if r.pipe != nil {
			r.stopPipe()
			r.pipe.Close()
		}
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
	built, err := a.build(ctx, a.dir, book, file)
	if err != nil && !errors.Is(err, syscall.ENOSPC) && !errors.Is(err, fs.ErrPermission) {
		return nil, err
	}
	if err != nil {
		// A cache that cannot be written still lets the download stream,
		// so build in the system's temporary directory instead.
		if built, err = a.build(ctx, "", book, file); err != nil {
			return nil, err
		}
	}
	return built, nil
}

// build builds the whole file into a new scratch directory in dir.
func (a fullBuild) build(ctx context.Context, dir string, book *models.Book, file *models.File) (*builtFile, error) {
	stagingDir, err := os.MkdirTemp(dir, stagingPrefix)
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

// partialFile is the file a job writes into.
type partialFile interface {
	io.Writer
	io.ReaderAt
	io.Closer
	Name() string
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
