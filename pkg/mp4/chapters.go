package mp4

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	gomp4 "github.com/abema/go-mp4"
	"github.com/pkg/errors"
)

// readChapters reads chapters from an M4B file.
// Priority: QuickTime chapters > Nero chapters.
func readChapters(r io.ReadSeeker) ([]Chapter, error) {
	// First, try to find QuickTime chapters
	chapters, err := readQuickTimeChapters(r)
	if err == nil && len(chapters) > 0 {
		return chapters, nil
	}

	// Fallback to Nero chapters
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}
	return readNeroChapters(r)
}

// readNeroChapters reads Nero-format chapters from the chpl box.
// Path: moov/udta/chpl.
func readNeroChapters(r io.ReadSeeker) ([]Chapter, error) {
	var chapters []Chapter
	var chplData []byte

	_, err := gomp4.ReadBoxStructure(r, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case BoxTypeMoov:
			return h.Expand()
		case BoxTypeUdta:
			return h.Expand()
		case BoxTypeChpl:
			// Read the chpl box raw data
			var buf bytes.Buffer
			_, err := h.ReadData(&buf)
			if err != nil {
				return nil, err
			}
			chplData = buf.Bytes()
			return nil, nil
		default:
			return nil, nil
		}
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(chplData) < 8 {
		return nil, nil
	}

	// Parse chpl box format:
	// [1 byte version][3 bytes flags][4 bytes reserved (version 0) or 1 byte reserved (version 1)]
	// [4 bytes chapter count (version 0) or 1 byte count (version 1)]
	// Then for each chapter:
	// [8 bytes timestamp][1 byte title length][title bytes]

	version := chplData[0]
	offset := 4 // Skip version and flags

	var chapterCount int
	if version == 0 {
		offset += 4 // Skip reserved
		if len(chplData) < offset+4 {
			return nil, nil
		}
		chapterCount = int(binary.BigEndian.Uint32(chplData[offset:]))
		offset += 4
	} else {
		offset++ // Skip reserved byte
		if len(chplData) < offset+1 {
			return nil, nil
		}
		chapterCount = int(chplData[offset])
		offset++
	}

	// Parse chapters
	for i := 0; i < chapterCount && offset < len(chplData)-9; i++ {
		startTime, title, bytesRead := parseNeroChapterEntry(chplData[offset:])
		if bytesRead == 0 {
			break
		}
		// Nero timestamps are in 100-nanosecond units
		chapters = append(chapters, Chapter{
			Title: title,
			Start: time.Duration(startTime) * 100 * time.Nanosecond,
		})
		offset += bytesRead
	}

	// Calculate end times based on next chapter's start
	for i := 0; i < len(chapters); i++ {
		if i < len(chapters)-1 {
			chapters[i].End = chapters[i+1].Start
		}
	}

	return chapters, nil
}

// chapterTrackInfo holds information about a chapter track.
type chapterTrackInfo struct {
	trackID         uint32
	timescale       uint32
	sampleCount     uint32
	sampleDeltas    []uint32    // from stts
	sampleSizes     []uint32    // from stsz
	chunkOffsets    []uint64    // from stco/co64
	samplesPerChunk []stscEntry // from stsc
}

// stscEntry represents a sample-to-chunk entry.
type stscEntry struct {
	firstChunk      uint32
	samplesPerChunk uint32
}

