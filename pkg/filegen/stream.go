package filegen

import (
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
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
	f    *os.File
	size int64
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
	return &FileSource{f: f, size: info.Size()}, nil
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
