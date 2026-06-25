package mp4

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"

	"github.com/pkg/errors"
)

// rebuiltChapterTrack is the result of regenerating a QuickTime chapter text
// track from a set of chapters.
type rebuiltChapterTrack struct {
	// moovContent is the moov box's children with the chapter track's sample
	// tables and timing rebuilt from the new chapters.
	moovContent []byte
	// sampleData is the concatenated text samples for the rebuilt track. The
	// caller writes these into a trailing mdat and patches the chapter track's
	// co64 entry to that mdat's absolute offset.
	sampleData []byte
	// co64FieldOffset is the byte offset, within moovContent, of the rebuilt
	// chapter track's 8-byte co64 chunk-offset value.
	co64FieldOffset int
}

// childBox is a box located within a parent's content buffer.
type childBox struct {
	typ        string
	start      int // offset within the buffer where the box (header) starts
	end        int // offset within the buffer where the box ends
	headerSize int
}

// childBoxes scans the sibling boxes within a container's content buffer.
func childBoxes(buf []byte) ([]childBox, error) {
	var boxes []childBox
	offset := 0
	for offset+8 <= len(buf) {
		size := int(binary.BigEndian.Uint32(buf[offset:]))
		headerSize := 8
		switch size {
		case 1:
			if offset+16 > len(buf) {
				return nil, errors.New("truncated 64-bit box header")
			}
			size64 := binary.BigEndian.Uint64(buf[offset+8:])
			if size64 > uint64(len(buf)) {
				return nil, errors.New("box size exceeds buffer length")
			}
			// #nosec G115 -- bounds checked against len(buf) above
			size = int(size64)
			headerSize = 16
		case 0:
			size = len(buf) - offset
		}
		if size < headerSize || offset+size > len(buf) {
			return nil, errors.New("invalid box size")
		}
		boxes = append(boxes, childBox{
			typ:        string(buf[offset+4 : offset+8]),
			start:      offset,
			end:        offset + size,
			headerSize: headerSize,
		})
		offset += size
	}
	return boxes, nil
}

// rebuildChapterTextTrack finds the QuickTime chapter text track inside moov
// content and replaces its sample tables (stts/stsc/stsz, emitting a 64-bit
// co64) and sample data with ones built from chapters. Audiobook players and
// ffprobe read this track in preference to the Nero chpl box, so the user's
// edited chapter titles and timings must land here, not only in chpl.
//
// The chapter sample bytes are returned separately for the caller to append as
// a trailing mdat; the co64 entry holds a placeholder offset the caller patches
// to that mdat's absolute file position. The audio mdat is never touched, so
// the audio chunk offsets are governed solely by the existing moov-resize shift.
//
// Returns ok=false (content unchanged) when the source has no chapter text track
// to rebuild, in which case the caller keeps the chpl-only behavior.
func rebuildChapterTextTrack(moovContent []byte, chapters []Chapter) (rebuiltChapterTrack, bool) {
	boxes, err := childBoxes(moovContent)
	if err != nil {
		return rebuiltChapterTrack{}, false
	}

	// Identify the chapter track the same way the reader and players do: the
	// track the audio track's tref/chap reference points at. Matching by track
	// ID (rather than "first text-handler track") keeps the writer in agreement
	// with the reader when a file carries more than one text track.
	chapterIdx := -1
	if targetID, ok := chapterTrackIDFromTref(moovContent, boxes); ok {
		for i, b := range boxes {
			if b.typ != "trak" {
				continue
			}
			trak := moovContent[b.start:b.end]
			if id, idOK := tkhdTrackID(trak); idOK && id == targetID && isChapterTextTrak(trak) {
				chapterIdx = i
				break
			}
		}
	}
	// Fall back to the first text-handler track when there is no tref/chap
	// reference to resolve.
	if chapterIdx == -1 {
		for i, b := range boxes {
			if b.typ == "trak" && isChapterTextTrak(moovContent[b.start:b.end]) {
				chapterIdx = i
				break
			}
		}
	}
	if chapterIdx == -1 {
		return rebuiltChapterTrack{}, false
	}

	origTrak := moovContent[boxes[chapterIdx].start:boxes[chapterIdx].end]
	res, ok := rebuildChapterTrak(origTrak, chapters)
	if !ok {
		return rebuiltChapterTrack{}, false
	}

	var result bytes.Buffer
	co64FieldOffset := -1
	for i, b := range boxes {
		if i == chapterIdx {
			co64FieldOffset = result.Len() + res.co64Rel
			result.Write(res.box)
		} else {
			result.Write(moovContent[b.start:b.end])
		}
	}

	return rebuiltChapterTrack{
		moovContent:     result.Bytes(),
		sampleData:      res.sampleData,
		co64FieldOffset: co64FieldOffset,
	}, true
}

