package mp4

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAudioObjectTypeToCodec tests the mapping from ISO 14496-3 audioObjectType to codec string.
func TestAudioObjectTypeToCodec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		aot      int
		expected string
	}{
		{"AAC Main", 1, "AAC Main"},
		{"AAC-LC", 2, "AAC-LC"},
		{"AAC SSR", 3, "AAC SSR"},
		{"AAC LTP", 4, "AAC LTP"},
		{"HE-AAC (SBR)", 5, "HE-AAC"},
		{"AAC Scalable", 6, "AAC Scalable"},
		{"HE-AACv2 (PS)", 29, "HE-AACv2"},
		{"xHE-AAC (USAC)", 42, "xHE-AAC"},
		{"Unknown defaults to AAC", 99, "AAC"},
		{"Zero defaults to AAC", 0, "AAC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := audioObjectTypeToCodec(tc.aot)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestParseCodecFromEsds tests codec extraction from raw esds box data.
func TestParseCodecFromEsds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name: "AAC-LC from typical esds",
			// Real esds data with ObjectTypeIndication=0x40 (MPEG-4 Audio) and audioObjectType=2 (AAC-LC)
			// Structure: [FullBox header][ES_Descriptor(0x03)][DecoderConfigDescriptor(0x04)][DecoderSpecificInfo(0x05)]
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox: version=0, flags=0
				0x03, 0x80, 0x80, 0x80, 0x25, // ES_Descriptor tag=0x03, size=37 (variable length)
				0x00, 0x01, 0x00, // ES_ID=1, flags
				0x04, 0x80, 0x80, 0x80, 0x17, // DecoderConfigDescriptor tag=0x04, size=23
				0x40,             // objectTypeIndication = 0x40 (MPEG-4 Audio)
				0x15,             // streamType=5 (audio), upStream=0, reserved=1
				0x00, 0x05, 0xec, // bufferSizeDB
				0x00, 0x01, 0x0a, 0xa6, // maxBitrate
				0x00, 0x01, 0x0a, 0xa6, // avgBitrate
				0x05, 0x80, 0x80, 0x80, 0x05, // DecoderSpecificInfo tag=0x05, size=5
				0x13, 0x90, 0x56, 0xe5, 0x00, // AudioSpecificConfig: audioObjectType=2 (AAC-LC)
				0x06, 0x80, 0x80, 0x80, 0x01, // SLConfigDescriptor
				0x02, // predefined=2
			},
			expected: "AAC-LC",
		},
		{
			name: "xHE-AAC (USAC) with extended audioObjectType",
			// esds with ObjectTypeIndication=0x40 and audioObjectType=42 (USAC/xHE-AAC)
			// audioObjectType 42 requires extended encoding: first 5 bits = 31, then 6 bits = 10 (31+32-32+10=42)
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x2b, // ES_Descriptor tag=0x03, size=43
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x23, // DecoderConfigDescriptor tag=0x04, size=35
				0x40,             // objectTypeIndication = 0x40 (MPEG-4 Audio)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x02, 0x56, 0x96, // maxBitrate
				0x00, 0x01, 0xa0, 0x89, // avgBitrate
				0x05, 0x14, // DecoderSpecificInfo tag=0x05, size=20
				0xf9, 0x48, 0x44, 0x22, // AudioSpecificConfig: first 5 bits=31 (escape), next 6 bits=10, so 32+10=42 (xHE-AAC)
				0x2c, 0xc0, 0x4c, 0x00,
				0xae, 0x40, 0x00, 0x84,
				0xe0, 0x02, 0x00, 0x00,
				0x46, 0xac, 0xb0, 0x00,
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "xHE-AAC",
		},
		{
			name: "MP3 from ObjectTypeIndication 0x6B",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x6b,             // objectTypeIndication = 0x6B (MP3)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "MP3",
		},
		{
			name: "MP3 from ObjectTypeIndication 0x69",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x69,             // objectTypeIndication = 0x69 (MP3 alternate)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "MP3",
		},
		{
			name: "MPEG-2 AAC Main from ObjectTypeIndication 0x66",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x66,             // objectTypeIndication = 0x66 (MPEG-2 AAC Main)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "MPEG-2 AAC Main",
		},
		{
			name: "MPEG-2 AAC-LC from ObjectTypeIndication 0x67",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x67,             // objectTypeIndication = 0x67 (MPEG-2 AAC-LC)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "MPEG-2 AAC-LC",
		},
		{
			name: "MPEG-2 AAC SSR from ObjectTypeIndication 0x68",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x68,             // objectTypeIndication = 0x68 (MPEG-2 AAC SSR)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "MPEG-2 AAC SSR",
		},
		{
			name: "Unknown ObjectTypeIndication returns Unknown",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x19, // ES_Descriptor
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x11, // DecoderConfigDescriptor
				0x20,             // objectTypeIndication = 0x20 (Visual ISO/IEC 14496-2 - not audio)
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x00, 0x00, 0x00, // maxBitrate
				0x00, 0x00, 0x00, 0x00, // avgBitrate
				0x05, 0x02, 0x00, 0x00, // DecoderSpecificInfo (minimal)
				0x06, 0x01, 0x02, // SLConfigDescriptor
			},
			expected: "Unknown",
		},
		{
			name:     "Empty data returns empty string",
			data:     []byte{},
			expected: "",
		},
		{
			name:     "Too short data returns empty string",
			data:     []byte{0x00, 0x00},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseCodecFromEsds(tc.data)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestParseBitrateFromEsds tests bitrate extraction from raw esds box data.
func TestParseBitrateFromEsds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{
			name: "Extract bitrate from typical esds",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x80, 0x80, 0x80, 0x25, // ES_Descriptor
				0x00, 0x01, 0x00, // ES_ID, flags
				0x04, 0x80, 0x80, 0x80, 0x17, // DecoderConfigDescriptor
				0x40,             // objectTypeIndication
				0x15,             // streamType
				0x00, 0x05, 0xec, // bufferSizeDB
				0x00, 0x01, 0x0a, 0xa6, // maxBitrate = 68262
				0x00, 0x01, 0x0a, 0xa6, // avgBitrate = 68262
			},
			expected: 68262,
		},
		{
			name: "Extract higher bitrate",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // FullBox header
				0x03, 0x80, 0x80, 0x80, 0x22, // ES_Descriptor (variable length size encoding)
				0x00, 0x00, 0x00, // ES_ID, flags
				0x04, 0x80, 0x80, 0x80, 0x14, // DecoderConfigDescriptor (variable length)
				0x40,             // objectTypeIndication
				0x15,             // streamType
				0x00, 0x00, 0x00, // bufferSizeDB
				0x00, 0x01, 0xF4, 0x00, // maxBitrate = 128000
				0x00, 0x01, 0xF4, 0x00, // avgBitrate = 128000
				0x05, 0x80, 0x80, 0x80, 0x02, // DecoderSpecificInfo tag=0x05
				0x11, 0x90, // AudioSpecificConfig
				0x06, 0x80, 0x80, 0x80, 0x01, // SLConfigDescriptor
				0x02, // predefined=2
			},
			expected: 128000,
		},
		{
			name:     "Empty data returns 0",
			data:     []byte{},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseBitrateFromEsds(tc.data)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestParseBitrateFromBtrt tests bitrate extraction from btrt box data (used by EAC3/AC3).
func TestParseBitrateFromBtrt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{
			name: "EAC3 btrt box with 640 kbps",
			// Simulates ec-3 sample entry data containing a btrt box
			// btrt structure: [4 bytes size][4 bytes "btrt"][4 bytes bufferSizeDB][4 bytes maxBitrate][4 bytes avgBitrate]
			data: []byte{
				// Some ec-3 specific data before btrt
				0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x02, // data reference index
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x02, // channel count
				0x00, 0x10, // sample size (16 bits)
				0x00, 0x00, // pre-defined
				0x00, 0x00, // reserved
				0xbb, 0x80, 0x00, 0x00, // sample rate (48000 Hz in 16.16 fixed point)
				// dec3 box (E-AC-3 specific config)
				0x00, 0x00, 0x00, 0x08, // size = 8
				'd', 'e', 'c', '3', // type
				// btrt box
				0x00, 0x00, 0x00, 0x14, // size = 20 (8 header + 12 payload)
				'b', 't', 'r', 't', // type
				0x00, 0x00, 0x10, 0x00, // bufferSizeDB = 4096
				0x00, 0x0a, 0xfc, 0x80, // maxBitrate = 720000 bps
				0x00, 0x09, 0xc4, 0x00, // avgBitrate = 640000 bps (640 kbps)
			},
			expected: 640000,
		},
		{
			name: "AC3 btrt box with 448 kbps",
			data: []byte{
				// Some ac-3 specific data before btrt
				0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x01, // data reference index
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x06, // channel count (5.1)
				0x00, 0x10, // sample size
				0x00, 0x00, // pre-defined
				0x00, 0x00, // reserved
				0xbb, 0x80, 0x00, 0x00, // sample rate
				// dac3 box (AC-3 specific config)
				0x00, 0x00, 0x00, 0x0b, // size = 11
				'd', 'a', 'c', '3', // type
				0x00, 0x00, 0x00, // AC-3 specific info
				// btrt box
				0x00, 0x00, 0x00, 0x14, // size = 20
				'b', 't', 'r', 't', // type
				0x00, 0x00, 0x08, 0x00, // bufferSizeDB
				0x00, 0x07, 0xa1, 0x20, // maxBitrate = 500000
				0x00, 0x06, 0xd6, 0x00, // avgBitrate = 448000 bps (448 kbps)
			},
			expected: 448000,
		},
		{
			name: "btrt at start of data",
			data: []byte{
				0x00, 0x00, 0x00, 0x14, // size = 20
				'b', 't', 'r', 't', // type
				0x00, 0x00, 0x04, 0x00, // bufferSizeDB
				0x00, 0x01, 0xf4, 0x00, // maxBitrate = 128000
				0x00, 0x01, 0x86, 0xa0, // avgBitrate = 100000 bps
			},
			expected: 100000,
		},
		{
			name:     "No btrt box returns 0",
			data:     []byte{0x00, 0x00, 0x00, 0x08, 'd', 'e', 'c', '3', 0x00, 0x00, 0x00, 0x00},
			expected: 0,
		},
		{
			name:     "Empty data returns 0",
			data:     []byte{},
			expected: 0,
		},
		{
			name:     "Data too short for btrt returns 0",
			data:     []byte{0x00, 0x00, 0x00, 0x14, 'b', 't', 'r', 't'},
			expected: 0,
		},
		{
			name: "btrt with truncated avgBitrate returns 0",
			data: []byte{
				0x00, 0x00, 0x00, 0x14, // size
				'b', 't', 'r', 't', // type
				0x00, 0x00, 0x04, 0x00, // bufferSizeDB
				0x00, 0x01, 0xf4, 0x00, // maxBitrate
				0x00, 0x01, // truncated avgBitrate (only 2 bytes)
			},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseBitrateFromBtrt(tc.data)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestParseAACProfile tests AAC profile extraction from esds data.
func TestParseAACProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name: "AAC-LC profile (audioObjectType=2)",
			data: []byte{
				0x00, 0x00, 0x00, 0x00,
				0x04, 0x11, // DecoderConfigDescriptor
				0x40, 0x15, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x05, 0x02, // DecoderSpecificInfo tag=0x05, size=2
				0x11, 0x90, // AudioSpecificConfig: 0x11 = 0001 0001 -> audioObjectType = 2 (AAC-LC)
			},
			expected: "AAC-LC",
		},
		{
			name: "HE-AAC profile (audioObjectType=5)",
			data: []byte{
				0x00, 0x00, 0x00, 0x00,
				0x04, 0x11,
				0x40, 0x15, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x05, 0x02,
				0x29, 0x90, // audioObjectType = 5 (HE-AAC/SBR): 0010 1001 -> first 5 bits = 00101 = 5
			},
			expected: "HE-AAC",
		},
		{
			name: "Fallback to AAC when no DecoderSpecificInfo",
			data: []byte{
				0x00, 0x00, 0x00, 0x00,
				0x04, 0x0d,
				0x40, 0x15, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				// No 0x05 tag
			},
			expected: "AAC",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseAACProfile(tc.data)
			assert.Equal(t, tc.expected, result)
		})
	}
}
