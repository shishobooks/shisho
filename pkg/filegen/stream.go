package filegen

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
)

// StreamGenerator builds a Generated File in two steps so a download can
// start sending before the file is complete (ADR 0010). Prepare does the
// cheap up-front work and is the only step that may reject the file: an
// error there is returned before any byte is sent. The Prepared it returns
// then writes the output front to back.
type StreamGenerator interface {
	// Prepare opens and parses what it needs from src and checks that the
	// file can be generated. It must not write output, and should read no
	// more of src than it needs to start writing. src stays open until the
	// Prepared's Write returns.
	Prepare(ctx context.Context, src Source, book *models.Book, file *models.File) (Prepared, error)
}

// Prepared is a Generated File ready to be written.
type Prepared interface {
	// Size returns the exact number of bytes Write writes when it is known
	// before writing, and ok false otherwise. A download sends it as
	// Content-Length, so it must be exact.
	Size() (size int64, ok bool)
	// Write writes the output to w front to back, never seeking, and never
	// holding the whole output in memory. It is called at most once, and
	// only after a successful Prepare.
	Write(ctx context.Context, w io.Writer) error
}

// Source is an open Original File a StreamGenerator reads from. Read it
// through ReaderAt (io.NewSectionReader for sequential access), so several
// readers can share it.
type Source interface {
	io.ReaderAt
	Size() int64
}

// FileSource is a Source backed by a file on disk.
type FileSource struct {
	f       *os.File
	size    int64
	modTime time.Time
}

// OpenSource opens the file at path as a Source. The caller closes it.
func OpenSource(path string) (*FileSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, errors.WithStack(err)
	}
	return &FileSource{f: f, size: info.Size(), modTime: info.ModTime()}, nil
}

// ReadAt implements io.ReaderAt.
func (s *FileSource) ReadAt(p []byte, off int64) (int, error) {
	return s.f.ReadAt(p, off)
}

// Size returns the size of the file when it was opened.
func (s *FileSource) Size() int64 {
	return s.size
}

// Close closes the file.
func (s *FileSource) Close() error {
	return s.f.Close()
}

// Unchanged returns an error when the file's size or modification time
// differ from when it was opened, meaning what was read from it may mix old
// and new contents.
func (s *FileSource) Unchanged() error {
	info, err := s.f.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	if info.Size() != s.size || !info.ModTime().Equal(s.modTime) {
		return errors.New("source file changed while it was being read")
	}
	return nil
}

// WriteFromSource runs prepared's writing step into w and then checks that
// src is unchanged since it was opened. A generator copies from its source
// while it writes, so a source rewritten mid-copy (by a library scan, say)
// yields a file mixing old and new contents, and the write fails instead.
// When prepared reports a size, its last byte is held back until the check
// passes, so a download sent with Content-Length never looks complete; one
// sent chunked fails without its final chunk either way.
func WriteFromSource(ctx context.Context, prepared Prepared, src *FileSource, w io.Writer) error {
	size, known := prepared.Size()
	if !known || size == 0 {
		if err := prepared.Write(ctx, w); err != nil {
			return err
		}
		return src.Unchanged()
	}
	held := &holdLastByte{w: w, before: size - 1}
	if err := prepared.Write(ctx, held); err != nil {
		return err
	}
	if err := src.Unchanged(); err != nil {
		return err
	}
	return held.release()
}

// generateFile writes gen's output for the file at srcPath to destPath, for
// the path-based Generate of a format that streams. The output appears at
// destPath only once it is complete.
func generateFile(ctx context.Context, gen StreamGenerator, fileType, srcPath, destPath string, book *models.Book, file *models.File) error {
	src, err := OpenSource(srcPath)
	if err != nil {
		return NewGenerationError(fileType, err, "failed to open source file")
	}
	defer src.Close()

	prepared, err := gen.Prepare(ctx, src, book, file)
	if err != nil {
		return err
	}

	tmp, err := fileutils.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*", 0o644)
	if err != nil {
		return NewGenerationError(fileType, err, "failed to create destination file")
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriter(tmp)
	err = WriteFromSource(ctx, prepared, src, w)
	if err == nil {
		err = errors.WithStack(w.Flush())
	}
	if closeErr := tmp.Close(); err == nil {
		err = errors.WithStack(closeErr)
	}
	if err != nil {
		var genErr *GenerationError
		if errors.As(err, &genErr) {
			return err
		}
		return NewGenerationError(fileType, err, "failed to write file")
	}
	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return NewGenerationError(fileType, err, "failed to write file")
	}
	return nil
}

// holdLastByte passes writes through, except the byte at offset before,
// which it holds until release. Bytes after it, which only a generator
// writing more than it reported produces, release it.
type holdLastByte struct {
	w      io.Writer
	before int64
	held   []byte
}

// Write reports every byte as written unless w fails; its callers give up on
// the whole output then.
func (h *holdLastByte) Write(p []byte) (int, error) {
	n := len(p)
	if h.before >= int64(len(p)) {
		h.before -= int64(len(p))
		return h.w.Write(p)
	}
	if h.before >= 0 {
		if _, err := h.w.Write(p[:h.before]); err != nil {
			return 0, errors.WithStack(err)
		}
		h.held = []byte{p[h.before]}
		p = p[h.before+1:]
		h.before = -1
		if len(p) == 0 {
			return n, nil
		}
	}
	if err := h.release(); err != nil {
		return 0, err
	}
	if _, err := h.w.Write(p); err != nil {
		return 0, errors.WithStack(err)
	}
	return n, nil
}

// release writes the byte held back, if any.
func (h *holdLastByte) release() error {
	if h.held == nil {
		return nil
	}
	_, err := h.w.Write(h.held)
	h.held = nil
	return errors.WithStack(err)
}
