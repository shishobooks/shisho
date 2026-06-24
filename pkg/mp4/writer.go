package mp4

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

// WriteOptions configures the write operation.
type WriteOptions struct {
	// CreateBackup creates a .bak file before modifying
	CreateBackup bool
}

// Write updates the metadata in an M4B/MP4 file.
// This modifies the file in place. Use CreateBackup option to create a backup first.
func Write(path string, metadata *Metadata, opts WriteOptions) error {
	// Create backup if requested
	if opts.CreateBackup {
		if err := createBackup(path); err != nil {
			return errors.WithStack(err)
		}
	}

	// Read the existing file
	inputData, err := os.ReadFile(path)
	if err != nil {
		return errors.WithStack(err)
	}

	// Modify the metadata
	outputData, err := writeMetadataToBytes(inputData, metadata)
	if err != nil {
		return errors.WithStack(err)
	}

	// Write back
	if err := os.WriteFile(path, outputData, 0600); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// WriteToFile writes modified metadata to a new file (source → destination).
// Uses atomic write pattern with temp file + rename.
func WriteToFile(srcPath, destPath string, metadata *Metadata) error {
	// Read the source file
	inputData, err := os.ReadFile(srcPath)
	if err != nil {
		return errors.WithStack(err)
	}

	// Modify the metadata
	outputData, err := writeMetadataToBytes(inputData, metadata)
	if err != nil {
		return errors.WithStack(err)
	}

	// Atomic write: temp file + rename
	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, outputData, 0600); err != nil {
		return errors.WithStack(err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath) // cleanup on failure
		return errors.WithStack(err)
	}

	return nil
}

// createBackup creates a backup of the file with .bak extension.
func createBackup(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}

