// Package ziputil writes zip entries without recompressing them, for
// generators that stream an archive built from another one.
package ziputil

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"strings"

	"github.com/pkg/errors"
)

// dataDescriptorFlag marks an entry whose sizes and CRC follow its data
// instead of sitting in its local header.
const dataDescriptorFlag = 0x8

// zip64ExtraID is the extra field holding an entry's 64-bit sizes and
// offset.
const zip64ExtraID = 0x0001

// RawHeader returns a header for copying f with zip.Writer.CreateRaw. The
// sizes and CRC go in the local header, so no data descriptor follows the
// data, and the source's ZIP64 extra field is dropped: it describes the
// source's offsets, and the writer adds its own when the output needs one.
// A directory becomes an empty stored entry, since the writer takes no data
// for one; some tools deflate a directory's empty body.
func RawHeader(f *zip.File) *zip.FileHeader {
	fh := f.FileHeader
	fh.Flags &^= dataDescriptorFlag
	fh.Extra = withoutExtra(fh.Extra, zip64ExtraID)
	if isDir(fh.Name) {
		fh.Method = zip.Store
		fh.CRC32 = 0
		fh.CompressedSize64 = 0
		fh.UncompressedSize64 = 0
	}
	return &fh
}

// isDir reports whether an entry named name is a directory, which has no
// data.
func isDir(name string) bool {
	return strings.HasSuffix(name, "/")
}

// CopyRaw writes f to zw as is, without decompressing it.
func CopyRaw(ctx context.Context, zw *zip.Writer, f *zip.File) error {
	return CopyRawWith(zw, f, func(w io.Writer, f *zip.File) error {
		return CopyData(ctx, w, f)
	})
}

// CopyRawWith creates f's entry in zw with RawHeader and has body write its
// raw data, unless it is a directory. Measuring an archive passes a body
// that writes placeholder bytes, so the measured size comes from the same
// code as the real write.
func CopyRawWith(zw *zip.Writer, f *zip.File, body func(io.Writer, *zip.File) error) error {
	w, err := zw.CreateRaw(RawHeader(f))
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", f.Name)
	}
	if isDir(f.Name) {
		return nil
	}
	return errors.Wrapf(body(w, f), "failed to copy %s", f.Name)
}

// CopyData copies f's raw data to w, checking ctx between chunks. A source
// that ends before the data does is an error rather than a short entry.
func CopyData(ctx context.Context, w io.Writer, f *zip.File) error {
	if f.CompressedSize64 > math.MaxInt64 {
		return errors.Errorf("%s is too large", f.Name)
	}
	r, err := f.OpenRaw()
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = io.CopyN(ctxWriter{ctx: ctx, w: w}, r, int64(f.CompressedSize64)) //nolint:gosec // checked against MaxInt64 above
	return errors.WithStack(err)
}

// ctxWriter stops writing once its context ends, so copying a large entry
// can be canceled partway.
type ctxWriter struct {
	ctx context.Context
	w   io.Writer
}

func (c ctxWriter) Write(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.w.Write(p)
}

// Entry is an entry compressed ahead of writing, so its size is known and
// writing it is a plain copy.
type Entry struct {
	Header zip.FileHeader
	Data   []byte
}

// Compress compresses content with method, which is zip.Store or
// zip.Deflate.
func Compress(name string, method uint16, content []byte) (*Entry, error) {
	e := &Entry{Header: zip.FileHeader{
		Name:               name,
		Method:             method,
		CRC32:              crc32.ChecksumIEEE(content),
		UncompressedSize64: uint64(len(content)),
	}}
	switch method {
	case zip.Store:
		e.Data = content
	case zip.Deflate:
		var buf bytes.Buffer
		fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := fw.Write(content); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := fw.Close(); err != nil {
			return nil, errors.WithStack(err)
		}
		e.Data = buf.Bytes()
	default:
		return nil, errors.Errorf("unsupported compression method %d for %s", method, name)
	}
	e.Header.CompressedSize64 = uint64(len(e.Data))
	return e, nil
}

// Write writes e to zw.
func (e *Entry) Write(zw *zip.Writer) error {
	fh := e.Header
	w, err := zw.CreateRaw(&fh)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", e.Header.Name)
	}
	if _, err := w.Write(e.Data); err != nil {
		return errors.Wrapf(err, "failed to write %s", e.Header.Name)
	}
	return nil
}

// withoutExtra returns extra without the fields with the given id. A
// malformed tail is kept as is.
func withoutExtra(extra []byte, id uint16) []byte {
	var out []byte
	for len(extra) >= 4 {
		size := int(binary.LittleEndian.Uint16(extra[2:4]))
		if 4+size > len(extra) {
			break
		}
		if binary.LittleEndian.Uint16(extra[:2]) != id {
			out = append(out, extra[:4+size]...)
		}
		extra = extra[4+size:]
	}
	return append(out, extra...)
}
