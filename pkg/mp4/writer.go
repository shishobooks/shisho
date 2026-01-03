package mp4

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"

	gomp4 "github.com/abema/go-mp4"
	"github.com/pkg/errors"
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

// createBackup creates a backup of the file with .bak extension.
func createBackup(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}

// writeMetadataToBytes modifies the metadata in the MP4 data and returns the new bytes.
func writeMetadataToBytes(input []byte, metadata *Metadata) ([]byte, error) {
	r := bytes.NewReader(input)
	var output bytes.Buffer

	// Track whether we've written the moov box
	moovWritten := false

	_, err := gomp4.ReadBoxStructure(r, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case BoxTypeMoov:
			// Rebuild moov with new metadata
			moovBytes, err := rebuildMoov(r, h, metadata)
			if err != nil {
				return nil, err
			}
			output.Write(moovBytes)
			moovWritten = true
			return nil, nil

		default:
			// Copy the box as-is
			boxBytes := input[h.BoxInfo.Offset : h.BoxInfo.Offset+h.BoxInfo.Size]
			output.Write(boxBytes)
			return nil, nil
		}
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !moovWritten {
		return nil, errors.New("moov box not found")
	}

	return output.Bytes(), nil
}

// rebuildMoov rebuilds the moov box with new metadata.
func rebuildMoov(r *bytes.Reader, h *gomp4.ReadHandle, metadata *Metadata) ([]byte, error) {
	// Read the original moov content
	origStart := h.BoxInfo.Offset + h.BoxInfo.HeaderSize
	origEnd := h.BoxInfo.Offset + h.BoxInfo.Size

	// Seek to moov content (safe conversion as file sizes are within int64 range)
	if origStart > 1<<62 {
		return nil, errors.New("file offset too large")
	}
	if _, err := r.Seek(int64(origStart), io.SeekStart); err != nil {
		return nil, err
	}

	origContent := make([]byte, origEnd-origStart)
	if _, err := io.ReadFull(r, origContent); err != nil {
		return nil, err
	}

	// Find and replace udta/meta/ilst within the moov content
	newContent := replaceIlstInContent(origContent, metadata)

	// Build new moov box
	return buildBox("moov", newContent), nil
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

	for offset < len(content)-8 {
		size := int(binary.BigEndian.Uint32(content[offset:]))
		if size < 8 || offset+size > len(content) {
			newContent.Write(content[offset:])
			break
		}

		boxType := string(content[offset+4 : offset+8])

		if boxType == "meta" {
			// Rebuild meta with new ilst
			newMeta := rebuildMeta(content[offset:offset+size], metadata)
			newContent.Write(newMeta)
		} else {
			newContent.Write(content[offset : offset+size])
		}

		offset += size
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
		content.Write(buildItunesTextAtom(AtomArtist, joinStrings(metadata.Authors)))
	}

	// Album
	if metadata.Album != "" {
		content.Write(buildItunesTextAtom(AtomAlbum, metadata.Album))
	}

	// Composer (narrators)
	if len(metadata.Narrators) > 0 {
		content.Write(buildItunesTextAtom(AtomComposer, joinStrings(metadata.Narrators)))
	}

	// Genre
	if metadata.Genre != "" {
		content.Write(buildItunesTextAtom(AtomGenre, metadata.Genre))
	}

	// Description
	if metadata.Description != "" {
		content.Write(buildItunesTextAtom(AtomDescription, metadata.Description))
	}

	// Cover
	if len(metadata.CoverData) > 0 {
		dataType := DataTypeJPEG
		if metadata.CoverMimeType == "image/png" {
			dataType = DataTypePNG
		}
		content.Write(buildItunesDataAtom(AtomCover, dataType, metadata.CoverData))
	}

	return buildBox("ilst", content.Bytes())
}

// buildItunesTextAtom builds a text-based iTunes atom.
func buildItunesTextAtom(atomType [4]byte, value string) []byte {
	return buildItunesDataAtom(atomType, DataTypeUTF8, []byte(value))
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
