package downloadcache

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/version"
)

// Cache manages the download cache for generated files.
//
// A Generated File is built by a job running in the background, detached
// from the request that asked for it, which writes into a partial file in
// the cache (ADR 0010). Every download of the file while the job runs, the
// first one included, reads that partial file as it grows. A client that
// disconnects leaves the job running, so the file still lands in the cache.
type Cache struct {
	dir               string
	maxSize           int64
	ShouldSkipCleanup func() bool

	// generators overrides the built-in generator lookup (WithGenerators).
	generators GeneratorLookup
	// createPartial creates the file a job writes into. Tests replace it to
	// make cache writes fail.
	createPartial func(dir string) (partialFile, error)

	// ctx ends when Close is called, canceling every job.
	ctx    context.Context
	cancel context.CancelFunc

	// background tracks cleanups and running jobs so Wait can drain them.
	background sync.WaitGroup

	// mu guards jobs, the running job for each cache entry and fingerprint,
	// and closed, which stops new jobs once Close has begun.
	mu     sync.Mutex
	jobs   map[string]*job
	closed bool

	// publishMu serializes publishing. Fingerprints of one file share its
	// cached path, so a rename and its metadata write must not interleave
	// with another job's.
	publishMu sync.Mutex
}

// GeneratorLookup returns the generator for a file type, or its KePub
// generator when kepub is set.
type GeneratorLookup func(fileType string, kepub bool) (filegen.StreamGenerator, error)

// Option configures a Cache.
type Option func(*Cache)

// WithGenerators replaces the built-in generators, so tests can stand in a
// generator they control.
func WithGenerators(lookup GeneratorLookup) Option {
	return func(c *Cache) { c.generators = lookup }
}