// readQuickTimeChapters reads QuickTime-format chapters.
// These are stored in a text track referenced via tref/chap.
func readQuickTimeChapters(r io.ReadSeeker) ([]Chapter, error) {
	// First pass: find which track ID is the chapter track
	var chapterTrackID uint32
	var movieTimescale uint32

	_, err := gomp4.ReadBoxStructure(r, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case BoxTypeMoov:
			return h.Expand()
		case BoxTypeMvhd:
			payload, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			if mvhd, ok := payload.(*gomp4.Mvhd); ok {
				movieTimescale = mvhd.Timescale
			}
			return nil, nil
		case BoxTypeTrak:
			return h.Expand()
		case BoxTypeTref:
			// tref contains child boxes like "chap" - parse manually
			var buf bytes.Buffer
			if _, err := h.ReadData(&buf); err != nil {
				return nil, err
			}
			data := buf.Bytes()
			// Parse child boxes within tref
			offset := 0
			for offset+8 <= len(data) {
				childSize := int(binary.BigEndian.Uint32(data[offset:]))
				if childSize < 8 || offset+childSize > len(data) {
					break
				}
				childType := string(data[offset+4 : offset+8])
				if childType == "chap" && childSize >= 12 {
					// chap box contains track IDs (4 bytes each)
					chapterTrackID = binary.BigEndian.Uint32(data[offset+8:])
				}
				offset += childSize
			}
			return nil, nil
		default:
			return nil, nil
		}
	})

	if err != nil || chapterTrackID == 0 {
		return nil, err
	}

	// Second pass: read the chapter track's sample table
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}

	var trackInfo *chapterTrackInfo
	var currentTrackID uint32
	var inChapterTrack bool

	_, err = gomp4.ReadBoxStructure(r, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case BoxTypeMoov:
			return h.Expand()
		case BoxTypeTrak:
			// Reset for new track
			currentTrackID = 0
			inChapterTrack = false
			return h.Expand()
		case gomp4.BoxTypeTkhd():
			payload, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			if tkhd, ok := payload.(*gomp4.Tkhd); ok {
				currentTrackID = tkhd.TrackID
				if currentTrackID == chapterTrackID {
					inChapterTrack = true
					trackInfo = &chapterTrackInfo{trackID: currentTrackID}
				}
			}
			return nil, nil
		case BoxTypeMdia:
			if inChapterTrack {
				return h.Expand()
			}
			return nil, nil
		case gomp4.BoxTypeMdhd():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if mdhd, ok := payload.(*gomp4.Mdhd); ok {
					trackInfo.timescale = mdhd.Timescale
				}
			}
			return nil, nil
		case gomp4.BoxTypeMinf():
			if inChapterTrack {
				return h.Expand()
			}
			return nil, nil
		case gomp4.BoxTypeStbl():
			if inChapterTrack {
				return h.Expand()
			}
			return nil, nil
		case gomp4.BoxTypeStts():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if stts, ok := payload.(*gomp4.Stts); ok {
					for _, entry := range stts.Entries {
						for i := uint32(0); i < entry.SampleCount; i++ {
							trackInfo.sampleDeltas = append(trackInfo.sampleDeltas, entry.SampleDelta)
						}
					}
				}
			}
			return nil, nil
		case gomp4.BoxTypeStsz():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if stsz, ok := payload.(*gomp4.Stsz); ok {
					trackInfo.sampleCount = stsz.SampleCount
					if stsz.SampleSize > 0 {
						// All samples have the same size
						for i := uint32(0); i < stsz.SampleCount; i++ {
							trackInfo.sampleSizes = append(trackInfo.sampleSizes, stsz.SampleSize)
						}
					} else {
						trackInfo.sampleSizes = stsz.EntrySize
					}
				}
			}
			return nil, nil
		case gomp4.BoxTypeStsc():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if stsc, ok := payload.(*gomp4.Stsc); ok {
					for _, entry := range stsc.Entries {
						trackInfo.samplesPerChunk = append(trackInfo.samplesPerChunk, stscEntry{
							firstChunk:      entry.FirstChunk,
							samplesPerChunk: entry.SamplesPerChunk,
						})
					}
				}
			}
			return nil, nil
		case gomp4.BoxTypeStco():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if stco, ok := payload.(*gomp4.Stco); ok {
					for _, offset := range stco.ChunkOffset {
						trackInfo.chunkOffsets = append(trackInfo.chunkOffsets, uint64(offset))
					}
				}
			}
			return nil, nil
		case gomp4.BoxTypeCo64():
			if inChapterTrack && trackInfo != nil {
				payload, _, err := h.ReadPayload()
				if err != nil {
					return nil, err
				}
				if co64, ok := payload.(*gomp4.Co64); ok {
					trackInfo.chunkOffsets = co64.ChunkOffset
				}
			}
			return nil, nil
		default:
			return nil, nil
		}
	})

	if err != nil || trackInfo == nil || len(trackInfo.sampleSizes) == 0 {
		return nil, err
	}

	// Third pass: read the actual chapter text samples from mdat
	chapters := readChapterSamples(r, trackInfo, movieTimescale)
	return chapters, nil
}

