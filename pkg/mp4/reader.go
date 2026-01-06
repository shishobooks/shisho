package mp4

import (
	"bytes"
	"io"
	"os"
	"strings"

	gomp4 "github.com/abema/go-mp4"
	"github.com/pkg/errors"
)

// rawMetadata holds the raw extracted metadata from the MP4 file
// before post-processing (like series parsing).
type rawMetadata struct {
	title        string
	artist       string
	album        string
	narrator     string // from ©nrt (dedicated narrator atom)
	composer     string // from ©cmp
	writer       string // from ©wrt (ffmpeg uses this for composer)
	genre        string
	description  string
	comment      string // from ©cmt
	year         string // from ©day
	copyright    string // from ©cpy
	encoder      string // from ©too
	coverData    []byte
	coverMime    string
	mediaType    int64
	timescale    uint32            // from mvhd - units per second
	duration     uint64            // from mvhd - in timescale units
	avgBitrate   uint32            // from esds - average bitrate in bps
	freeform     map[string]string // freeform (----) atoms like com.apple.iTunes:ASIN
	chapters     []Chapter         // chapter list
	unknownAtoms []RawAtom         // unrecognized atoms to preserve
}

// readMetadata reads metadata from an MP4 file using go-mp4.
func readMetadata(path string) (*rawMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	meta, err := readMetadataFromReader(f)
	if err != nil {
		return nil, err
	}

	// Second pass: read chapters
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}
	chapters, _ := readChapters(f)
	meta.chapters = chapters

	return meta, nil
}

// readMetadataFromReader reads metadata from an io.ReadSeeker.
func readMetadataFromReader(r io.ReadSeeker) (*rawMetadata, error) {
	meta := &rawMetadata{}

	// Read the box structure looking for moov/udta/meta/ilst and audio track info
	_, err := gomp4.ReadBoxStructure(r, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case BoxTypeMoov:
			// Descend into moov
			return h.Expand()

		case BoxTypeMvhd:
			// Read movie header for duration info
			return processMvhd(h, meta)

		case BoxTypeTrak:
			// Descend into trak to find audio track
			return h.Expand()

		case BoxTypeMdia:
			// Descend into mdia
			return h.Expand()

		case BoxTypeMinf:
			// Descend into minf
			return h.Expand()

		case BoxTypeStbl:
			// Descend into stbl
			return h.Expand()

		case BoxTypeStsd:
			// Descend into stsd (sample description)
			return h.Expand()

		case BoxTypeMp4a:
			// Descend into mp4a (MPEG-4 audio)
			return h.Expand()

		case BoxTypeEsds:
			// Read esds for bitrate info
			return processEsds(h, meta)

		case BoxTypeUdta:
			// Descend into udta
			return h.Expand()

		case BoxTypeMeta:
			// Descend into meta
			return h.Expand()

		case BoxTypeIlst:
			// Found the item list - expand and process children
			return h.Expand()

		default:
			// Check if this is a metadata atom (child of ilst)
			// Process ALL potential metadata atoms - known ones are parsed,
			// unknown ones are preserved as raw atoms
			if isPotentialMetadataAtom(h.BoxInfo.Type) {
				return processMetadataBox(h, meta)
			}
			return nil, nil
		}
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return meta, nil
}

// processEsds reads the elementary stream descriptor for bitrate info.
func processEsds(h *gomp4.ReadHandle, meta *rawMetadata) (interface{}, error) {
	payload, _, err := h.ReadPayload()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	esds, ok := payload.(*gomp4.Esds)
	if !ok {
		return nil, nil
	}

	// Extract average bitrate from the descriptor
	for _, desc := range esds.Descriptors {
		if desc.DecoderConfigDescriptor != nil {
			meta.avgBitrate = desc.DecoderConfigDescriptor.AvgBitrate
			break
		}
	}

	return nil, nil
}

