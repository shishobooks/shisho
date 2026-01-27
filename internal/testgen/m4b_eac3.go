package testgen

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// M4BEAC3Options configures the synthetic EAC3 M4B file.
type M4BEAC3Options struct {
	Title   string
	Bitrate uint32 // Average bitrate in bps (default: 640000)
}

// GenerateM4BWithEAC3 creates a minimal valid M4B file with EAC3 (Dolby Digital Plus) audio.
// This is a synthetic file that contains the necessary box structure for codec detection
// but does not contain playable audio data.
func GenerateM4BWithEAC3(t *testing.T, dir, filename string, opts M4BEAC3Options) string {
	t.Helper()

	path := filepath.Join(dir, filename)

	// Set defaults
	bitrate := opts.Bitrate
	if bitrate == 0 {
		bitrate = 640000 // 640 kbps default
	}

	// Build minimal MP4 structure:
	// ftyp (file type)
	// moov (movie container)
	//   mvhd (movie header)
	//   trak (track)
	//     tkhd (track header)
	//     mdia (media)
	//       mdhd (media header)
	//       hdlr (handler reference)
	//       minf (media info)
	//         smhd (sound media header)
	//         dinf (data info)
	//           dref (data reference)
	//         stbl (sample table)
	//           stsd (sample description) - contains ec-3 box
	//           stts (time to sample)
	//           stsc (sample to chunk)
	//           stsz (sample sizes)
	//           stco (chunk offsets)
	//   udta (user data)
	//     meta (metadata)
	//       hdlr
	//       ilst (iTunes metadata)

	var data []byte

	// ftyp box
	ftyp := buildBox("ftyp", []byte("M4A \x00\x00\x00\x00M4A mp42isom"))
	data = append(data, ftyp...)

	// Build moov box content
	var moovContent []byte

	// mvhd (movie header) - 108 bytes for version 0
	mvhd := buildFullBox("mvhd", 0, 0, buildMvhdContent())
	moovContent = append(moovContent, mvhd...)

	// trak
	trak := buildBox("trak", buildTrakContent(bitrate))
	moovContent = append(moovContent, trak...)

	// udta with metadata
	if opts.Title != "" {
		udta := buildUdtaWithTitle(opts.Title)
		moovContent = append(moovContent, udta...)
	}

	// moov box
	moov := buildBox("moov", moovContent)
	data = append(data, moov...)

	// Write file
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write EAC3 M4B file: %v", err)
	}

	return path
}

// buildBox creates an MP4 box with the given type and content.
func buildBox(boxType string, content []byte) []byte {
	size := 8 + len(content)
	box := make([]byte, size)
	binary.BigEndian.PutUint32(box[0:4], uint32(size)) //nolint:gosec // size is always small for test files
	copy(box[4:8], boxType)
	copy(box[8:], content)
	return box
}

// buildFullBox creates an MP4 full box with version and flags.
func buildFullBox(boxType string, _ uint8, flags uint32, content []byte) []byte {
	fullContent := make([]byte, 4+len(content))
	fullContent[0] = 0 // version is always 0 for our test files
	fullContent[1] = byte((flags >> 16) & 0xff)
	fullContent[2] = byte((flags >> 8) & 0xff)
	fullContent[3] = byte(flags & 0xff)
	copy(fullContent[4:], content)
	return buildBox(boxType, fullContent)
}

// buildMvhdContent creates movie header content (version 0).
func buildMvhdContent() []byte {
	content := make([]byte, 96) // mvhd v0 is 96 bytes (excluding version/flags)
	// creation_time, modification_time (8 bytes each for v0 = 0)
	// timescale at offset 12
	binary.BigEndian.PutUint32(content[12:16], 48000) // 48kHz timescale
	// duration at offset 16
	binary.BigEndian.PutUint32(content[16:20], 48000) // 1 second duration
	// rate at offset 20 (1.0 = 0x00010000)
	binary.BigEndian.PutUint32(content[20:24], 0x00010000)
	// volume at offset 24 (1.0 = 0x0100)
	binary.BigEndian.PutUint16(content[24:26], 0x0100)
	// reserved, matrix, pre_defined... (fill with standard values)
	// matrix at offset 36: identity matrix
	binary.BigEndian.PutUint32(content[36:40], 0x00010000) // a
	binary.BigEndian.PutUint32(content[48:52], 0x00010000) // d
	binary.BigEndian.PutUint32(content[60:64], 0x40000000) // w
	binary.BigEndian.PutUint32(content[92:96], 2)          // next_track_ID
	return content
}