// readChapterSamples reads the chapter text samples from the file.
func readChapterSamples(r io.ReadSeeker, info *chapterTrackInfo, movieTimescale uint32) []Chapter {
	if len(info.chunkOffsets) == 0 || len(info.sampleSizes) == 0 {
		return nil
	}

	timescale := info.timescale
	if timescale == 0 {
		timescale = movieTimescale
	}
	if timescale == 0 {
		timescale = 1000 // Default
	}

	var chapters []Chapter
	var currentTime uint64

	// Build sample-to-chunk mapping
	sampleOffsets := calculateSampleOffsets(info)

	for i, size := range info.sampleSizes {
		if i >= len(sampleOffsets) {
			break
		}

		offset := sampleOffsets[i]

		// Read the sample data (safe conversion - file offsets are within int64 range)
		// #nosec G115 -- offset is from file structure, within safe range
		if _, err := r.Seek(int64(offset), io.SeekStart); err != nil {
			continue
		}

		sampleData := make([]byte, size)
		if _, err := io.ReadFull(r, sampleData); err != nil {
			continue
		}

		// Parse chapter title from text sample
		title := parseTextSample(sampleData)

		// Calculate timestamp using float to avoid overflow
		startTimeSec := float64(currentTime) / float64(timescale)
		startTime := time.Duration(startTimeSec * float64(time.Second))

		chapters = append(chapters, Chapter{
			Title: title,
			Start: startTime,
		})

		// Advance time by sample delta
		if i < len(info.sampleDeltas) {
			currentTime += uint64(info.sampleDeltas[i])
		}
	}

	// Calculate end times
	for i := 0; i < len(chapters); i++ {
		if i < len(chapters)-1 {
			chapters[i].End = chapters[i+1].Start
		}
	}

	return chapters
}

// calculateSampleOffsets calculates the file offset for each sample.
func calculateSampleOffsets(info *chapterTrackInfo) []uint64 {
	if len(info.chunkOffsets) == 0 {
		return nil
	}

	offsets := make([]uint64, 0, len(info.sampleSizes))

	sampleIndex := 0
	chunkNum := uint32(0)
	for _, chunkOffset := range info.chunkOffsets {
		// Determine samples per chunk (chunk numbers are 1-based)
		chunkNum++
		samplesInChunk := uint32(1)
		for _, entry := range info.samplesPerChunk {
			if chunkNum >= entry.firstChunk {
				samplesInChunk = entry.samplesPerChunk
			}
		}

		// Add offsets for each sample in this chunk
		currentOffset := chunkOffset
		for s := uint32(0); s < samplesInChunk && sampleIndex < len(info.sampleSizes); s++ {
			offsets = append(offsets, currentOffset)
			currentOffset += uint64(info.sampleSizes[sampleIndex])
			sampleIndex++
		}
	}

	return offsets
}

// parseTextSample extracts the chapter title from a text sample.
// QuickTime text samples have format: [2 bytes length][text][optional style atoms].
func parseTextSample(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	textLen := int(binary.BigEndian.Uint16(data[0:2]))
	if textLen > len(data)-2 {
		textLen = len(data) - 2
	}

	if textLen <= 0 {
		return ""
	}

	return string(data[2 : 2+textLen])
}

// parseNeroChapterEntry parses a single Nero chapter entry.
// Format: [8 bytes timestamp][1 byte title length][title bytes].
func parseNeroChapterEntry(data []byte) (startTime int64, title string, bytesRead int) {
	if len(data) < 9 {
		return 0, "", 0
	}

	// Read as uint64 first, then convert safely
	rawTime := binary.BigEndian.Uint64(data[0:8])
	if rawTime > 1<<63-1 {
		// Overflow, treat as 0
		startTime = 0
	} else {
		startTime = int64(rawTime)
	}
	titleLen := int(data[8])
	bytesRead = 9

	if len(data) >= 9+titleLen {
		title = string(data[9 : 9+titleLen])
		bytesRead = 9 + titleLen
	}

	return startTime, title, bytesRead
}