// isMetadataAtom checks if a box type is a known metadata atom.
func isMetadataAtom(boxType gomp4.BoxType) bool {
	return atomTypeEquals(boxType, AtomTitle) ||
		atomTypeEquals(boxType, AtomArtist) ||
		atomTypeEquals(boxType, AtomAlbum) ||
		atomTypeEquals(boxType, AtomNarrator) ||
		atomTypeEquals(boxType, AtomComposer) ||
		atomTypeEquals(boxType, AtomGenre) ||
		atomTypeEquals(boxType, AtomGenreID) ||
		atomTypeEquals(boxType, AtomCover) ||
		atomTypeEquals(boxType, AtomDescription) ||
		atomTypeEquals(boxType, AtomMediaType) ||
		atomTypeEquals(boxType, AtomWriter) ||
		atomTypeEquals(boxType, AtomGrouping) ||
		atomTypeEquals(boxType, AtomComment) ||
		atomTypeEquals(boxType, AtomYear) ||
		atomTypeEquals(boxType, AtomCopyright) ||
		atomTypeEquals(boxType, AtomEncoder) ||
		atomTypeEquals(boxType, AtomFreeform)
}

// isPotentialMetadataAtom checks if a box type could be an ilst metadata atom.
// This is more permissive than isMetadataAtom and includes any box that could
// be inside an ilst container. Returns true for:
// - Known metadata atoms
// - Atoms starting with © (0xA9) - iTunes metadata convention
// - Freeform (----) atoms
// - Common unknown atoms like cprt, aART, etc.
func isPotentialMetadataAtom(boxType gomp4.BoxType) bool {
	// Known atoms
	if isMetadataAtom(boxType) {
		return true
	}

	// Atoms starting with © (0xA9) are iTunes metadata convention
	if boxType[0] == 0xA9 {
		return true
	}

	// aART (album artist) is a common metadata atom
	if boxType == [4]byte{'a', 'A', 'R', 'T'} {
		return true
	}

	// cprt (copyright) is also common
	if boxType == [4]byte{'c', 'p', 'r', 't'} {
		return true
	}

	return false
}

// processMvhd reads the movie header box to extract duration info.
func processMvhd(h *gomp4.ReadHandle, meta *rawMetadata) (interface{}, error) {
	// Read the mvhd box payload
	payload, _, err := h.ReadPayload()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	mvhd, ok := payload.(*gomp4.Mvhd)
	if !ok {
		return nil, nil
	}

	meta.timescale = mvhd.Timescale
	if mvhd.Version == 0 {
		meta.duration = uint64(mvhd.DurationV0)
	} else {
		meta.duration = mvhd.DurationV1
	}

	return nil, nil
}

// processMetadataBox reads and processes a metadata atom box.
func processMetadataBox(h *gomp4.ReadHandle, meta *rawMetadata) (interface{}, error) {
	// Read the box data using ReadData
	var buf bytes.Buffer
	_, err := h.ReadData(&buf)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	data := buf.Bytes()
	boxType := h.BoxInfo.Type

	// Handle freeform (----) atoms specially
	if atomTypeEquals(boxType, AtomFreeform) {
		processFreeformAtom(data, meta)
		return nil, nil
	}

	// Check if this is a known metadata atom
	if isMetadataAtom(boxType) {
		// The data should contain a "data" box
		dataContent := extractDataBoxContent(data)
		if dataContent == nil {
			return nil, nil
		}
		processMetadataAtom(ilstChild{atomType: boxType, data: dataContent}, meta)
	} else {
		// Unknown atom - preserve it as raw data
		// Rebuild the full atom with header
		fullAtom := buildBoxWithType(boxType, data)
		meta.unknownAtoms = append(meta.unknownAtoms, RawAtom{
			Type: boxType,
			Data: fullAtom,
		})
	}

	return nil, nil
}

// processFreeformAtom parses a freeform (----) atom.
// Structure: [mean box][name box][data box].
func processFreeformAtom(data []byte, meta *rawMetadata) {
	// Initialize freeform map if needed
	if meta.freeform == nil {
		meta.freeform = make(map[string]string)
	}

	var mean, name string
	var dataContent []byte

	offset := 0
	for offset < len(data)-8 {
		size := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		if size < 8 || offset+size > len(data) {
			break
		}

		boxType := string(data[offset+4 : offset+8])
		boxContent := data[offset+8 : offset+size]

		switch boxType {
		case "mean":
			// Skip version/flags (4 bytes)
			if len(boxContent) > 4 {
				mean = string(boxContent[4:])
			}
		case "name":
			// Skip version/flags (4 bytes)
			if len(boxContent) > 4 {
				name = string(boxContent[4:])
			}
		case "data":
			dataContent = boxContent
		}

		offset += size
	}

	// Only store if we have both mean and name
	if mean != "" && name != "" && len(dataContent) > 0 {
		value := parseTextData(dataContent)
		key := mean + ":" + name
		meta.freeform[key] = value
	}
}

