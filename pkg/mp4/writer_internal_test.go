package mp4

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the chunk-offset shift helpers directly. The integration
// tests in writer_chunkoffset_test.go can only produce small files that always
// use 32-bit stco, so the co64 path and the overflow guard are unreachable from
// there and are unit-tested here against hand-built boxes.

// stcoBox builds an stco box (full box including header) from the given 32-bit
// chunk offsets.
func stcoBox(offsets ...uint32) []byte {
	content := make([]byte, 8+4*len(offsets))
	// content[0:4] = version + flags (zero)
	// #nosec G115 -- test input, len is small
	binary.BigEndian.PutUint32(content[4:8], uint32(len(offsets)))
	for i, o := range offsets {
		binary.BigEndian.PutUint32(content[8+4*i:], o)
	}
	return buildBox("stco", content)
}

// co64Box builds a co64 box (full box including header) from the given 64-bit
// chunk offsets.
func co64Box(offsets ...uint64) []byte {
	content := make([]byte, 8+8*len(offsets))
	// #nosec G115 -- test input, len is small
	binary.BigEndian.PutUint32(content[4:8], uint32(len(offsets)))
	for i, o := range offsets {
		binary.BigEndian.PutUint64(content[8+8*i:], o)
	}
	return buildBox("co64", content)
}

func stcoOffsets(box []byte) []uint32 {
	count := int(binary.BigEndian.Uint32(box[12:16]))
	out := make([]uint32, count)
	for i := 0; i < count; i++ {
		out[i] = binary.BigEndian.Uint32(box[16+4*i:])
	}
	return out
}

func co64Offsets(box []byte) []uint64 {
	count := int(binary.BigEndian.Uint32(box[12:16]))
	out := make([]uint64, count)
	for i := 0; i < count; i++ {
		out[i] = binary.BigEndian.Uint64(box[16+8*i:])
	}
	return out
}

func TestShiftStco_AddsPositiveDelta(t *testing.T) {
	t.Parallel()
	box := stcoBox(1000, 2000, 3000)
	require.NoError(t, shiftStco(box, 500))
	assert.Equal(t, []uint32{1500, 2500, 3500}, stcoOffsets(box))
}

func TestShiftStco_AddsNegativeDelta(t *testing.T) {
	t.Parallel()
	box := stcoBox(1000, 2000)
	require.NoError(t, shiftStco(box, -250))
	assert.Equal(t, []uint32{750, 1750}, stcoOffsets(box))
}

func TestShiftStco_ErrorsWhenShiftOverflowsUint32(t *testing.T) {
	t.Parallel()
	box := stcoBox(math.MaxUint32 - 10)
	err := shiftStco(box, 100)
	require.Error(t, err, "an offset that would exceed uint32 must error, not wrap")
}

func TestShiftStco_ErrorsWhenShiftUnderflowsBelowZero(t *testing.T) {
	t.Parallel()
	box := stcoBox(10)
	err := shiftStco(box, -100)
	require.Error(t, err)
}

func TestShiftCo64_AddsDeltaTo64BitOffsets(t *testing.T) {
	t.Parallel()
	// Offsets above 4 GiB, only representable as co64.
	box := co64Box(5_000_000_000, 8_000_000_000)
	require.NoError(t, shiftCo64(box, 1_000))
	assert.Equal(t, []uint64{5_000_001_000, 8_000_001_000}, co64Offsets(box))
}

func TestShiftCo64_ErrorsWhenShiftUnderflowsBelowZero(t *testing.T) {
	t.Parallel()
	box := co64Box(100)
	err := shiftCo64(box, -200)
	require.Error(t, err)
}

// TestShiftChunkOffsets_WalksNestedTracksAndBothTableTypes verifies the walker
// descends moov → trak → mdia → minf → stbl and patches every stco/co64 it
// finds, including a co64 table (which integration fixtures never produce).
func TestShiftChunkOffsets_WalksNestedTracksAndBothTableTypes(t *testing.T) {
	t.Parallel()

	stbl := func(table []byte) []byte {
		minf := buildBox("minf", table)
		mdia := buildBox("mdia", minf)
		return buildBox("trak", mdia)
	}

	audioTrak := stbl(buildBox("stbl", stcoBox(1000, 2000)))
	chapterTrak := stbl(buildBox("stbl", co64Box(9_000_000_000)))

	moov := buildBox("moov", append(append([]byte{}, audioTrak...), chapterTrak...))

	require.NoError(t, shiftChunkOffsets(moov, 777))

	got := collectAllOffsets(t, moov)
	assert.Equal(t, []uint64{1777, 2777, 9_000_000_777}, got,
		"both the audio stco and the chapter co64 must be shifted")
}

// collectAllOffsets walks a moov box and returns every stco/co64 entry in order.
func collectAllOffsets(t *testing.T, moov []byte) []uint64 {
	t.Helper()
	var out []uint64
	var walk func(buf []byte)
	walk = func(buf []byte) {
		off := 0
		for off+8 <= len(buf) {
			size := int(binary.BigEndian.Uint32(buf[off:]))
			if size < 8 || off+size > len(buf) {
				return
			}
			switch string(buf[off+4 : off+8]) {
			case "stco":
				for _, o := range stcoOffsets(buf[off : off+size]) {
					out = append(out, uint64(o))
				}
			case "co64":
				out = append(out, co64Offsets(buf[off:off+size])...)
			case "moov", "trak", "mdia", "minf", "stbl", "edts":
				walk(buf[off+8 : off+size])
			}
			off += size
		}
	}
	walk(moov[8:])
	return out
}