// NewCache creates a new Cache with the given directory and max size.
func NewCache(dir string, maxSizeBytes int64, opts ...Option) *Cache {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{
		dir:     dir,
		maxSize: maxSizeBytes,
		ctx:     ctx,
		cancel:  cancel,
		jobs:    make(map[string]*job),
	}
	c.createPartial = func(dir string) (partialFile, error) { return createPartial(dir) }
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Generated is a Generated File ready to send. Either Path names the
// complete file in the cache, or the file is still being built and Body
// streams it.
type Generated struct {
	// Filename is the name to save the download under.
	Filename string
	// Path is the complete file in the cache, or "" while it is being built.
	Path string
	// Size is the exact size of a file still being built, or -1 when it is
	// not known until the file is complete.
	Size int64
	// Body reads a file still being built as it grows. It is nil for a
	// cached file and for Head. The caller closes it.
	Body io.ReadCloser
}

// Open returns the cached Generated File for the book's file, or starts
// building it (or joins the build already running) and streams it. An error
// from the preparation step is returned before anything is sent. The build
// keeps running if ctx ends.
func (c *Cache) Open(ctx context.Context, book *models.Book, file *models.File, kepub bool) (*Generated, error) {
	e, err := c.entryFor(book, file, kepub)
	if err != nil {
		return nil, err
	}
	return c.open(ctx, e, false)
}

// Head answers a HEAD request for the Generated File: the cached file when
// there is one, and otherwise the result of the preparation step alone,
// which reports any error and the size when it is known. Nothing is
// generated, except by generators that build a whole file before streaming
// it: their preparation step is the build, so Head starts it as Open would
// and the build lands in the cache.
func (c *Cache) Head(ctx context.Context, book *models.Book, file *models.File, kepub bool) (*Generated, error) {
	e, err := c.entryFor(book, file, kepub)
	if err != nil {
		return nil, err
	}
	return c.open(ctx, e, true)
}

// GetOrGenerate returns the path to a cached file, generating it if necessary
// and waiting until it is complete. It returns the cached file path, the
// formatted download filename, and any error.
func (c *Cache) GetOrGenerate(ctx context.Context, book *models.Book, file *models.File) (cachedPath string, downloadFilename string, err error) {
	e, err := c.entryFor(book, file, false)
	if err != nil {
		return "", "", err
	}
	path, err := c.complete(ctx, e)
	return path, e.filename, err
}

// GetOrGenerateKepub returns the path to a cached KePub file, generating it
// if necessary and waiting until it is complete. It returns the cached file
// path, the formatted download filename, and any error. Returns
// ErrKepubNotSupported if the file type cannot be converted to KePub.
func (c *Cache) GetOrGenerateKepub(ctx context.Context, book *models.Book, file *models.File) (cachedPath string, downloadFilename string, err error) {
	e, err := c.entryFor(book, file, true)
	if err != nil {
		return "", "", err
	}
	path, err := c.complete(ctx, e)
	return path, e.filename, err
}

// entry describes one cache entry: a file in one output format, at one
// fingerprint.
type entry struct {
	book *models.Book
	file *models.File
	hash string
	// destPath is where the complete file is published. Fingerprints of one
	// file share it.
	destPath string
	filename string
	// lookup returns the published path if it matches the current
	// fingerprint, or "" on a miss.
	lookup func() (string, error)
	// touch records a cache hit. Failures are ignored.
	touch func()
	// readMeta returns the entry's metadata, or nil when there is none.
	readMeta func() (*CacheMetadata, error)
	// writeMeta publishes metadata for a freshly generated file, or records
	// a failure.
	writeMeta func(meta *CacheMetadata) error
	// meta carries the identifying fields; timestamps and size are filled in.
	meta CacheMetadata
	// generator returns the generator that builds the file.
	generator func() (filegen.StreamGenerator, error)
}

func (e *entry) key() string {
	return e.destPath + "\x00" + e.hash
}

// entryFor describes the cache entry for the book's file, or its KePub
// conversion when kepub is set.
func (c *Cache) entryFor(book *models.Book, file *models.File, kepub bool) (*entry, error) {
	if kepub && !filegen.SupportsKepub(file.FileType) {
		return nil, filegen.ErrKepubNotSupported
	}

	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute fingerprint")
	}
	if kepub {
		fp.Format = FormatKepub
	}
	hash, err := fp.Hash()
	if err != nil {
		return nil, errors.Wrap(err, "failed to hash fingerprint")
	}

	e := &entry{book: book, file: file, hash: hash}
	e.generator = func() (filegen.StreamGenerator, error) { return c.generatorFor(e, kepub) }
	if kepub {
		e.destPath = kepubCachedFilename(c.dir, file.ID)
		e.filename = FormatKepubDownloadFilename(book, file)
		e.lookup = func() (string, error) { return GetKepubCachedFilePath(c.dir, file.ID, hash) }
		e.touch = func() { _ = UpdateKepubLastAccessed(c.dir, file.ID) }
		e.readMeta = func() (*CacheMetadata, error) { return ReadKepubMetadata(c.dir, file.ID) }
		e.writeMeta = func(meta *CacheMetadata) error {
			return errors.Wrap(WriteKepubMetadata(c.dir, meta), "failed to write kepub cache metadata")
		}
		e.meta = CacheMetadata{FileID: file.ID, Format: FormatKepub, FingerprintHash: hash}
		return e, nil
	}
	e.destPath = cachedFilename(c.dir, file.ID, file.FileType)
	e.filename = FormatDownloadFilename(book, file)
	e.lookup = func() (string, error) { return GetCachedFilePath(c.dir, file.ID, file.FileType, hash) }
	e.touch = func() { _ = UpdateLastAccessed(c.dir, file.ID) }
	e.readMeta = func() (*CacheMetadata, error) { return ReadMetadata(c.dir, file.ID) }
	e.writeMeta = func(meta *CacheMetadata) error {
		return errors.Wrap(WriteMetadata(c.dir, meta), "failed to write cache metadata")
	}
	e.meta = CacheMetadata{FileID: file.ID, FingerprintHash: hash}
	return e, nil
}

// generatorFor returns the generator for e's file. A built-in generator
// that does not stream yet is adapted with fullBuild.
func (c *Cache) generatorFor(e *entry, kepub bool) (filegen.StreamGenerator, error) {
	if c.generators != nil {
		return c.generators(e.file.FileType, kepub)
	}
	var gen filegen.Generator
	var err error
	what := "failed to generate file"
	if kepub {
		gen, err = filegen.GetKepubGenerator(e.file.FileType)
		what = "failed to generate kepub file"
	} else {
		gen, err = filegen.GetGenerator(e.file.FileType)
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get generator")
	}
	if sg, ok := gen.(filegen.StreamGenerator); ok {
		return sg, nil
	}
	return fullBuild{gen: gen, dir: c.dir, name: filepath.Base(e.destPath), what: what}, nil
}

