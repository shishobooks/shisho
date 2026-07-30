package mp4

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/seriesnum"
)

// WriteOptions configures the write operation.
type WriteOptions struct {
	// CreateBackup creates a .bak file before modifying
	CreateBackup bool
}

// Write updates the metadata in an M4B/MP4 file.
// This modifies the file in place. Use CreateBackup option to create a backup first.
func Write(path string, metadata *Metadata, opts WriteOptions) error {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.WithStack(err)
	}
	if opts.CreateBackup {
		if err := createBackup(path); err != nil {
			return errors.WithStack(err)
		}
	}
	return writeToFileContext(context.Background(), resolvedPath, resolvedPath, metadata, nil)
}

// WriteToFile writes modified metadata to a new file using a bounded-memory,
// atomic rewrite. Unchanged top-level boxes are copied incrementally.
func WriteToFile(srcPath, destPath string, metadata *Metadata) error {
	return WriteToFileContext(context.Background(), srcPath, destPath, metadata)
}

// WriteToFileContext is WriteToFile with cancellation support. Cancellation
// interrupts incremental copying and removes the temporary destination.
func WriteToFileContext(ctx context.Context, srcPath, destPath string, metadata *Metadata) error {
	mode := os.FileMode(0600)
	return writeToFileContext(ctx, srcPath, destPath, metadata, &mode)
}