// ilstChild represents a child atom in the ilst.
type ilstChild struct {
	atomType gomp4.BoxType
	data     []byte
}

// extractDataBoxContent extracts the content of the "data" box from an atom's content.
func extractDataBoxContent(content []byte) []byte {
	if len(content) < 16 {
		return nil
	}

	// Look for "data" box: [4 bytes size][4 bytes "data"][...content...]
	// The "data" string should be at bytes 4-7
	if content[4] == 'd' && content[5] == 'a' && content[6] == 't' && content[7] == 'a' {
		// Size is in first 4 bytes (big-endian)
		// Return everything after the data box header (8 bytes)
		return content[8:]
	}

	return nil
}

// processMetadataAtom processes a single metadata atom and updates rawMetadata.
func processMetadataAtom(child ilstChild, meta *rawMetadata) {
	if len(child.data) == 0 {
		return
	}

	boxType := child.atomType

	switch {
	case atomTypeEquals(boxType, AtomTitle):
		meta.title = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomArtist):
		meta.artist = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomAlbum):
		meta.album = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomNarrator):
		meta.narrator = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomComposer):
		meta.composer = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomGenre):
		meta.genre = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomGenreID):
		// gnre is a genre ID (ID3v1 index), convert to string
		if id, ok := parseIntegerData(child.data); ok {
			meta.genre = genreIDToString(int(id))
		}

	case atomTypeEquals(boxType, AtomCover):
		if data, mime, ok := parseImageData(child.data); ok {
			meta.coverData = data
			meta.coverMime = mime
		}

	case atomTypeEquals(boxType, AtomWriter):
		meta.writer = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomDescription):
		meta.description = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomMediaType):
		if id, ok := parseIntegerData(child.data); ok {
			meta.mediaType = id
		}

	case atomTypeEquals(boxType, AtomComment):
		meta.comment = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomYear):
		meta.year = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomCopyright):
		meta.copyright = parseTextData(child.data)

	case atomTypeEquals(boxType, AtomEncoder):
		meta.encoder = parseTextData(child.data)
	}
}

// splitMultiValue splits a comma-separated string into individual values.
func splitMultiValue(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// genreIDToString converts an ID3v1 genre ID to its string representation.
// The ID is 1-based in MP4 (ID3v1 is 0-based, so we subtract 1).
func genreIDToString(id int) string {
	// ID3v1 genres (0-based in the spec, but 1-based in MP4)
	genres := []string{
		"Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge",
		"Hip-Hop", "Jazz", "Metal", "New Age", "Oldies", "Other", "Pop", "R&B",
		"Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative", "Ska",
		"Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient",
		"Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance", "Classical",
		"Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel",
		"Noise", "AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative",
		"Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic", "Darkwave",
		"Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream",
		"Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40", "Christian Rap",
		"Pop/Funk", "Jungle", "Native American", "Cabaret", "New Wave",
		"Psychadelic", "Rave", "Showtunes", "Trailer", "Lo-Fi", "Tribal",
		"Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical", "Rock & Roll",
		"Hard Rock", "Folk", "Folk-Rock", "National Folk", "Swing", "Fast Fusion",
		"Bebob", "Latin", "Revival", "Celtic", "Bluegrass", "Avantgarde",
		"Gothic Rock", "Progressive Rock", "Psychedelic Rock", "Symphonic Rock",
		"Slow Rock", "Big Band", "Chorus", "Easy Listening", "Acoustic",
		"Humour", "Speech", "Chanson", "Opera", "Chamber Music", "Sonata",
		"Symphony", "Booty Bass", "Primus", "Porn Groove", "Satire", "Slow Jam",
		"Club", "Tango", "Samba", "Folklore", "Ballad", "Power Ballad",
		"Rhythmic Soul", "Freestyle", "Duet", "Punk Rock", "Drum Solo",
		"A capella", "Euro-House", "Dance Hall", "Audiobook", "Audio Theatre",
	}

	// MP4 uses 1-based indexing
	idx := id - 1
	if idx >= 0 && idx < len(genres) {
		return genres[idx]
	}
	return ""
}