// open serves Open and Head.
func (c *Cache) open(ctx context.Context, e *entry, head bool) (*Generated, error) {
	hit, err := c.lookup(e)
	if err != nil {
		return nil, err
	}
	if hit != "" {
		return &Generated{Filename: e.filename, Path: hit}, nil
	}
	if err := c.rememberedFailure(e); err != nil {
		return nil, err
	}

	gen, err := e.generator()
	if err != nil {
		return nil, err
	}
	if _, builds := gen.(fullBuild); head && !builds {
		return c.prepareOnly(ctx, e, gen)
	}

	j, hit, err := c.join(ctx, e, gen)
	var unwritable *unwritableError
	if errors.As(err, &unwritable) {
		logger.FromContext(ctx).Warn("download cache cannot be written, streaming without caching", c.logData(e, err))
		if head {
			return c.prepareOnly(ctx, e, gen)
		}
		return c.stream(ctx, e, gen)
	}
	if err != nil {
		return nil, err
	}
	if hit != "" {
		return &Generated{Filename: e.filename, Path: hit}, nil
	}
	select {
	case <-j.prepared:
	case <-ctx.Done():
		return nil, errors.WithStack(ctx.Err())
	}
	if j.prepErr != nil {
		return nil, j.prepErr
	}
	if head {
		return &Generated{Filename: e.filename, Size: j.size}, nil
	}
	return c.served(ctx, e, j)
}

// served returns what a download of e reads from the prepared job j: the
// file as it grows, or the published file once the job has ended. When j
// dropped its cache copy, the download builds its own copy instead.
func (c *Cache) served(ctx context.Context, e *entry, j *job) (*Generated, error) {
	body, state := j.reader(ctx)
	switch state {
	case attached:
		return &Generated{Filename: e.filename, Size: j.size, Body: body}, nil
	case cacheDropped:
		return c.stream(ctx, e, j.gen)
	case jobEnded:
	}
	<-j.finished
	// A job that dropped its cache copy before this download attached may
	// have stopped for want of readers.
	if j.cacheErr != nil && (j.err == nil || errors.Is(j.err, errNoReaders)) {
		return c.stream(ctx, e, j.gen)
	}
	if j.err != nil {
		return nil, j.err
	}
	// Fingerprints of one file share its published path, so look the entry
	// up again rather than trusting that path still holds this job's file.
	return c.open(ctx, e, false)
}

// complete returns the path of e's complete file, generating it or waiting
// for the job already running.
func (c *Cache) complete(ctx context.Context, e *entry) (string, error) {
	hit, err := c.lookup(e)
	if err != nil || hit != "" {
		return hit, err
	}
	if err := c.rememberedFailure(e); err != nil {
		return "", err
	}
	gen, err := e.generator()
	if err != nil {
		return "", err
	}
	j, hit, err := c.join(ctx, e, gen)
	if err != nil || hit != "" {
		return hit, err
	}
	select {
	case <-j.finished:
	case <-ctx.Done():
		return "", errors.WithStack(ctx.Err())
	}
	// A job that dropped its cache copy stops once no download reads it,
	// which says less than why the copy was dropped.
	if j.cacheErr != nil {
		return "", j.cacheErr
	}
	if j.err != nil {
		return "", j.err
	}
	// Fingerprints of one file share its published path, so look the entry
	// up again rather than trusting that path still holds this job's file.
	return c.complete(ctx, e)
}

// lookup returns e's published path on a hit, recording the access.
func (c *Cache) lookup(e *entry) (string, error) {
	hit, err := e.lookup()
	if err != nil {
		return "", errors.Wrap(err, "failed to check cache")
	}
	if hit != "" {
		e.touch()
	}
	return hit, nil
}

// prepareOnly runs only the preparation step, for Head.
func (c *Cache) prepareOnly(ctx context.Context, e *entry, gen filegen.StreamGenerator) (*Generated, error) {
	src, err := filegen.OpenSource(e.file.Filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open source file")
	}
	defer src.Close()
	prepared, err := gen.Prepare(ctx, src, e.book, e.file)
	if err != nil {
		return nil, err
	}
	if built, ok := prepared.(*builtFile); ok {
		os.RemoveAll(built.dir)
	}
	g := &Generated{Filename: e.filename, Size: -1}
	if size, ok := prepared.Size(); ok {
		g.Size = size
	}
	return g, nil
}