// buildTrakContent creates track content with EAC3 audio.
func buildTrakContent(bitrate uint32) []byte {
	var content []byte

	// tkhd (track header)
	tkhd := buildFullBox("tkhd", 0, 3, buildTkhdContent()) // flags=3 (enabled+in_movie)
	content = append(content, tkhd...)

	// mdia (media)
	mdia := buildBox("mdia", buildMdiaContent(bitrate))
	content = append(content, mdia...)

	return content
}

// buildTkhdContent creates track header content.
func buildTkhdContent() []byte {
	content := make([]byte, 80) // tkhd v0 is 80 bytes
	// track_ID at offset 12
	binary.BigEndian.PutUint32(content[12:16], 1)
	// duration at offset 20
	binary.BigEndian.PutUint32(content[20:24], 48000)
	// volume at offset 36 (audio track = 0x0100)
	binary.BigEndian.PutUint16(content[36:38], 0x0100)
	// matrix at offset 40: identity matrix
	binary.BigEndian.PutUint32(content[40:44], 0x00010000)
	binary.BigEndian.PutUint32(content[52:56], 0x00010000)
	binary.BigEndian.PutUint32(content[64:68], 0x40000000)
	return content
}

// buildMdiaContent creates media container content.
func buildMdiaContent(bitrate uint32) []byte {
	var content []byte

	// mdhd (media header)
	mdhd := buildFullBox("mdhd", 0, 0, buildMdhdContent())
	content = append(content, mdhd...)

	// hdlr (handler reference)
	hdlr := buildFullBox("hdlr", 0, 0, buildHdlrContent("soun", "SoundHandler"))
	content = append(content, hdlr...)

	// minf (media info)
	minf := buildBox("minf", buildMinfContent(bitrate))
	content = append(content, minf...)

	return content
}

// buildMdhdContent creates media header content.
func buildMdhdContent() []byte {
	content := make([]byte, 20) // mdhd v0 is 20 bytes
	// timescale at offset 8
	binary.BigEndian.PutUint32(content[8:12], 48000)
	// duration at offset 12
	binary.BigEndian.PutUint32(content[12:16], 48000)
	// language at offset 16 (und = undetermined)
	binary.BigEndian.PutUint16(content[16:18], 0x55C4) // "und"
	return content
}

// buildHdlrContent creates handler reference content.
func buildHdlrContent(handlerType, name string) []byte {
	content := make([]byte, 20+len(name)+1)
	// pre_defined at offset 0 (4 bytes = 0)
	// handler_type at offset 4
	copy(content[4:8], handlerType)
	// reserved at offset 8-20 (12 bytes = 0)
	// name at offset 20 (null-terminated)
	copy(content[20:], name)
	return content
}

// buildMinfContent creates media info content.
func buildMinfContent(bitrate uint32) []byte {
	var content []byte

	// smhd (sound media header)
	smhd := buildFullBox("smhd", 0, 0, make([]byte, 4)) // balance + reserved
	content = append(content, smhd...)

	// dinf (data info)
	dref := buildFullBox("dref", 0, 0, buildDrefContent())
	dinf := buildBox("dinf", dref)
	content = append(content, dinf...)

	// stbl (sample table)
	stbl := buildBox("stbl", buildStblContent(bitrate))
	content = append(content, stbl...)

	return content
}

// buildDrefContent creates data reference content.
func buildDrefContent() []byte {
	// entry_count (4 bytes) + url box
	content := make([]byte, 4)
	binary.BigEndian.PutUint32(content[0:4], 1) // 1 entry
	// url box with self-reference flag
	url := buildFullBox("url ", 0, 1, nil) // flags=1 means self-contained
	return append(content, url...)
}

