package mp4

import "errors"

// Errors returned by the mp4 package.
var (
	// ErrNotMP4 is returned when the file is not a valid MP4/M4B file.
	ErrNotMP4 = errors.New("not a valid MP4/M4B file")

	// ErrNoMetadata is returned when the file has no metadata.
	ErrNoMetadata = errors.New("no metadata found")

	// ErrInvalidBox is returned when a box structure is invalid.
	ErrInvalidBox = errors.New("invalid box structure")
)