// join returns the job running for e, starting one if there is none. When
// a job for e finished between the caller's lookup and now, hit is the
// published path instead.
func (c *Cache) join(ctx context.Context, e *entry, gen filegen.StreamGenerator) (j *job, hit string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j, ok := c.jobs[e.key()]; ok {
		return j, "", nil
	}
	if hit, err := c.lookup(e); err != nil || hit != "" {
		return nil, hit, err
	}
	if c.closed {
		return nil, "", errors.New("download cache is closed")
	}

	f, err := c.createPartial(c.dir)
	if err != nil {
		return nil, "", &unwritableError{err: err}
	}
	j = newJob(f, e.destPath, gen)
	c.jobs[e.key()] = j

	// The job outlives the request but keeps its values (the logger), and
	// ends when the cache closes.
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	j.onDirect = func(err error) {
		// The start of the output is gone, so later downloads build their
		// own copy rather than join this one.
		c.forget(e, j)
		logger.FromContext(jobCtx).Warn("download cache write failed, streaming without caching", c.logData(e, err))
	}
	stop := context.AfterFunc(c.ctx, cancel)
	c.background.Add(1)
	go func() {
		defer c.background.Done()
		defer cancel()
		defer stop()
		c.run(jobCtx, e, j, gen)
	}()
	return j, "", nil
}

// run generates e into j's partial file and publishes it. A failure after
// the preparation step is remembered against e's fingerprint, unless the
// build was only canceled. A cache that fails to take the file does not fail
// the downloads reading it: they still get all of it, and nothing is
// published.
func (c *Cache) run(ctx context.Context, e *entry, j *job, gen filegen.StreamGenerator) {
	err := c.buildRecovering(ctx, e, j, gen)
	writing := isClosed(j.prepared)
	if err == nil && j.cacheErr == nil {
		if perr := c.publish(e, j); perr != nil {
			j.cacheErr = perr
			logger.FromContext(ctx).Warn("download cache write failed, generated file not cached", c.logData(e, perr))
		}
	}
	if err != nil || j.cacheErr != nil {
		os.Remove(j.partialPath)
	}

	c.forget(e, j)
	if !writing {
		j.prepErr = err
		close(j.prepared)
	} else if err != nil && !j.readersGone && !stopped(ctx, err) {
		c.failedPartway(ctx, e, err)
	}
	j.finish(err)

	if err == nil && j.cacheErr == nil {
		c.triggerCleanupAsync()
	}
}

// forget removes j from the running jobs if it is still e's.
func (c *Cache) forget(e *entry, j *job) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.jobs[e.key()] == j {
		delete(c.jobs, e.key())
	}
}

// stopped reports whether a build ended because it was canceled or timed
// out, rather than because the file failed. Builds that stop because every
// download left are told apart where the write failed, since a generator
// need not keep the error it got.
func stopped(ctx context.Context, err error) bool {
	return ctx.Err() != nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// unwritableError is a cache that cannot take a new file.
type unwritableError struct{ err error }

func (u *unwritableError) Error() string { return u.err.Error() }
func (u *unwritableError) Unwrap() error { return u.err }

// stream builds e straight into a download, without the cache, for when
// the cache cannot hold it. The build runs only while the download reads
// it.
func (c *Cache) stream(ctx context.Context, e *entry, gen filegen.StreamGenerator) (*Generated, error) {
	// Counted like a job, so Close waits for it.
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("download cache is closed")
	}
	c.background.Add(1)
	c.mu.Unlock()
	started := false
	defer func() {
		if !started {
			c.background.Done()
		}
	}()

	src, err := filegen.OpenSource(e.file.Filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open source file")
	}
	prepared, err := gen.Prepare(ctx, src, e.book, e.file)
	if err != nil {
		src.Close()
		return nil, err
	}
	size := int64(-1)
	if n, ok := prepared.Size(); ok {
		size = n
	}

	// The build stops when the download ends or the cache closes, and a
	// download that ends without reading the body releases the build.
	buildCtx, cancel := context.WithCancel(ctx)
	stopOnClose := context.AfterFunc(c.ctx, cancel)
	pr, pw := io.Pipe()
	stopOnDone := context.AfterFunc(ctx, func() { pr.CloseWithError(ctx.Err()) })
	started = true
	go func() {
		defer c.background.Done()
		defer src.Close()
		defer cancel()
		defer stopOnClose()
		defer stopOnDone()
		out := &clientWriter{w: pw}
		err := writeRecovering(buildCtx, prepared, src, size, out)
		if err != nil && !out.gone && !stopped(buildCtx, err) {
			c.failedPartway(ctx, e, err)
		}
		pw.CloseWithError(err)
	}()
	return &Generated{Filename: e.filename, Size: size, Body: pr}, nil
}

// clientWriter notes when the download it writes to has gone away.
type clientWriter struct {
	w    io.Writer
	gone bool
}

func (cw *clientWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if err != nil {
		cw.gone = true
	}
	return n, err
}