// chapterTrackIDFromTref returns the track ID referenced by the first
// tref/chap box found among the moov's traks (the audio track points at its
// chapter track this way). Mirrors how readQuickTimeChapters locates the track.
func chapterTrackIDFromTref(moovContent []byte, boxes []childBox) (uint32, bool) {
	for _, b := range boxes {
		if b.typ != "trak" {
			continue
		}
		trak := moovContent[b.start:b.end]
		tboxes, err := childBoxes(trak[8:])
		if err != nil {
			continue
		}
		for _, tb := range tboxes {
			if tb.typ != "tref" {
				continue
			}
			tref := trak[8+tb.start : 8+tb.end]
			refs, err := childBoxes(tref[8:])
			if err != nil {
				continue
			}
			for _, rb := range refs {
				if rb.typ == "chap" {
					chap := tref[8+rb.start : 8+rb.end]
					if len(chap) >= 12 {
						return binary.BigEndian.Uint32(chap[8:12]), true
					}
				}
			}
		}
	}
	return 0, false
}

// tkhdTrackID extracts the track ID from a trak's tkhd box.
// Layout: size(4) type(4) version(1) flags(3) then, for v0, creation(4)
// modification(4) track_id(4); for v1, creation(8) modification(8) track_id(4).
func tkhdTrackID(trak []byte) (uint32, bool) {
	boxes, err := childBoxes(trak[8:])
	if err != nil {
		return 0, false
	}
	for _, b := range boxes {
		if b.typ != "tkhd" {
			continue
		}
		box := trak[8+b.start : 8+b.end]
		if len(box) < 9 {
			return 0, false
		}
		if box[8] == 1 {
			if len(box) < 32 {
				return 0, false
			}
			return binary.BigEndian.Uint32(box[28:32]), true
		}
		if len(box) < 24 {
			return 0, false
		}
		return binary.BigEndian.Uint32(box[20:24]), true
	}
	return 0, false
}

// isChapterTextTrak reports whether a trak is a text track (mdia/hdlr handler
// type "text"), which is how QuickTime chapter tracks are tagged.
func isChapterTextTrak(trak []byte) bool {
	if len(trak) < 8 {
		return false
	}
	boxes, err := childBoxes(trak[8:])
	if err != nil {
		return false
	}
	for _, b := range boxes {
		if b.typ != "mdia" {
			continue
		}
		mdia := trak[8+b.start : 8+b.end]
		mboxes, err := childBoxes(mdia[8:])
		if err != nil {
			return false
		}
		for _, mb := range mboxes {
			if mb.typ == "hdlr" {
				return hdlrHandlerType(mdia[8+mb.start:8+mb.end]) == "text"
			}
		}
	}
	return false
}

// hdlrHandlerType extracts the 4-character handler type from an hdlr box.
// Layout: size(4) type(4) version(1) flags(3) pre_defined(4) handler_type(4) ...
func hdlrHandlerType(box []byte) string {
	if len(box) < 20 {
		return ""
	}
	return string(box[16:20])
}

// chapterRebuildResult is the common return of the per-box rebuild helpers: the
// new box bytes, the chapter sample data discovered beneath it, and co64Rel,
// the offset of the chapter track's co64 value relative to the start of the
// returned box.
type chapterRebuildResult struct {
	box        []byte
	sampleData []byte
	co64Rel    int
}

// replaceChapterChild rebuilds a container box by replacing its single child of
// type childType via rebuildFn and copying every other child verbatim, threading
// the co64 offset up from the rebuilt subtree. It is the shared spine of the
// trak -> mdia -> minf descent toward the chapter track's stbl.
func replaceChapterChild(boxType string, container []byte, childType string,
	rebuildFn func(child []byte) (chapterRebuildResult, bool),
) (chapterRebuildResult, bool) {
	boxes, err := childBoxes(container[8:])
	if err != nil {
		return chapterRebuildResult{}, false
	}
	var content bytes.Buffer
	co64Rel := -1
	var sampleData []byte
	for _, b := range boxes {
		child := container[8+b.start : 8+b.end]
		if b.typ == childType {
			res, cok := rebuildFn(child)
			if !cok {
				return chapterRebuildResult{}, false
			}
			co64Rel = content.Len() + res.co64Rel
			content.Write(res.box)
			sampleData = res.sampleData
		} else {
			content.Write(child)
		}
	}
	if co64Rel == -1 {
		return chapterRebuildResult{}, false
	}
	return chapterRebuildResult{
		box:        buildBox(boxType, content.Bytes()),
		sampleData: sampleData,
		co64Rel:    8 + co64Rel,
	}, true
}