// buildStblContent creates sample table content with EAC3 sample entry.
func buildStblContent(bitrate uint32) []byte {
	var content []byte

	// stsd (sample descriptions) - THIS IS WHERE ec-3 GOES
	stsd := buildFullBox("stsd", 0, 0, buildStsdWithEC3(bitrate))
	content = append(content, stsd...)

	// stts (time to sample) - empty for minimal file
	sttsContent := make([]byte, 4) // entry_count = 0
	stts := buildFullBox("stts", 0, 0, sttsContent)
	content = append(content, stts...)

	// stsc (sample to chunk) - empty
	stscContent := make([]byte, 4) // entry_count = 0
	stsc := buildFullBox("stsc", 0, 0, stscContent)
	content = append(content, stsc...)

	// stsz (sample sizes) - empty
	stszContent := make([]byte, 8) // sample_size=0, sample_count=0
	stsz := buildFullBox("stsz", 0, 0, stszContent)
	content = append(content, stsz...)

	// stco (chunk offsets) - empty
	stcoContent := make([]byte, 4) // entry_count = 0
	stco := buildFullBox("stco", 0, 0, stcoContent)
	content = append(content, stco...)

	return content
}

// buildStsdWithEC3 creates sample description with ec-3 (E-AC-3) entry.
func buildStsdWithEC3(bitrate uint32) []byte {
	// entry_count (4 bytes)
	content := make([]byte, 4)
	binary.BigEndian.PutUint32(content[0:4], 1)

	// ec-3 sample entry (AudioSampleEntry structure)
	ec3Content := buildEC3SampleEntry(bitrate)
	ec3 := buildBox("ec-3", ec3Content)

	return append(content, ec3...)
}

// buildEC3SampleEntry creates E-AC-3 audio sample entry content.
func buildEC3SampleEntry(bitrate uint32) []byte {
	// AudioSampleEntry base structure (28 bytes)
	entry := make([]byte, 28)
	// reserved (6 bytes) at offset 0
	// data_reference_index at offset 6
	binary.BigEndian.PutUint16(entry[6:8], 1)
	// reserved (8 bytes) at offset 8
	// channelcount at offset 16
	binary.BigEndian.PutUint16(entry[16:18], 6) // 5.1 channels
	// samplesize at offset 18
	binary.BigEndian.PutUint16(entry[18:20], 16)
	// pre_defined at offset 20
	// reserved at offset 22
	// samplerate at offset 24 (16.16 fixed point)
	binary.BigEndian.PutUint32(entry[24:28], 48000<<16) // 48000 Hz

	// dec3 box (E-AC-3 specific configuration)
	dec3Content := make([]byte, 3) // minimal dec3 content
	dec3Content[0] = 0x00          // data_rate MSB
	dec3Content[1] = 0x00          // data_rate LSB + num_ind_sub
	dec3Content[2] = 0x00          // acmod, lfeon, etc.
	dec3 := buildBox("dec3", dec3Content)
	entry = append(entry, dec3...)

	// btrt box (bitrate info)
	btrtContent := make([]byte, 12)
	binary.BigEndian.PutUint32(btrtContent[0:4], 4096)          // bufferSizeDB
	binary.BigEndian.PutUint32(btrtContent[4:8], bitrate+80000) // maxBitrate
	binary.BigEndian.PutUint32(btrtContent[8:12], bitrate)      // avgBitrate
	btrt := buildBox("btrt", btrtContent)
	entry = append(entry, btrt...)

	return entry
}

// buildUdtaWithTitle creates user data container with title metadata.
func buildUdtaWithTitle(title string) []byte {
	// Build ilst content
	var ilstContent []byte

	// ©nam (title) atom
	titleData := buildDataBox(1, []byte(title)) // type 1 = UTF-8
	nam := buildBox("\xa9nam", titleData)
	ilstContent = append(ilstContent, nam...)

	// Build meta content
	ilst := buildBox("ilst", ilstContent)
	hdlr := buildFullBox("hdlr", 0, 0, buildHdlrContent("mdir", ""))
	metaContent := append(hdlr, ilst...)
	meta := buildFullBox("meta", 0, 0, metaContent)

	return buildBox("udta", meta)
}

// buildDataBox creates a data box for metadata.
func buildDataBox(dataType uint32, content []byte) []byte {
	// data box: [version(1)][type(3)][locale(4)][data...]
	dataContent := make([]byte, 8+len(content))
	dataContent[0] = 0                              // version
	dataContent[1] = byte((dataType >> 16) & 0xff)  // type MSB
	dataContent[2] = byte((dataType >> 8) & 0xff)   // type
	dataContent[3] = byte(dataType & 0xff)          // type LSB
	binary.BigEndian.PutUint32(dataContent[4:8], 0) // locale
	copy(dataContent[8:], content)
	return buildBox("data", dataContent)
}
