package testgen

import (
	"bytes"
	"os"
	"testing"
)

// M4BType18Options configures the synthetic M4B file with data type 18 genre.
type M4BType18Options struct {
	Title string
	Genre string // Will be written with data type 18
}

// GenerateM4BWithType18Genre creates an M4B file with a genre atom using data type 18
// (the problematic type that dhowden/tag doesn't handle).
//
// Strategy: Generate a valid M4B with ffmpeg, then patch the genre data type to 18.
// This ensures the overall file structure is valid and recognized by parsers.
func GenerateM4BWithType18Genre(t *testing.T, dir, filename string, opts M4BType18Options) string {
	t.Helper()
	SkipIfNoFFmpeg(t)

	// First, generate a valid M4B with ffmpeg including genre
	path := GenerateM4B(t, dir, filename, M4BOptions{
		Title: opts.Title,
		Genre: opts.Genre,
	})

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read M4B file: %v", err)
	}

	// Find and patch the genre data type from 1 to 18
	// The ©gen atom structure is:
	//   [4 bytes size][4 bytes "©gen"][4 bytes size][4 bytes "data"]
	//   [1 byte version][3 bytes type][4 bytes locale][...data...]
	// We need to find "©gen" followed by "data" and change the type bytes

	// Look for the ©gen atom - it uses the copyright symbol which is 0xA9 in MacRoman
	genMarker := []byte{0xA9, 'g', 'e', 'n'}
	dataMarker := []byte("data")

	patched := false
	for i := 0; i < len(data)-20; i++ {
		// Look for ©gen
		if bytes.Equal(data[i:i+4], genMarker) {
			// Skip ©gen box size (4 bytes before marker) and marker (4 bytes)
			// Next should be the data box: size (4 bytes) + "data" (4 bytes)
			dataBoxStart := i + 4 // after ©gen marker
			if dataBoxStart+8 < len(data) && bytes.Equal(data[dataBoxStart+4:dataBoxStart+8], dataMarker) {
				// Found the data box, now patch the type
				// Type is at offset: dataBoxStart + 8 (after "data") + 1 (version byte)
				// The type is 3 bytes big-endian
				typeOffset := dataBoxStart + 8 + 1
				if typeOffset+3 <= len(data) {
					// Set type to 18 (0x000012) - big endian in 3 bytes
					data[typeOffset] = 0
					data[typeOffset+1] = 0
					data[typeOffset+2] = 18
					patched = true
					break
				}
			}
		}
	}

	if !patched {
		t.Fatalf("failed to find and patch ©gen atom in M4B file")
	}

	// Write the patched file back
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write patched M4B file: %v", err)
	}

	return path
}
