package mp4_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteToFile_FaststartLayoutPreservesAudioChunkOffsets is the regression
// test for the audiobook download corruption bug: M4B files with a faststart
// layout (moov before mdat, as Audible/Apple Books export) became unplayable
// after download because rewriting the metadata grows moov, shifts mdat down,
// but left the stco/co64 chunk offset tables pointing at the old positions —
// which now land inside the grown moov box. The decoder then reads metadata as
// audio ("channel element is not allocated") and players refuse the file.
//
// The contract: after a metadata rewrite, every chunk offset must still point
// at the same audio bytes it pointed at in the source.
func TestWriteToFile_FaststartLayoutPreservesAudioChunkOffsets(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-faststart-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Original Title",
		Artist:    "Original Author",
		Duration:  5.0,
		Faststart: true,
	})

	// The fixture must really be moov-before-mdat, otherwise it doesn't
	// exercise the bug.
	srcMoov, srcMdat := boxPositions(t, srcPath)
	require.Less(t, srcMoov.offset, srcMdat.offset,
		"fixture must be faststart (moov before mdat) to exercise the bug")

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)

	// Grow moov by attaching a large cover so mdat is pushed down on rewrite.
	meta.CoverData = bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 128*1024) // 512 KB
	meta.CoverMimeType = "image/jpeg"

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	// Sanity: moov must have actually grown, or the test proves nothing.
	destMoov, _ := boxPositions(t, destPath)
	require.Greater(t, destMoov.size, srcMoov.size,
		"moov must grow for the test to exercise the shift")

	assertChunkOffsetsLocateSameAudio(t, srcPath, destPath)
}

// TestWriteToFile_MdatFirstLayoutLeavesChunkOffsetsUnchanged guards against
// over-correction: when mdat comes before moov (ffmpeg's default, non-faststart
// layout), growing moov does not move mdat, so the chunk offsets must be left
// exactly as they are.
func TestWriteToFile_MdatFirstLayoutLeavesChunkOffsetsUnchanged(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-mdatfirst-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:    "Original Title",
		Artist:   "Original Author",
		Duration: 5.0,
		// No Faststart: ffmpeg writes mdat before moov.
	})

	srcMoov, srcMdat := boxPositions(t, srcPath)
	require.Greater(t, srcMoov.offset, srcMdat.offset,
		"fixture must be mdat-first to exercise the guard")

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.CoverData = bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 128*1024) // 512 KB
	meta.CoverMimeType = "image/jpeg"

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	destMoov, _ := boxPositions(t, destPath)
	require.Greater(t, destMoov.size, srcMoov.size, "moov must grow for the guard to be meaningful")

	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	dest, err := os.ReadFile(destPath)
	require.NoError(t, err)
	require.Equal(t, collectChunkOffsets(t, src), collectChunkOffsets(t, dest),
		"mdat did not move, so chunk offsets must be untouched")

	// And the audio is of course still where the offsets say it is.
	assertChunkOffsetsLocateSameAudio(t, srcPath, destPath)
}

// TestWriteToFile_FaststartWithChaptersPreservesAudioAndChapters covers the
// realistic case: a real audiobook has a chapter text track in addition to the
// audio track, so the moov holds two chunk-offset tables. On rewrite the audio
// track's offsets must still be shifted to follow the audio when moov grows
// (the #393 guarantee), while the chapter text track is rebuilt from the
// chapters and relocated into a trailing mdat. A regression that failed to shift
// the audio track would corrupt the audio; audio-only fixtures cannot catch the
// two-table case.
func TestWriteToFile_FaststartWithChaptersPreservesAudioAndChapters(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-faststart-chapters-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Original Title",
		Artist:    "Original Author",
		Duration:  5.0,
		Faststart: true,
		Chapters: []testgen.M4BChapter{
			{Title: "Chapter One", Start: 0},
			{Title: "Chapter Two", Start: 2.0},
		},
	})

	srcMoov, srcMdat := boxPositions(t, srcPath)
	require.Less(t, srcMoov.offset, srcMdat.offset, "fixture must be faststart")

	// The fixture must actually contain more than one chunk-offset table,
	// otherwise this degrades to the single-track case and proves nothing.
	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, countChunkOffsetTables(t, src), 2,
		"fixture must have an audio track and a chapter track (two stco tables)")

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.CoverData = bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 128*1024) // 512 KB
	meta.CoverMimeType = "image/jpeg"

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	destMoov, _ := boxPositions(t, destPath)
	require.Greater(t, destMoov.size, srcMoov.size, "moov must grow")

	// The audio track's chunk offsets must still locate the same audio bytes.
	// The chapter track's offset is excluded because it is intentionally
	// rebuilt and moved into a trailing mdat, not shifted in place.
	assertAudioChunkOffsetsLocateSameAudio(t, srcPath, destPath)

	// And the chapters must survive the rebuild, read back from the QuickTime
	// track that players prefer.
	out, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	require.Len(t, out.Chapters, 2)
	assert.Equal(t, "Chapter One", out.Chapters[0].Title)
	assert.Equal(t, "Chapter Two", out.Chapters[1].Title)
}