// writeMetadataToBytes modifies the metadata in the MP4 data and returns the
// new bytes.
//
// The moov box is rebuilt with the new metadata, which usually changes its size
// (e.g. when cover art is added). When moov sits before mdat — the "faststart"
// layout that Audible/Apple Books exports use — growing moov shifts mdat (and
// every box after moov) down by the size delta. The stco/co64 chunk offset
// tables inside moov hold absolute file offsets into mdat, so they must be
// shifted by the same delta; otherwise they keep pointing at the old positions,
// which now land inside the resized moov, and the audio becomes undecodable.
func writeMetadataToBytes(input []byte, metadata *Metadata) ([]byte, error) {
	boxes, err := topLevelBoxes(input)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var moov *topLevelBox
	firstMdatOffset := 0
	haveMdat := false
	for i := range boxes {
		switch boxes[i].typ {
		case "moov":
			moov = &boxes[i]
		case "mdat":
			if !haveMdat {
				firstMdatOffset = boxes[i].offset
				haveMdat = true
			}
		}
	}
	if moov == nil {
		return nil, errors.New("moov box not found")
	}

	// Rebuild moov with the new metadata (find and replace udta/meta/ilst).
	origContent := input[moov.offset+moov.headerSize : moov.offset+moov.size]
	moovContent := replaceIlstInContent(origContent, metadata)

	// Rebuild the QuickTime chapter text track from the chapters so the user's
	// edited titles/timings land in the track players read preferentially, not
	// only the Nero chpl box. The new text samples go into a trailing mdat
	// (appended below), leaving the audio mdat untouched; the chapter track's
	// co64 offset is patched to that mdat once its position is known.
	var chapterMdat *rebuiltChapterTrack
	if len(metadata.Chapters) > 0 {
		if rebuilt, ok := rebuildChapterTextTrack(moovContent, metadata.Chapters); ok {
			moovContent = rebuilt.moovContent
			chapterMdat = &rebuilt
		}
	}

	newMoov := buildBox("moov", moovContent)

	// Park a safe placeholder in the chapter track's co64 entry so the faststart
	// shift below cannot underflow it; the real absolute offset is written after
	// reassembly. len(input) exceeds any moov-resize delta in magnitude.
	if chapterMdat != nil {
		binary.BigEndian.PutUint64(newMoov[8+chapterMdat.co64FieldOffset:], uint64(len(input)))
	}

	// Only when moov precedes mdat does rewriting moov relocate the audio.
	if haveMdat && moov.offset < firstMdatOffset {
		delta := int64(len(newMoov)) - int64(moov.size)
		if delta != 0 {
			if err := shiftChunkOffsets(newMoov, delta); err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}

	// Reassemble the file in original box order, substituting the rebuilt moov.
	var output bytes.Buffer
	moovOutputStart := 0
	for _, b := range boxes {
		if b.offset == moov.offset {
			moovOutputStart = output.Len()
			output.Write(newMoov)
		} else {
			output.Write(input[b.offset : b.offset+b.size])
		}
	}
	outBytes := output.Bytes()

	// Append the rebuilt chapter samples as a trailing mdat and point the
	// chapter track's co64 at the sample data (after the mdat's 8-byte header).
	if chapterMdat != nil {
		// A box with a size-0 header means "extends to EOF" and is only legal as
		// the last box. Appending the chapter mdat after one would make that box
		// swallow it for strict parsers, so rewrite its size to a concrete value
		// first. (The audio chunk offsets are unaffected: only the 4-byte size
		// header changes, not any sample bytes.) Skip moov: it was rebuilt to a
		// different length than its source box, so len(outBytes)-last.size would
		// not locate its start.
		last := boxes[len(boxes)-1]
		if last.offset != moov.offset &&
			binary.BigEndian.Uint32(input[last.offset:last.offset+4]) == 0 && last.size <= math.MaxUint32 {
			lastOutputStart := len(outBytes) - last.size
			// #nosec G115 -- last.size bounded by math.MaxUint32 above
			binary.BigEndian.PutUint32(outBytes[lastOutputStart:lastOutputStart+4], uint32(last.size))
		}

		mdatBox := buildBox("mdat", chapterMdat.sampleData)
		sampleDataOffset := len(outBytes) + 8
		patchPos := moovOutputStart + 8 + chapterMdat.co64FieldOffset
		// #nosec G115 -- file offset is non-negative and within int64 range
		binary.BigEndian.PutUint64(outBytes[patchPos:patchPos+8], uint64(sampleDataOffset))
		outBytes = append(outBytes, mdatBox...)
	}

	return outBytes, nil
}

// topLevelBox describes a box at the root of the file.
type topLevelBox struct {
	typ        string
	offset     int
	size       int
	headerSize int
}

// topLevelBoxes scans the root-level box structure of an MP4 file.
func topLevelBoxes(input []byte) ([]topLevelBox, error) {
	var boxes []topLevelBox
	offset := 0
	for offset+8 <= len(input) {
		size := int(binary.BigEndian.Uint32(input[offset:]))
		headerSize := 8
		switch size {
		case 1:
			// 64-bit largesize follows the type field.
			if offset+16 > len(input) {
				return nil, errors.New("truncated 64-bit box header")
			}
			size64 := binary.BigEndian.Uint64(input[offset+8:])
			if size64 > uint64(len(input)) {
				return nil, errors.New("box size exceeds file length")
			}
			// #nosec G115 -- bounds checked against len(input) above
			size = int(size64)
			headerSize = 16
		case 0:
			// Box extends to EOF.
			size = len(input) - offset
		}
		if size < headerSize || offset+size > len(input) {
			return nil, errors.New("invalid box size")
		}
		boxes = append(boxes, topLevelBox{
			typ:        string(input[offset+4 : offset+8]),
			offset:     offset,
			size:       size,
			headerSize: headerSize,
		})
		offset += size
	}
	return boxes, nil
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
		// #nosec G115 -- chunk offsets are file positions, always within int64
		shifted := int64(binary.BigEndian.Uint64(box[pos:pos+8])) + delta
		if shifted < 0 {
			return errors.New("chunk offset underflows below zero after shift")
		}
		// #nosec G115 -- shifted is guaranteed non-negative above
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
		grouping := formatSeriesGrouping(metadata.Series, metadata.SeriesNumber)
		content.Write(buildItunesTextAtom(AtomGrouping, grouping))
		content.Write(buildFreeformAtom("com.apple.iTunes", "SERIES", metadata.Series))
		if metadata.SeriesNumber != nil {
			content.Write(buildFreeformAtom("com.apple.iTunes", "SERIES-PART", formatSeriesNumber(*metadata.SeriesNumber)))
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

// formatSeriesGrouping formats series info as a grouping string: "Series Name #N".
func formatSeriesGrouping(series string, number *float64) string {
	if series == "" {
		return ""
	}
	if number == nil {
		return series
	}
	return series + " #" + formatSeriesNumber(*number)
}

// formatSeriesNumber formats a series number as integer when whole, decimal otherwise.
func formatSeriesNumber(num float64) string {
	if num == float64(int(num)) {
		return strconv.Itoa(int(num))
	}
	return strconv.FormatFloat(num, 'f', -1, 64)
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