// rebuildChapterTrak rebuilds a chapter text trak, replacing the sample tables
// and timing while preserving tkhd/edts verbatim so the audio track's tref/chap
// reference and the chapter track ID stay valid.
func rebuildChapterTrak(trak []byte, chapters []Chapter) (chapterRebuildResult, bool) {
	return replaceChapterChild("trak", trak, "mdia", func(mdia []byte) (chapterRebuildResult, bool) {
		return rebuildChapterMdia(mdia, chapters)
	})
}

// rebuildChapterMdia reads the media timescale and rebuilds the minf within.
func rebuildChapterMdia(mdia []byte, chapters []Chapter) (chapterRebuildResult, bool) {
	timescale := uint32(0)
	if boxes, err := childBoxes(mdia[8:]); err == nil {
		for _, b := range boxes {
			if b.typ == "mdhd" {
				timescale = parseMdhdTimescale(mdia[8+b.start : 8+b.end])
			}
		}
	}
	if timescale == 0 {
		timescale = 1000
	}
	return replaceChapterChild("mdia", mdia, "minf", func(minf []byte) (chapterRebuildResult, bool) {
		return replaceChapterChild("minf", minf, "stbl", func(stbl []byte) (chapterRebuildResult, bool) {
			return rebuildChapterStbl(stbl, chapters, timescale)
		})
	})
}

// rebuildChapterStbl preserves stsd (and any non-table boxes) and regenerates
// the stts/stsc/stsz/co64 sample tables from the chapters. The chunk offset is a
// placeholder; the caller patches it once the trailing mdat position is known.
func rebuildChapterStbl(stbl []byte, chapters []Chapter, timescale uint32) (chapterRebuildResult, bool) {
	boxes, err := childBoxes(stbl[8:])
	if err != nil {
		return chapterRebuildResult{}, false
	}

	stts, stsc, stsz, co64, data := buildChapterSampleTables(chapters, timescale)

	// Preserve everything except the chunk/sample tables we regenerate.
	regenerated := map[string]bool{
		"stts": true, "stsc": true, "stsz": true, "stz2": true, "stco": true, "co64": true,
	}
	var content bytes.Buffer
	for _, b := range boxes {
		if regenerated[b.typ] {
			continue
		}
		content.Write(stbl[8+b.start : 8+b.end])
	}
	content.Write(stts)
	content.Write(stsc)
	content.Write(stsz)
	co64Start := content.Len()
	content.Write(co64)

	// co64 layout: header(8) version+flags(4) entry_count(4) then the 8-byte
	// offset value, so the value sits 16 bytes into the co64 box.
	return chapterRebuildResult{
		box:        buildBox("stbl", content.Bytes()),
		sampleData: data,
		co64Rel:    8 + co64Start + 16,
	}, true
}

// buildChapterSampleTables builds the stts, stsc, stsz, and co64 boxes plus the
// concatenated text sample bytes for a chapter text track. All samples are
// placed in a single chunk; the co64 offset is left as a placeholder (0).
func buildChapterSampleTables(chapters []Chapter, timescale uint32) (stts, stsc, stsz, co64, sampleData []byte) {
	n := len(chapters)

	var data bytes.Buffer
	sizes := make([]uint32, n)
	for i, ch := range chapters {
		sample := buildChapterTextSample(ch.Title)
		// #nosec G115 -- sample length is bounded by buildChapterTextSample
		sizes[i] = uint32(len(sample))
		data.Write(sample)
	}

	deltas := computeChapterDeltas(chapters, timescale)

	var sttsBody bytes.Buffer
	sttsBody.Write([]byte{0, 0, 0, 0}) // version + flags
	// #nosec G115 -- chapter count is bounded by practical limits
	_ = binary.Write(&sttsBody, binary.BigEndian, uint32(n)) // entry_count
	for i := 0; i < n; i++ {
		_ = binary.Write(&sttsBody, binary.BigEndian, uint32(1)) // sample_count
		_ = binary.Write(&sttsBody, binary.BigEndian, deltas[i]) // sample_delta
	}
	stts = buildBox("stts", sttsBody.Bytes())

	var stscBody bytes.Buffer
	stscBody.Write([]byte{0, 0, 0, 0})                       // version + flags
	_ = binary.Write(&stscBody, binary.BigEndian, uint32(1)) // entry_count
	_ = binary.Write(&stscBody, binary.BigEndian, uint32(1)) // first_chunk
	// #nosec G115 -- chapter count is bounded by practical limits
	_ = binary.Write(&stscBody, binary.BigEndian, uint32(n)) // samples_per_chunk
	_ = binary.Write(&stscBody, binary.BigEndian, uint32(1)) // sample_description_index
	stsc = buildBox("stsc", stscBody.Bytes())

	var stszBody bytes.Buffer
	stszBody.Write([]byte{0, 0, 0, 0})                       // version + flags
	_ = binary.Write(&stszBody, binary.BigEndian, uint32(0)) // sample_size = 0 (per-sample sizes follow)
	// #nosec G115 -- chapter count is bounded by practical limits
	_ = binary.Write(&stszBody, binary.BigEndian, uint32(n)) // sample_count
	for _, s := range sizes {
		_ = binary.Write(&stszBody, binary.BigEndian, s)
	}
	stsz = buildBox("stsz", stszBody.Bytes())

	var co64Body bytes.Buffer
	co64Body.Write([]byte{0, 0, 0, 0})                       // version + flags
	_ = binary.Write(&co64Body, binary.BigEndian, uint32(1)) // entry_count
	_ = binary.Write(&co64Body, binary.BigEndian, uint64(0)) // chunk offset (placeholder)
	co64 = buildBox("co64", co64Body.Bytes())

	return stts, stsc, stsz, co64, data.Bytes()
}