func writeToFileContext(ctx context.Context, srcPath, destPath string, metadata *Metadata, requestedMode *os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return errors.WithStack(err)
	}

	boxes, err := inspectTopLevelBoxes(ctx, src, info.Size())
	if err != nil {
		return errors.WithStack(err)
	}

	mode := info.Mode().Perm()
	if requestedMode != nil {
		mode = *requestedMode
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return errors.WithStack(err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(mode); err != nil {
		return errors.WithStack(err)
	}

	if err := rewriteToFile(ctx, src, tmp, info.Size(), boxes, metadata); err != nil {
		return errors.WithStack(err)
	}
	afterInfo, err := src.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	if afterInfo.Size() != info.Size() || !afterInfo.ModTime().Equal(info.ModTime()) {
		return errors.New("source file changed during M4B rewrite")
	}
	if err := tmp.Sync(); err != nil {
		return errors.WithStack(err)
	}
	if err := tmp.Close(); err != nil {
		return errors.WithStack(err)
	}
	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return errors.WithStack(err)
	}
	if err := syncDirectory(filepath.Dir(destPath)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// createBackup creates a backup of the file with .bak extension without
// materializing the source in memory.
func createBackup(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dest, err := os.OpenFile(path+".bak", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(dest, src, make([]byte, copyBufferSize)); err != nil {
		_ = dest.Close()
		return err
	}
	return dest.Close()
}

const (
	copyBufferSize      = 64 * 1024
	maxInMemoryMoovSize = 256 * 1024 * 1024
	maxTopLevelBoxCount = 100_000
)

type fileBox struct {
	typ        string
	offset     int64
	size       int64
	headerSize int64
	toEOF      bool
}

func inspectTopLevelBoxes(ctx context.Context, input io.ReaderAt, fileSize int64) ([]fileBox, error) {
	var boxes []fileBox
	offset := int64(0)
	for fileSize-offset >= 8 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		header := make([]byte, 16)
		if _, err := input.ReadAt(header[:8], offset); err != nil {
			return nil, err
		}

		size := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := int64(8)
		toEOF := false
		switch size {
		case 1:
			if fileSize-offset < 16 {
				return nil, errors.New("truncated 64-bit box header")
			}
			if _, err := input.ReadAt(header[8:16], offset+8); err != nil {
				return nil, err
			}
			size64 := binary.BigEndian.Uint64(header[8:16])
			if size64 > math.MaxInt64 {
				return nil, errors.New("box size exceeds supported file size")
			}
			size = int64(size64)
			headerSize = 16
		case 0:
			size = fileSize - offset
			toEOF = true
		}
		if size < headerSize || size > fileSize-offset {
			return nil, errors.New("invalid box size")
		}

		if len(boxes) >= maxTopLevelBoxCount {
			return nil, errors.Errorf("top-level box count exceeds %d-box safety limit", maxTopLevelBoxCount)
		}
		boxes = append(boxes, fileBox{
			typ:        string(header[4:8]),
			offset:     offset,
			size:       size,
			headerSize: headerSize,
			toEOF:      toEOF,
		})
		offset += size
	}
	if offset != fileSize {
		if len(boxes) >= maxTopLevelBoxCount {
			return nil, errors.Errorf("top-level box count exceeds %d-box safety limit", maxTopLevelBoxCount)
		}
		boxes = append(boxes, fileBox{offset: offset, size: fileSize - offset})
	}
	return boxes, nil
}

func rewriteToFile(ctx context.Context, src *os.File, dest io.Writer, sourceSize int64, boxes []fileBox, metadata *Metadata) error {
	if sourceSize < 0 {
		return errors.New("source file size is negative")
	}

	var moov *fileBox
	var firstMdat *fileBox
	for i := range boxes {
		switch boxes[i].typ {
		case "moov":
			moov = &boxes[i]
		case "mdat":
			if firstMdat == nil {
				firstMdat = &boxes[i]
			}
		}
	}
	if moov == nil {
		return errors.New("moov box not found")
	}
	if moov.size > maxInMemoryMoovSize {
		return errors.Errorf("moov box exceeds %d-byte safety limit", maxInMemoryMoovSize)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// #nosec G115 -- moov.size is positive and bounded by maxInMemoryMoovSize.
	moovData := make([]byte, int(moov.size))
	if _, err := src.ReadAt(moovData, moov.offset); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	origContent := moovData[moov.headerSize:]
	moovContent := replaceIlstInContent(origContent, metadata)

	var chapterMdat *rebuiltChapterTrack
	if len(metadata.Chapters) > 0 {
		if rebuilt, ok := rebuildChapterTextTrack(moovContent, metadata.Chapters); ok {
			moovContent = rebuilt.moovContent
			chapterMdat = &rebuilt
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	newMoov := buildBox("moov", moovContent)
	if int64(len(newMoov)) > maxInMemoryMoovSize {
		return errors.Errorf("rewritten moov box exceeds %d-byte safety limit", maxInMemoryMoovSize)
	}

	if chapterMdat != nil {
		// #nosec G115 -- sourceSize is checked as non-negative above.
		binary.BigEndian.PutUint64(newMoov[8+chapterMdat.co64FieldOffset:], uint64(sourceSize))
	}
	if firstMdat != nil && moov.offset < firstMdat.offset {
		delta := int64(len(newMoov)) - moov.size
		if delta != 0 {
			if err := shiftChunkOffsets(newMoov, delta); err != nil {
				return err
			}
		}
	}

	var finalBox *fileBox
	if len(boxes) > 0 {
		finalBox = &boxes[len(boxes)-1]
	}
	appendChaptersToFinalMdat := chapterMdat != nil && finalBox != nil && finalBox.toEOF && finalBox.typ == "mdat"
	promoteFinalBox := chapterMdat != nil && finalBox != nil && finalBox.toEOF &&
		finalBox.offset != moov.offset && !appendChaptersToFinalMdat && finalBox.size > math.MaxUint32

	outputSize := int64(0)
	for i, box := range boxes {
		partSize := box.size
		if box.offset == moov.offset {
			partSize = int64(len(newMoov))
		} else if promoteFinalBox && i == len(boxes)-1 {
			if partSize > math.MaxInt64-8 {
				return errors.New("promoted final box exceeds supported file size")
			}
			partSize += 8
		}
		if partSize > math.MaxInt64-outputSize {
			return errors.New("rewritten file size exceeds supported file size")
		}
		outputSize += partSize
	}
	if chapterMdat != nil {
		sampleDataOffset := outputSize
		if !appendChaptersToFinalMdat {
			if sampleDataOffset > math.MaxInt64-8 {
				return errors.New("chapter sample offset exceeds supported file size")
			}
			sampleDataOffset += 8
		}
		if int64(len(chapterMdat.sampleData)) > math.MaxInt64-sampleDataOffset {
			return errors.New("chapter samples exceed supported file size")
		}
		patchPos := 8 + chapterMdat.co64FieldOffset
		// #nosec G115 -- sampleDataOffset is non-negative and checked for overflow above.
		binary.BigEndian.PutUint64(newMoov[patchPos:patchPos+8], uint64(sampleDataOffset))
	}

	buffer := make([]byte, copyBufferSize)
	for i, box := range boxes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if box.offset == moov.offset {
			if _, err := dest.Write(newMoov); err != nil {
				return err
			}
			continue
		}

		if chapterMdat != nil && i == len(boxes)-1 && box.toEOF && !appendChaptersToFinalMdat {
			if promoteFinalBox {
				header := make([]byte, 16)
				binary.BigEndian.PutUint32(header[:4], 1)
				copy(header[4:8], box.typ)
				// #nosec G115 -- the promoted size is checked against MaxInt64 above.
				binary.BigEndian.PutUint64(header[8:16], uint64(box.size+8))
				if _, err := dest.Write(header); err != nil {
					return err
				}
				if err := copySectionContext(ctx, dest, src, box.offset+8, box.size-8, buffer); err != nil {
					return err
				}
				continue
			}

			header := make([]byte, 4)
			// #nosec G115 -- box.size is bounded by MaxUint32 when promotion is false.
			binary.BigEndian.PutUint32(header, uint32(box.size))
			if _, err := dest.Write(header); err != nil {
				return err
			}
			if err := copySectionContext(ctx, dest, src, box.offset+4, box.size-4, buffer); err != nil {
				return err
			}
			continue
		}
		if err := copySectionContext(ctx, dest, src, box.offset, box.size, buffer); err != nil {
			return err
		}
	}

	if chapterMdat != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		chapterOutput := chapterMdat.sampleData
		if !appendChaptersToFinalMdat {
			chapterOutput = buildBox("mdat", chapterOutput)
		}
		if _, err := dest.Write(chapterOutput); err != nil {
			return err
		}
	}
	return nil
}

func copySectionContext(ctx context.Context, dest io.Writer, src io.ReaderAt, offset, size int64, buffer []byte) error {
	reader := &contextReadSeeker{ctx: ctx, ReadSeeker: io.NewSectionReader(src, offset, size)}
	_, err := io.CopyBuffer(dest, reader, buffer)
	return err
}

// chunkOffsetContainers are the box types on the path to the stco/co64 chunk
// offset tables (trak → mdia → minf → stbl), plus edts which can sit between
// trak and mdia. Only these are descended into when shifting offsets.
var chunkOffsetContainers = map[string]bool{
	"trak": true,
	"mdia": true,
	"minf": true,
	"stbl": true,
	"edts": true,
}

// shiftChunkOffsets adds delta to every stco/co64 chunk offset within the given
// moov box (a complete box including its 8-byte header).
func shiftChunkOffsets(moovBox []byte, delta int64) error {
	if len(moovBox) < 8 {
		return errors.New("moov box too small")
	}
	return shiftChunkOffsetsInChildren(moovBox[8:], delta)
}

// shiftChunkOffsetsInChildren walks sibling boxes within a container's content,
// recursing into known containers and patching stco/co64 tables.
func shiftChunkOffsetsInChildren(buf []byte, delta int64) error {
	offset := 0
	for offset+8 <= len(buf) {
		size := int(binary.BigEndian.Uint32(buf[offset:]))
		headerSize := 8
		switch size {
		case 1:
			if offset+16 > len(buf) {
				return errors.New("truncated 64-bit box header in moov")
			}
			size64 := binary.BigEndian.Uint64(buf[offset+8:])
			if size64 > uint64(len(buf)) {
				return errors.New("box size exceeds moov length")
			}
			// #nosec G115 -- bounds checked against len(buf) above
			size = int(size64)
			headerSize = 16
		case 0:
			size = len(buf) - offset
		}
		if size < headerSize || offset+size > len(buf) {
			return errors.New("invalid box size in moov")
		}

		typ := string(buf[offset+4 : offset+8])
		switch {
		case typ == "stco":
			if err := shiftStco(buf[offset:offset+size], delta); err != nil {
				return err
			}
		case typ == "co64":
			if err := shiftCo64(buf[offset:offset+size], delta); err != nil {
				return err
			}
		case chunkOffsetContainers[typ]:
			if err := shiftChunkOffsetsInChildren(buf[offset+headerSize:offset+size], delta); err != nil {
				return err
			}
		}
		offset += size
	}
	return nil
}

// shiftStco adds delta to each 32-bit chunk offset in an stco box. It errors if
// any shifted offset would not fit in a uint32 (which would require promoting
// the table to 64-bit co64 entries — a rewrite this writer does not perform; it
// only happens for files approaching 4 GiB).
func shiftStco(box []byte, delta int64) error {
	// box: size(4) + type(4) + version(1) + flags(3) + count(4) + entries(4 each)
	if len(box) < 16 {
		return errors.New("stco box too small")
	}
	count := int(binary.BigEndian.Uint32(box[12:16]))
	pos := 16
	for i := 0; i < count; i++ {
		if pos+4 > len(box) {
			return errors.New("stco box truncated")
		}
		shifted := int64(binary.BigEndian.Uint32(box[pos:pos+4])) + delta
		if shifted < 0 || shifted > math.MaxUint32 {
			return errors.New("chunk offset overflows uint32 after shift; co64 promotion not supported")
		}
		// #nosec G115 -- bounds checked against [0, math.MaxUint32] above
		binary.BigEndian.PutUint32(box[pos:pos+4], uint32(shifted))
		pos += 4
	}
	return nil
}

// shiftCo64 adds delta to each 64-bit chunk offset in a co64 box.
func shiftCo64(box []byte, delta int64) error {
	// box: size(4) + type(4) + version(1) + flags(3) + count(4) + entries(8 each)
	if len(box) < 16 {
		return errors.New("co64 box too small")
	}
	count := int(binary.BigEndian.Uint32(box[12:16]))
	pos := 16
	for i := 0; i < count; i++ {
		if pos+8 > len(box) {
			return errors.New("co64 box truncated")
		}
		rawOffset := binary.BigEndian.Uint64(box[pos : pos+8])
		if rawOffset > math.MaxInt64 {
			return errors.New("chunk offset exceeds supported file size")
		}
		// #nosec G115 -- rawOffset is bounded by MaxInt64 above.
		shifted := int64(rawOffset)
		if delta > 0 && shifted > math.MaxInt64-delta {
			return errors.New("chunk offset overflows int64 after shift")
		}
		if delta == math.MinInt64 || delta < 0 && shifted < -delta {
			return errors.New("chunk offset underflows below zero after shift")
		}
		shifted += delta
		// #nosec G115 -- shifted is guaranteed non-negative above.
		binary.BigEndian.PutUint64(box[pos:pos+8], uint64(shifted))
		pos += 8
	}
	return nil
}

// replaceIlstInContent finds and replaces the ilst box within moov content.
func replaceIlstInContent(content []byte, metadata *Metadata) []byte {
	// This is a simplified implementation that looks for the ilst box
	// and replaces it with new content.
	// A full implementation would need to properly rebuild the box hierarchy.

	var result bytes.Buffer
	offset := 0

	for offset < len(content)-8 {
		size := int(binary.BigEndian.Uint32(content[offset:]))
		if size < 8 || offset+size > len(content) {
			// Invalid box, copy remaining content
			result.Write(content[offset:])
			break
		}

		boxType := string(content[offset+4 : offset+8])

		if boxType == "udta" {
			// Rebuild udta with new ilst
			newUdta := rebuildUdta(content[offset:offset+size], metadata)
			result.Write(newUdta)
		} else {
			// Copy box as-is
			result.Write(content[offset : offset+size])
		}

		offset += size
	}

	return result.Bytes()
}

// rebuildUdta rebuilds the udta box with new metadata.
func rebuildUdta(udtaBox []byte, metadata *Metadata) []byte {
	if len(udtaBox) < 8 {
		return udtaBox
	}

	// Skip udta header
	content := udtaBox[8:]
	var newContent bytes.Buffer
	offset := 0
	foundChpl := false

	for offset < len(content)-8 {
		size := int(binary.BigEndian.Uint32(content[offset:]))
		if size < 8 || offset+size > len(content) {
			newContent.Write(content[offset:])
			break
		}

		boxType := string(content[offset+4 : offset+8])

		switch boxType {
		case "meta":
			// Rebuild meta with new ilst
			newMeta := rebuildMeta(content[offset:offset+size], metadata)
			newContent.Write(newMeta)
		case "chpl":
			// Replace chpl (chapters) box if we have chapters to write
			if len(metadata.Chapters) > 0 {
				newChpl := buildChpl(metadata.Chapters)
				newContent.Write(newChpl)
			}
			// If no chapters, omit the chpl box entirely
			foundChpl = true
		default:
			newContent.Write(content[offset : offset+size])
		}

		offset += size
	}

	// Add chpl box if we have chapters and didn't find an existing one
	if !foundChpl && len(metadata.Chapters) > 0 {
		newChpl := buildChpl(metadata.Chapters)
		newContent.Write(newChpl)
	}

	return buildBox("udta", newContent.Bytes())
}

// rebuildMeta rebuilds the meta box with new metadata.
func rebuildMeta(metaBox []byte, metadata *Metadata) []byte {
	if len(metaBox) < 12 {
		return metaBox
	}

	// Meta box has 4 bytes of version/flags after header
	versionFlags := metaBox[8:12]
	content := metaBox[12:]

	var newContent bytes.Buffer
	newContent.Write(versionFlags)

	offset := 0
	foundIlst := false

	for offset < len(content)-8 {
		size := int(binary.BigEndian.Uint32(content[offset:]))
		if size < 8 || offset+size > len(content) {
			newContent.Write(content[offset:])
			break
		}

		boxType := string(content[offset+4 : offset+8])

		if boxType == "ilst" {
			// Build new ilst
			newIlst := buildIlst(metadata)
			newContent.Write(newIlst)
			foundIlst = true
		} else {
			newContent.Write(content[offset : offset+size])
		}

		offset += size
	}

	// If no ilst was found, add one
	if !foundIlst {
		newIlst := buildIlst(metadata)
		newContent.Write(newIlst)
	}

	return buildBox("meta", newContent.Bytes())
}

// buildIlst builds an ilst box from metadata.
func buildIlst(metadata *Metadata) []byte {
	var content bytes.Buffer

	// Title
	if metadata.Title != "" {
		content.Write(buildItunesTextAtom(AtomTitle, metadata.Title))
	}

	// Artist (authors)
	if len(metadata.Authors) > 0 {
		content.Write(buildItunesTextAtom(AtomArtist, joinAuthorNames(metadata.Authors)))
	}

	// Album: always the book title. Audiobook players (Bound, Overcast, etc.)
	// commonly use Album as the canonical book-title atom; leaving it empty or
	// putting series info here makes those players show "Unknown". Series info
	// goes into ©grp and the Audible-style SERIES / SERIES-PART freeforms
	// below instead.
	if metadata.Album != "" {
		content.Write(buildItunesTextAtom(AtomAlbum, metadata.Album))
	}

	// Series info: write to ©grp (legacy/compatibility) and to the Audible-
	// style freeform atoms com.apple.iTunes:SERIES + SERIES-PART (preferred
	// modern source, used by Audible, Tone, Audiobookshelf).
	//
	// When metadata.Series is empty, we emit none of these atoms. Any ©grp
	// from the source file is dropped on regeneration because ©grp is not
	// round-tripped through Metadata (no Grouping field) — series data is
	// expected to live in the DB, not in file tags.
	if metadata.Series != "" {
		seriesPart, hasSeriesPart := formatSeriesRange(metadata.SeriesNumber, metadata.SeriesNumberEnd)
		grouping := metadata.Series
		if hasSeriesPart {
			grouping += " #" + seriesPart
		}
		content.Write(buildItunesTextAtom(AtomGrouping, grouping))
		content.Write(buildFreeformAtom("com.apple.iTunes", "SERIES", metadata.Series))
		if hasSeriesPart {
			content.Write(buildFreeformAtom("com.apple.iTunes", "SERIES-PART", seriesPart))
		}
	}

	// Narrators: write to both ©nrt (dedicated narrator) and ©cmp (composer) for compatibility
	if len(metadata.Narrators) > 0 {
		narratorStr := joinStrings(metadata.Narrators)
		content.Write(buildItunesTextAtom(AtomNarrator, narratorStr))
		content.Write(buildItunesTextAtom(AtomComposer, narratorStr))
	}

	// Genre
	if metadata.Genre != "" {
		content.Write(buildItunesTextAtom(AtomGenre, metadata.Genre))
	}

	// Description
	if metadata.Description != "" {
		content.Write(buildItunesTextAtom(AtomDescription, metadata.Description))
	}

	// Subtitle as freeform atom
	if metadata.Subtitle != "" {
		content.Write(buildFreeformAtom("com.apple.iTunes", "SUBTITLE", metadata.Subtitle))
	}

	// Tags as freeform atom (comma-separated)
	if len(metadata.Tags) > 0 {
		content.Write(buildFreeformAtom("com.shisho", "tags", joinStrings(metadata.Tags)))
	}

	// ASIN as freeform atom (from identifiers)
	for _, id := range metadata.Identifiers {
		if id.Type == "asin" && id.Value != "" {
			content.Write(buildFreeformAtom("com.apple.iTunes", "ASIN", id.Value))
			break // Only write first ASIN
		}
	}

	// Write any remaining freeform atoms from the Freeform map. This preserves
	// atoms that aren't explicitly handled above (e.g., com.pilabor.tone:LANGUAGE,
	// com.pilabor.tone:ABRIDGED) plus anything carried over from the source file
	// via src.Freeform. com.apple.iTunes:SERIES/SERIES-PART also flow through here
	// when metadata.Series is empty (i.e., no series in the DB).
	//
	// Keys that are already written by an explicit branch above are skipped to
	// avoid duplicate atoms. SERIES/SERIES-PART are only excluded when
	// metadata.Series is set (the Series branch already wrote them); when
	// metadata.Series is empty, we allow passthrough so existing freeform values
	// from the source file survive the round-trip.
	explicitFreeformKeys := map[string]bool{
		"com.apple.iTunes:SUBTITLE":     true,
		"com.pilabor.tone:SUBTITLE":     true,
		"com.shisho:tags":               true,
		"com.apple.iTunes:ASIN":         true,
		"com.pilabor.tone:AUDIBLE_ASIN": true,
	}
	if metadata.Series != "" {
		explicitFreeformKeys["com.apple.iTunes:SERIES"] = true
		explicitFreeformKeys["com.apple.iTunes:SERIES-PART"] = true
	}
	for key, value := range metadata.Freeform {
		if value == "" || explicitFreeformKeys[key] {
			continue
		}
		namespace, name, ok := splitFreeformKey(key)
		if !ok {
			continue
		}
		content.Write(buildFreeformAtom(namespace, name, value))
	}

	// Cover
	if len(metadata.CoverData) > 0 {
		dataType := DataTypeJPEG
		if metadata.CoverMimeType == "image/png" {
			dataType = DataTypePNG
		}
		content.Write(buildItunesDataAtom(AtomCover, dataType, metadata.CoverData))
	}

	// Comment
	if metadata.Comment != "" {
		content.Write(buildItunesTextAtom(AtomComment, metadata.Comment))
	}

	// Year
	if metadata.Year != "" {
		content.Write(buildItunesTextAtom(AtomYear, metadata.Year))
	}

	// Copyright
	if metadata.Copyright != "" {
		content.Write(buildItunesTextAtom(AtomCopyright, metadata.Copyright))
	}

	// Encoder
	if metadata.Encoder != "" {
		content.Write(buildItunesTextAtom(AtomEncoder, metadata.Encoder))
	}

	// Media Type (stik) - audiobook = 2
	if metadata.MediaType > 0 {
		content.Write(buildItunesDataAtom(AtomMediaType, DataTypeInteger, []byte{byte(metadata.MediaType)}))
	}

	// Write preserved unknown atoms
	for _, atom := range metadata.UnknownAtoms {
		content.Write(atom.Data)
	}

	return buildBox("ilst", content.Bytes())
}

// buildChpl builds a Nero-format chapter list (chpl) box.
// Format: [version 1 byte][flags 3 bytes][reserved 4 bytes (v0) or 1 byte (v1)]
//
//	[chapter count 4 bytes (v0) or 1 byte (v1)]
//	For each chapter: [timestamp 8 bytes in 100ns units][title length 1 byte][title bytes]
func buildChpl(chapters []Chapter) []byte {
	if len(chapters) == 0 {
		return nil
	}

	var content bytes.Buffer

	// Version 0 format (more compatible)
	content.WriteByte(0)              // version
	content.Write([]byte{0, 0, 0})    // flags (3 bytes)
	content.Write([]byte{0, 0, 0, 0}) // reserved (4 bytes for version 0)

	// Chapter count (4 bytes for version 0)
	// #nosec G115 -- chapter count is bounded by practical limits
	chapterCount := uint32(len(chapters))
	_ = binary.Write(&content, binary.BigEndian, chapterCount)

	// Write each chapter
	for _, ch := range chapters {
		// Timestamp in 100-nanosecond units
		// time.Duration is in nanoseconds, so divide by 100
		// #nosec G115 -- nanoseconds/100 fits in uint64 for any practical duration
		timestamp := uint64(ch.Start.Nanoseconds() / 100)
		_ = binary.Write(&content, binary.BigEndian, timestamp)

		// Title length (1 byte) and title
		title := ch.Title
		titleLen := len(title)
		if titleLen > 255 {
			title = title[:255]
			titleLen = 255
		}
		content.WriteByte(byte(titleLen))
		content.WriteString(title)
	}

	return buildBox("chpl", content.Bytes())
}

// buildItunesTextAtom builds a text-based iTunes atom.
func buildItunesTextAtom(atomType [4]byte, value string) []byte {
	return buildItunesDataAtom(atomType, DataTypeUTF8, []byte(value))
}

// buildFreeformAtom builds a freeform (----) atom with mean, name, and data boxes.
// This is used for custom metadata like ----:com.apple.iTunes:SUBTITLE.
func buildFreeformAtom(namespace, name, value string) []byte {
	var content bytes.Buffer

	// Build mean box: [size][mean][version/flags (4 bytes)][namespace string]
	meanContent := make([]byte, 4+len(namespace))
	// First 4 bytes are version/flags (all zeros)
	copy(meanContent[4:], namespace)
	content.Write(buildBox("mean", meanContent))

	// Build name box: [size][name][version/flags (4 bytes)][name string]
	nameContent := make([]byte, 4+len(name))
	// First 4 bytes are version/flags (all zeros)
	copy(nameContent[4:], name)
	content.Write(buildBox("name", nameContent))

	// Build data box with UTF-8 text
	var dataContent bytes.Buffer
	dataContent.WriteByte(0)                  // version
	dataContent.WriteByte(0)                  // type byte 1
	dataContent.WriteByte(0)                  // type byte 2
	dataContent.WriteByte(byte(DataTypeUTF8)) // type byte 3 (UTF-8)
	dataContent.Write([]byte{0, 0, 0, 0})     // locale
	dataContent.Write([]byte(value))
	content.Write(buildBox("data", dataContent.Bytes()))

	return buildBoxWithType(AtomFreeform, content.Bytes())
}

// splitFreeformKey splits a freeform atom key of the form "namespace:name"
// into its parts. Splits on the LAST ":" to handle namespaces like
// "com.apple.iTunes" that themselves contain dots but not colons.
func splitFreeformKey(key string) (namespace, name string, ok bool) {
	idx := strings.LastIndex(key, ":")
	if idx <= 0 || idx == len(key)-1 {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}

// formatSeriesRange formats a valid series number group for embedded metadata.
func formatSeriesRange(number, numberEnd *float64) (string, bool) {
	if number == nil {
		return "", false
	}
	formatted := seriesnum.FormatRange(*number, numberEnd)
	_, _, ok := seriesnum.ParseRange(formatted)
	return formatted, ok
}

// buildItunesDataAtom builds an iTunes atom with a data box.
func buildItunesDataAtom(atomType [4]byte, dataType int, value []byte) []byte {
	// Build data box content: [version 1 byte][type 3 bytes][locale 4 bytes][data]
	var dataContent bytes.Buffer
	dataContent.WriteByte(0)                             // version
	dataContent.WriteByte(byte((dataType >> 16) & 0xFF)) // type byte 1
	dataContent.WriteByte(byte((dataType >> 8) & 0xFF))  // type byte 2
	dataContent.WriteByte(byte(dataType & 0xFF))         // type byte 3
	dataContent.Write([]byte{0, 0, 0, 0})                // locale
	dataContent.Write(value)

	// Build data box
	dataBox := buildBox("data", dataContent.Bytes())

	// Build atom box
	var atomContent bytes.Buffer
	atomContent.Write(dataBox)

	return buildBoxWithType(atomType, atomContent.Bytes())
}

// buildBox builds a box with standard 4-byte type.
func buildBox(boxType string, content []byte) []byte {
	contentLen := len(content)
	// Clamp to max safe size to avoid overflow (box size uses uint32).
	const maxSize = 1<<31 - 9 // Max content size that fits in uint32 with 8 byte header
	if contentLen > maxSize {
		contentLen = maxSize
	}
	// #nosec G115 -- contentLen is clamped above to prevent overflow
	size := uint32(8 + contentLen)

	buf := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], content)
	return buf
}

// buildBoxWithType builds a box with a 4-byte array type.
func buildBoxWithType(boxType [4]byte, content []byte) []byte {
	contentLen := len(content)
	// Clamp to max safe size to avoid overflow (box size uses uint32).
	const maxSize = 1<<31 - 9 // Max content size that fits in uint32 with 8 byte header
	if contentLen > maxSize {
		contentLen = maxSize
	}
	// #nosec G115 -- contentLen is clamped above to prevent overflow
	size := uint32(8 + contentLen)

	buf := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType[:])
	copy(buf[8:], content)
	return buf
}

// joinStrings joins strings with comma separator.
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	var buf bytes.Buffer
	buf.WriteString(strs[0])
	for i := 1; i < len(strs); i++ {
		buf.WriteString(", ")
		buf.WriteString(strs[i])
	}
	return buf.String()
}

// joinAuthorNames joins author names from ParsedAuthor slice with comma separator.
func joinAuthorNames(authors []mediafile.ParsedAuthor) string {
	if len(authors) == 0 {
		return ""
	}
	if len(authors) == 1 {
		return authors[0].Name
	}
	var buf bytes.Buffer
	buf.WriteString(authors[0].Name)
	for i := 1; i < len(authors); i++ {
		buf.WriteString(", ")
		buf.WriteString(authors[i].Name)
	}
	return buf.String()
}
