package ziputil

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readAll(t *testing.T, f *zip.File) string {
	t.Helper()
	r, err := f.Open()
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}

// A copied entry reads back the same, keeps its compression, and carries
// its sizes in the local header rather than a data descriptor.
func TestCopyRaw(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("compressible text ", 200)
	var src bytes.Buffer
	zw := zip.NewWriter(&src)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Deflate})
	require.NoError(t, err)
	_, err = io.WriteString(w, content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(src.Bytes()), int64(src.Len()))
	require.NoError(t, err)
	require.NotZero(t, zr.File[0].Flags&dataDescriptorFlag, "zip.Writer uses a data descriptor")

	var out bytes.Buffer
	zw = zip.NewWriter(&out)
	require.NoError(t, CopyRaw(context.Background(), zw, zr.File[0]))
	require.NoError(t, zw.Close())

	copied, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	f := copied.File[0]
	assert.Equal(t, content, readAll(t, f))
	assert.Equal(t, zip.Deflate, f.Method)
	assert.Equal(t, zr.File[0].CompressedSize64, f.CompressedSize64)
	assert.Zero(t, f.Flags&dataDescriptorFlag)
}

func TestCompress(t *testing.T) {
	t.Parallel()
	content := []byte(strings.Repeat("abc", 1000))
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		e, err := Compress("x", method, content)
		require.NoError(t, err)
		var out bytes.Buffer
		zw := zip.NewWriter(&out)
		require.NoError(t, e.Write(zw))
		require.NoError(t, zw.Close())
		zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
		require.NoError(t, err)
		assert.Equal(t, string(content), readAll(t, zr.File[0]))
		assert.Equal(t, method, zr.File[0].Method)
	}
	_, err := Compress("x", 99, content)
	require.Error(t, err)
}

func TestWithoutExtra(t *testing.T) {
	t.Parallel()
	extra := []byte{
		0x01, 0x00, 0x04, 0x00, 1, 2, 3, 4, // zip64, dropped
		0x55, 0x54, 0x01, 0x00, 9, // extended timestamp, kept
		0x01, 0x00, // malformed tail, kept
	}
	assert.Equal(t, []byte{0x55, 0x54, 0x01, 0x00, 9, 0x01, 0x00}, withoutExtra(extra, zip64ExtraID))
}

// A directory entry stored with a deflated empty body is copied as an empty
// stored directory, since zip.Writer accepts no data for one.
func TestCopyRaw_Directory(t *testing.T) {
	t.Parallel()
	// zip.Writer will not write data for a directory, so write the entry
	// under a file name of the same length and rename it in the bytes.
	var src bytes.Buffer
	zw := zip.NewWriter(&src)
	_, err := zw.CreateHeader(&zip.FileHeader{Name: "META-INF_", Method: zip.Deflate})
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	raw := bytes.ReplaceAll(src.Bytes(), []byte("META-INF_"), []byte("META-INF/"))
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NotZero(t, zr.File[0].CompressedSize64, "the fixture directory has a body")

	var out bytes.Buffer
	zw = zip.NewWriter(&out)
	require.NoError(t, CopyRaw(context.Background(), zw, zr.File[0]))
	require.NoError(t, zw.Close())

	copied, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	f := copied.File[0]
	assert.Equal(t, "META-INF/", f.Name)
	assert.Equal(t, zip.Store, f.Method)
	assert.Zero(t, f.CompressedSize64)
	assert.Empty(t, readAll(t, f))
}