// writeRecovering runs writeChecked, turning a panic into an error, for
// writes outside a job.
func writeRecovering(ctx context.Context, prepared filegen.Prepared, src *filegen.FileSource, size int64, w io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("generator panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return writeChecked(ctx, prepared, src, size, w)
}

// writeChecked runs prepared's writing step into w and checks that src did
// not change while it was copied and that it wrote the size it reported, or
// -1 for no size.
func writeChecked(ctx context.Context, prepared filegen.Prepared, src *filegen.FileSource, size int64, w io.Writer) error {
	cw := &countingWriter{w: w}
	if err := filegen.WriteFromSource(ctx, prepared, src, cw); err != nil {
		return err
	}
	if size >= 0 && cw.n != size {
		return errors.Errorf("generator reported %d bytes and wrote %d", size, cw.n)
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

// failedPartway logs a build that failed after bytes were sent and
// remembers it against e's fingerprint, so later downloads get the error
// up front instead of another cut connection.
func (c *Cache) failedPartway(ctx context.Context, e *entry, err error) {
	log := logger.FromContext(ctx)
	log.Warn("generated file failed partway through a download", c.logData(e, err))
	if rerr := c.rememberFailure(e, err); rerr != nil {
		log.Warn("failed to remember a generated file failure", c.logData(e, rerr))
	}
}

func (c *Cache) logData(e *entry, err error) logger.Data {
	return logger.Data{
		"file_id":  e.file.ID,
		"filepath": e.file.Filepath,
		"format":   e.meta.Format,
		"error":    err.Error(),
	}
}

// rememberFailure records err against e's fingerprint in e's metadata,
// keeping whatever file the metadata already describes.
func (c *Cache) rememberFailure(e *entry, err error) error {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()
	meta, rerr := e.readMeta()
	if rerr != nil {
		return rerr
	}
	if meta == nil {
		m := e.meta
		m.FingerprintHash = ""
		meta = &m
	}
	// Only the first line: a panic's message carries its stack.
	message, _, _ := strings.Cut(err.Error(), "\n")
	meta.Failure = &FailureRecord{FingerprintHash: e.hash, Version: version.Version, Message: message, FailedAt: time.Now()}
	return e.writeMeta(meta)
}

// rememberedFailure returns the failure recorded against e's fingerprint
// as a generation error, so device routes serve the Original File and the
// web UI shows the error, or nil when there is none.
func (c *Cache) rememberedFailure(e *entry) error {
	meta, err := e.readMeta()
	if err != nil {
		return errors.Wrap(err, "failed to check cache")
	}
	// Another release may have fixed whatever failed, so it tries again.
	if meta == nil || meta.Failure == nil || meta.Failure.FingerprintHash != e.hash || meta.Failure.Version != version.Version {
		return nil
	}
	return filegen.NewGenerationError(e.file.FileType, nil, "an earlier download failed partway: "+meta.Failure.Message)
}

// buildRecovering runs build, turning a panic into an error. The job runs
// outside any request, so no middleware would recover it, and a generator
// that panics on a malformed file would otherwise take down the process.
func (c *Cache) buildRecovering(ctx context.Context, e *entry, j *job, gen filegen.StreamGenerator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("generator panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return c.build(ctx, e, j, gen)
}

// build runs the preparation and writing steps into j.
func (c *Cache) build(ctx context.Context, e *entry, j *job, gen filegen.StreamGenerator) error {
	src, err := filegen.OpenSource(e.file.Filepath)
	if err != nil {
		return errors.Wrap(err, "failed to open source file")
	}
	defer src.Close()

	prepared, err := gen.Prepare(ctx, src, e.book, e.file)
	if err != nil {
		return err
	}
	if built, ok := prepared.(*builtFile); ok && filepath.Dir(built.dir) == c.dir {
		// A file built in full in the cache becomes the job's file as it
		// is, rather than being copied a second time. One staged elsewhere,
		// because the cache was full, is copied through j like any output,
		// so a cache that cannot take it is dropped without failing.
		if err := j.adopt(built); err != nil {
			return err
		}
		close(j.prepared)
		return nil
	}
	if size, ok := prepared.Size(); ok {
		j.size = size
	}
	close(j.prepared)
	return writeChecked(ctx, prepared, src, j.size, j)
}

// publish renames j's complete partial file into place and writes its
// metadata.
func (c *Cache) publish(e *entry, j *job) error {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if err := os.Rename(j.partialPath, e.destPath); err != nil {
		return errors.Wrap(err, "failed to publish generated file")
	}
	now := time.Now()
	meta := e.meta
	meta.GeneratedAt = now
	meta.LastAccessedAt = now
	meta.SizeBytes = j.written
	if err := e.writeMeta(&meta); err != nil {
		// Without metadata the file is unreachable, so don't leave it behind.
		os.Remove(e.destPath)
		return err
	}
	return nil
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Close cancels every running job and waits, until ctx ends, for them to
// delete their partial files and end. Downloads still reading a canceled
// job fail, and no new job starts. Call it on shutdown, once the server has
// stopped taking requests. A generator that is slow to notice the
// cancellation can outlast ctx; RemoveLeftovers clears what it leaves at the
// next startup.
func (c *Cache) Close(ctx context.Context) {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancel()

	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Invalidate removes the cached file for a given file ID.
func (c *Cache) Invalidate(fileID int, fileType string) error {
	return DeleteCachedFile(c.dir, fileID, fileType)
}

// InvalidateKepub removes the cached KePub file for a given file ID.
func (c *Cache) InvalidateKepub(fileID int) error {
	return DeleteKepubCachedFile(c.dir, fileID)
}

// GetCachedPath returns the path to a cached file if it exists and is valid.
// Returns empty string if the cache doesn't exist or is invalid.
func (c *Cache) GetCachedPath(fileID int, fileType string, book *models.Book, file *models.File) (string, error) {
	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return "", errors.Wrap(err, "failed to compute fingerprint")
	}

	hash, err := fp.Hash()
	if err != nil {
		return "", errors.Wrap(err, "failed to hash fingerprint")
	}

	return GetCachedFilePath(c.dir, fileID, fileType, hash)
}

// BulkZipDir returns the directory path for bulk zip files.
func (c *Cache) BulkZipDir() string {
	return filepath.Join(c.dir, "bulk")
}

// BulkZipPath returns the full path for a bulk zip file identified by its fingerprint hash.
func (c *Cache) BulkZipPath(fingerprintHash string) string {
	return filepath.Join(c.dir, "bulk", fingerprintHash+".zip")
}

// BulkZipExists returns true if a bulk zip file with the given fingerprint hash exists.
func (c *Cache) BulkZipExists(fingerprintHash string) bool {
	_, err := os.Stat(c.BulkZipPath(fingerprintHash))
	return err == nil
}

// TriggerCleanup runs cache cleanup if the cache exceeds the max size.
// This runs in the current goroutine - call with `go` to run in background.
func (c *Cache) TriggerCleanup() {
	// Cleanup errors are non-fatal - best effort only
	_ = c.runCleanup()
}

// runCleanup performs the actual cleanup operation.
func (c *Cache) runCleanup() error {
	if c.ShouldSkipCleanup != nil && c.ShouldSkipCleanup() {
		return nil
	}

	// Get lock file to prevent concurrent cleanups
	lockPath := filepath.Join(c.dir, ".cleanup.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// Another cleanup is running or we can't create the lock
		return nil
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath)
	}()

	return RunCleanup(c.dir, c.maxSize)
}

// triggerCleanupAsync runs a best-effort cleanup in the background so a
// download never waits on cache eviction.
func (c *Cache) triggerCleanupAsync() {
	c.background.Add(1)
	go func() {
		defer c.background.Done()
		c.TriggerCleanup()
	}()
}

// Wait blocks until every background cleanup and job started so far has
// finished. Both write into the cache directory, so anything that removes the
// directory (tests using t.TempDir, mostly) must Wait first or the removal can
// fail with "directory not empty".
func (c *Cache) Wait() {
	c.background.Wait()
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}

// MaxSize returns the maximum cache size in bytes.
func (c *Cache) MaxSize() int64 {
	return c.maxSize
}

// SizeBytes returns the total size in bytes and file count of the cache.
// Includes individual cached files (tracked via metadata) and bulk zip files.
// A missing root directory is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error) {
	var totalBytes int64
	var totalCount int

	entries, err := ListCacheEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list cache entries")
	}
	for _, e := range entries {
		totalBytes += e.SizeBytes
		totalCount++
	}

	bulkEntries, bulkBytes, err := listBulkZipEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list bulk entries")
	}
	totalBytes += bulkBytes
	totalCount += len(bulkEntries)

	return totalBytes, totalCount, nil
}

// Clear removes all cached content while preserving the root directory itself.
// Safe to call when the root does not exist.
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "failed to read cache directory")
	}
	for _, e := range entries {
		path := filepath.Join(c.dir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}
