package mp4

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// Parse reads metadata from an M4B/MP4 file and returns it in the
// mediafile.ParsedMetadata format for compatibility with the existing scanner.
func Parse(path string) (*mediafile.ParsedMetadata, error) {
	meta, err := ParseFull(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Convert to the mediafile.ParsedMetadata format
	return &mediafile.ParsedMetadata{
		Title:           meta.Title,
		Subtitle:        meta.Subtitle,
		Authors:         meta.Authors,
		Narrators:       meta.Narrators,
		Series:          meta.Series,
		SeriesNumber:    meta.SeriesNumber,
		SeriesNumberEnd: meta.SeriesNumberEnd,
		Genres:          meta.Genres,
		Tags:            meta.Tags,
		Description:     meta.Description,
		Publisher:       meta.Publisher,
		URL:             meta.URL,
		ReleaseDate:     meta.ReleaseDate,
		CoverMimeType:   meta.CoverMimeType,
		CoverData:       meta.CoverData,
		DataSource:      models.DataSourceM4BMetadata,
		Duration:        meta.Duration,
		BitrateBps:      meta.Bitrate, // from esds, already in bps
		Codec:           meta.Codec,   // from esds AudioSpecificConfig
		Identifiers:     meta.Identifiers,
		Chapters:        convertChaptersToParsed(meta.Chapters),
		Language:        meta.Language,
		Abridged:        meta.Abridged,
	}, nil
}

// ParseFull reads complete metadata from an M4B/MP4 file including
// duration, chapters, and other extended information.
func ParseFull(path string) (*Metadata, error) {
	return ParseFullContext(context.Background(), path)
}

// ParseFullContext is ParseFull with cancellation support.
func ParseFullContext(ctx context.Context, path string) (*Metadata, error) {
	raw, err := readMetadataContext(ctx, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return convertRawMetadata(raw), nil
}