// buildChapterTextSample encodes a chapter title as a QuickTime text sample: a
// 2-byte big-endian length prefix, the UTF-8 text, then a 12-byte "encd"
// text-encoding atom. The encd atom mirrors what ffmpeg and Apple write for
// chapter samples; readers that only want the title (Shisho's own, ffprobe)
// ignore it, but emitting it keeps the samples byte-shaped like real audiobooks
// for stricter players.
func buildChapterTextSample(title string) []byte {
	b := []byte(title)
	// Reserve room for the 12-byte encd atom so the title itself can still be
	// the full uint16 range.
	if len(b) > math.MaxUint16 {
		b = b[:math.MaxUint16]
	}
	sample := make([]byte, 2+len(b)+12)
	// #nosec G115 -- length clamped to MaxUint16 above
	binary.BigEndian.PutUint16(sample[0:2], uint16(len(b)))
	copy(sample[2:], b)
	encd := sample[2+len(b):]
	binary.BigEndian.PutUint32(encd[0:4], 12) // box size
	copy(encd[4:8], "encd")                   // box type
	binary.BigEndian.PutUint32(encd[8:12], 0x00000100)
	return sample
}

// computeChapterDeltas converts chapter start/end times into per-sample
// durations (in media timescale units). Deltas are taken from the gaps between
// consecutive starts so that a reader accumulating them reproduces the start
// times; the final sample runs to the last chapter's end. A QuickTime chapter
// track tiles the timeline from 0, so a non-zero start on the first chapter is
// absorbed (the first sample begins at 0); the Nero chpl box still carries the
// absolute first timestamp.
func computeChapterDeltas(chapters []Chapter, timescale uint32) []uint32 {
	n := len(chapters)
	deltas := make([]uint32, n)
	starts := make([]int64, n)
	for i, ch := range chapters {
		starts[i] = durationToUnits(ch.Start, timescale)
	}
	for i := 0; i < n; i++ {
		var d int64
		if i < n-1 {
			d = starts[i+1] - starts[i]
		} else {
			d = durationToUnits(chapters[i].End, timescale) - starts[i]
		}
		if d < 1 {
			d = 1
		}
		if d > math.MaxUint32 {
			d = math.MaxUint32
		}
		// #nosec G115 -- clamped to [1, MaxUint32] above
		deltas[i] = uint32(d)
	}
	return deltas
}

// durationToUnits converts a duration to media timescale units.
func durationToUnits(d time.Duration, timescale uint32) int64 {
	if d <= 0 {
		return 0
	}
	return int64(math.Round(d.Seconds() * float64(timescale)))
}

// parseMdhdTimescale extracts the timescale field from an mdhd box.
// Layout (v0): size(4) type(4) version(1) flags(3) creation(4) modification(4) timescale(4) ...
// Layout (v1): ... creation(8) modification(8) timescale(4) ...
func parseMdhdTimescale(box []byte) uint32 {
	if len(box) < 24 {
		return 0
	}
	version := box[8]
	if version == 1 {
		if len(box) < 32 {
			return 0
		}
		return binary.BigEndian.Uint32(box[28:32])
	}
	return binary.BigEndian.Uint32(box[20:24])
}
