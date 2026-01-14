package mp4

import (
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// Parse reads metadata from an M4B/MP4 file and returns it in the
// mediafile.ParsedMetadata format for compatibility with the existing scanner.
func Parse(path string) (*mediafile.ParsedMetadata, error) {
	// Read raw metadata using go-mp4
	raw, err := readMetadata(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Convert to the full Metadata struct (which does series parsing, etc.)
	meta := convertRawMetadata(raw)

	// Convert to the mediafile.ParsedMetadata format
	return &mediafile.ParsedMetadata{
		Title:         meta.Title,
		Subtitle:      meta.Subtitle,
		Authors:       meta.Authors,
		Narrators:     meta.Narrators,
		Series:        meta.Series,
		SeriesNumber:  meta.SeriesNumber,
		Genres:        meta.Genres,
		Tags:          meta.Tags,
		Description:   meta.Description,
		Publisher:     meta.Publisher,
		Imprint:       meta.Imprint,
		URL:           meta.URL,
		ReleaseDate:   meta.ReleaseDate,
		CoverMimeType: meta.CoverMimeType,
		CoverData:     meta.CoverData,
		DataSource:    models.DataSourceM4BMetadata,
		Duration:      meta.Duration,
		BitrateBps:    meta.Bitrate, // from esds, already in bps
		Identifiers:   meta.Identifiers,
	}, nil
}

// ParseFull reads complete metadata from an M4B/MP4 file including
// duration, chapters, and other extended information.
func ParseFull(path string) (*Metadata, error) {
	// Read raw metadata using go-mp4
	raw, err := readMetadata(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Convert to the full Metadata struct (bitrate is set from esds)
	return convertRawMetadata(raw), nil
}