type boxPos struct {
	offset int
	size   int
}

// boxPositions returns the top-level moov and mdat box positions.
func boxPositions(t *testing.T, path string) (moov, mdat boxPos) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	off := 0
	for off+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[off:]))
		hdr := 8
		switch size {
		case 1:
			require.LessOrEqual(t, off+16, len(data), "truncated 64-bit box header")
			size = int(binary.BigEndian.Uint64(data[off+8:]))
			hdr = 16
		case 0:
			size = len(data) - off
		}
		require.GreaterOrEqual(t, size, hdr, "invalid box size")
		require.LessOrEqual(t, off+size, len(data), "box extends past EOF")

		switch string(data[off+4 : off+8]) {
		case "moov":
			moov = boxPos{offset: off, size: size}
		case "mdat":
			if mdat.size == 0 {
				mdat = boxPos{offset: off, size: size}
			}
		}
		off += size
	}
	return moov, mdat
}

// collectChunkOffsets walks the box tree and returns every stco (32-bit) and
// co64 (64-bit) chunk offset in document order.
func collectChunkOffsets(t *testing.T, data []byte) []uint64 {
	t.Helper()
	var out []uint64

	var walk func(buf []byte)
	walk = func(buf []byte) {
		off := 0
		for off+8 <= len(buf) {
			size := int(binary.BigEndian.Uint32(buf[off:]))
			hdr := 8
			switch size {
			case 1:
				if off+16 > len(buf) {
					return
				}
				size = int(binary.BigEndian.Uint64(buf[off+8:]))
				hdr = 16
			case 0:
				size = len(buf) - off
			}
			if size < hdr || off+size > len(buf) {
				return
			}

			typ := string(buf[off+4 : off+8])
			switch typ {
			case "stco", "co64":
				// body: version(1) + flags(3) + entry_count(4) + entries
				body := buf[off+hdr : off+size]
				require.GreaterOrEqual(t, len(body), 8, "%s box too small", typ)
				count := int(binary.BigEndian.Uint32(body[4:8]))
				p := 8
				for i := 0; i < count; i++ {
					if typ == "stco" {
						require.LessOrEqual(t, p+4, len(body), "stco truncated")
						out = append(out, uint64(binary.BigEndian.Uint32(body[p:p+4])))
						p += 4
					} else {
						require.LessOrEqual(t, p+8, len(body), "co64 truncated")
						out = append(out, binary.BigEndian.Uint64(body[p:p+8]))
						p += 8
					}
				}
			case "moov", "trak", "mdia", "minf", "stbl", "edts":
				walk(buf[off+hdr : off+size])
			}
			off += size
		}
	}
	walk(data)
	return out
}

// countChunkOffsetTables returns the number of stco/co64 boxes in the file.
func countChunkOffsetTables(t *testing.T, data []byte) int {
	t.Helper()
	count := 0

	var walk func(buf []byte)
	walk = func(buf []byte) {
		off := 0
		for off+8 <= len(buf) {
			size := int(binary.BigEndian.Uint32(buf[off:]))
			hdr := 8
			switch size {
			case 1:
				if off+16 > len(buf) {
					return
				}
				size = int(binary.BigEndian.Uint64(buf[off+8:]))
				hdr = 16
			case 0:
				size = len(buf) - off
			}
			if size < hdr || off+size > len(buf) {
				return
			}
			switch string(buf[off+4 : off+8]) {
			case "stco", "co64":
				count++
			case "moov", "trak", "mdia", "minf", "stbl", "edts":
				walk(buf[off+hdr : off+size])
			}
			off += size
		}
	}
	walk(data)
	return count
}

// assertChunkOffsetsLocateSameAudio verifies that every chunk offset in dest
// points at the same audio bytes the corresponding source offset points at.
// Use this only for fixtures with no chapter text track, where every offset
// belongs to the audio track. With a chapter track present, use
// assertAudioChunkOffsetsLocateSameAudio instead, since the chapter track is
// deliberately rebuilt and relocated.
func assertChunkOffsetsLocateSameAudio(t *testing.T, srcPath, destPath string) {
	t.Helper()
	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	dest, err := os.ReadFile(destPath)
	require.NoError(t, err)

	srcOffs := collectChunkOffsets(t, src)
	destOffs := collectChunkOffsets(t, dest)
	require.NotEmpty(t, srcOffs, "source must have chunk offsets")
	assertOffsetsLocateSameBytes(t, src, dest, srcOffs, destOffs)
}

// assertAudioChunkOffsetsLocateSameAudio is the chapter-aware variant: it
// compares only the offsets belonging to audio (handler "soun") tracks, so it
// ignores the chapter text track that the writer rebuilds into a trailing mdat.
func assertAudioChunkOffsetsLocateSameAudio(t *testing.T, srcPath, destPath string) {
	t.Helper()
	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	dest, err := os.ReadFile(destPath)
	require.NoError(t, err)

	srcOffs := audioChunkOffsets(t, src)
	destOffs := audioChunkOffsets(t, dest)
	require.NotEmpty(t, srcOffs, "source must have audio chunk offsets")
	assertOffsetsLocateSameBytes(t, src, dest, srcOffs, destOffs)
}

// assertOffsetsLocateSameBytes checks that each dest offset points at the same
// 64-byte window as the corresponding source offset.
func assertOffsetsLocateSameBytes(t *testing.T, src, dest []byte, srcOffs, destOffs []uint64) {
	t.Helper()
	require.Len(t, destOffs, len(srcOffs), "chunk count must be preserved")

	const window = 64
	for i := range srcOffs {
		so := int(srcOffs[i])
		do := int(destOffs[i])
		require.LessOrEqual(t, so, len(src), "source offset %d past EOF (chunk %d)", so, i)
		require.LessOrEqual(t, do, len(dest), "dest offset %d past EOF (chunk %d)", do, i)

		w := window
		if rem := len(src) - so; rem < w {
			w = rem
		}
		if rem := len(dest) - do; rem < w {
			w = rem
		}
		assert.Equal(t, src[so:so+w], dest[do:do+w],
			"chunk %d: rewritten offset %d does not point at the same audio bytes as source offset %d",
			i, do, so)
	}
}

// audioChunkOffsets returns the chunk offsets that belong to audio (handler
// "soun") tracks, in document order.
func audioChunkOffsets(t *testing.T, data []byte) []uint64 {
	t.Helper()
	var out []uint64
	moov := firstBoxBody(data, "moov")
	require.NotNil(t, moov, "moov box not found")
	walkChildBoxes(moov, func(typ string, body []byte) {
		if typ != "trak" {
			return
		}
		if trackHandlerType(body) == "soun" {
			collectStcoCo64Bodies(body, &out)
		}
	})
	return out
}

// trackHandlerType returns a trak's media handler type (e.g. "soun", "text").
// body is the trak box content (after the 8-byte box header).
func trackHandlerType(body []byte) string {
	mdia := firstBoxBody(body, "mdia")
	if mdia == nil {
		return ""
	}
	hdlr := firstBoxBody(mdia, "hdlr")
	if len(hdlr) < 12 {
		return ""
	}
	// hdlr content: version(1) flags(3) pre_defined(4) handler_type(4) ...
	return string(hdlr[8:12])
}

// collectStcoCo64Bodies walks buf (a box content buffer) and appends every
// stco/co64 chunk offset it finds, recursing into the usual sample-table
// container boxes.
func collectStcoCo64Bodies(buf []byte, out *[]uint64) {
	walkChildBoxes(buf, func(typ string, body []byte) {
		switch typ {
		case "stco":
			if len(body) < 8 {
				return
			}
			count := int(binary.BigEndian.Uint32(body[4:8]))
			p := 8
			for i := 0; i < count && p+4 <= len(body); i++ {
				*out = append(*out, uint64(binary.BigEndian.Uint32(body[p:p+4])))
				p += 4
			}
		case "co64":
			if len(body) < 8 {
				return
			}
			count := int(binary.BigEndian.Uint32(body[4:8]))
			p := 8
			for i := 0; i < count && p+8 <= len(body); i++ {
				*out = append(*out, binary.BigEndian.Uint64(body[p:p+8]))
				p += 8
			}
		case "mdia", "minf", "stbl", "edts":
			collectStcoCo64Bodies(body, out)
		}
	})
}

// firstBoxBody returns the content (after the header) of the first child box of
// the given type within buf, or nil if absent.
func firstBoxBody(buf []byte, typ string) []byte {
	var found []byte
	walkChildBoxes(buf, func(t string, body []byte) {
		if found == nil && t == typ {
			found = body
		}
	})
	return found
}

// walkChildBoxes iterates the immediate child boxes of buf, calling fn with each
// box's type and its content (after the box header).
func walkChildBoxes(buf []byte, fn func(typ string, body []byte)) {
	off := 0
	for off+8 <= len(buf) {
		size := int(binary.BigEndian.Uint32(buf[off:]))
		hdr := 8
		switch size {
		case 1:
			if off+16 > len(buf) {
				return
			}
			size = int(binary.BigEndian.Uint64(buf[off+8:]))
			hdr = 16
		case 0:
			size = len(buf) - off
		}
		if size < hdr || off+size > len(buf) {
			return
		}
		fn(string(buf[off+4:off+8]), buf[off+hdr:off+size])
		off += size
	}
}
