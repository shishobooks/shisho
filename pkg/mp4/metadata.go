package mp4

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Metadata represents extracted M4B audiobook metadata.
type Metadata struct {
	Title         string
	Subtitle      string            // from ----:com.apple.iTunes:SUBTITLE freeform atom
	Authors       []string          // from ©ART (artist)
	Narrators     []string          // from ©nrt (narrator) or ©cmp (composer)
	Album         string            // from ©alb
	Series        string            // parsed from album or ©grp
	SeriesNumber  *float64          // parsed from album
	Genre         string            // from ©gen or gnre
	Description   string            // from desc
	CoverData     []byte            // cover artwork
	CoverMimeType string            // "image/jpeg" or "image/png"
	Duration      time.Duration     // from mvhd
	Bitrate       int               // bps from esds
	Chapters      []Chapter         // chapter list (Phase 3)
	MediaType     int               // from stik (2 = audiobook)
	Freeform      map[string]string // freeform (----) atoms like com.apple.iTunes:ASIN
}

// Chapter represents a chapter in the audiobook.
type Chapter struct {
	Title string
	Start time.Duration
	End   time.Duration
}

// seriesInfo holds parsed series information.
type seriesInfo struct {
	series string
	number *float64
}

// convertRawMetadata converts rawMetadata to the public Metadata struct.
func convertRawMetadata(raw *rawMetadata) *Metadata {
	meta := &Metadata{
		Title:         raw.title,
		Album:         raw.album,
		Genre:         raw.genre,
		Description:   raw.description,
		CoverData:     raw.coverData,
		CoverMimeType: raw.coverMime,
		MediaType:     int(raw.mediaType),
	}

	// Parse authors from artist field (comma-separated)
	meta.Authors = splitMultiValue(raw.artist)

	// Parse narrators (comma-separated)
	// Prefer ©nrt (dedicated narrator), fall back to ©cmp (composer), then ©wrt (writer)
	if raw.narrator != "" {
		meta.Narrators = splitMultiValue(raw.narrator)
	} else if raw.composer != "" {
		meta.Narrators = splitMultiValue(raw.composer)
	} else if raw.writer != "" {
		meta.Narrators = splitMultiValue(raw.writer)
	}

	// Parse series information from album field
	if raw.album != "" {
		if parsed := parseSeriesFromGrouping(raw.album); parsed.series != "" {
			meta.Series = parsed.series
			meta.SeriesNumber = parsed.number
		}
	}

	// Calculate duration from timescale and duration values
	if raw.timescale > 0 && raw.duration > 0 {
		// Safe conversion: duration in seconds as float, then to time.Duration
		durationSec := float64(raw.duration) / float64(raw.timescale)
		meta.Duration = time.Duration(durationSec * float64(time.Second))
	}

	// Copy bitrate from esds (already in bps)
	meta.Bitrate = int(raw.avgBitrate)

	// Copy freeform atoms and extract subtitle
	if len(raw.freeform) > 0 {
		meta.Freeform = make(map[string]string, len(raw.freeform))
		for k, v := range raw.freeform {
			meta.Freeform[k] = v
		}
		// Extract subtitle from freeform SUBTITLE atom
		if subtitle, ok := raw.freeform["com.apple.iTunes:SUBTITLE"]; ok {
			meta.Subtitle = subtitle
		}
	}

	// Copy chapters
	meta.Chapters = raw.chapters

	return meta
}

// parseSeriesFromGrouping extracts series name and number from a grouping string.
// Handles patterns like "Dungeon Crawler Carl #7", "Series Name, Book 3",
// and "Series Name - Volume 2".
func parseSeriesFromGrouping(grouping string) seriesInfo {
	// Pattern: "Series Name #N" or "Series Name #N.N"
	hashPattern := regexp.MustCompile(`^(.+?)\s*#(\d+(?:\.\d+)?)$`)
	if matches := hashPattern.FindStringSubmatch(grouping); len(matches) == 3 {
		seriesName := strings.TrimSpace(matches[1])
		if num, err := strconv.ParseFloat(matches[2], 64); err == nil {
			return seriesInfo{series: seriesName, number: &num}
		}
		return seriesInfo{series: seriesName, number: nil}
	}

	// Pattern: "Series Name, Book N"
	bookPattern := regexp.MustCompile(`^(.+?),\s*[Bb]ook\s+(\d+(?:\.\d+)?)$`)
	if matches := bookPattern.FindStringSubmatch(grouping); len(matches) == 3 {
		seriesName := strings.TrimSpace(matches[1])
		if num, err := strconv.ParseFloat(matches[2], 64); err == nil {
			return seriesInfo{series: seriesName, number: &num}
		}
		return seriesInfo{series: seriesName, number: nil}
	}

	// Pattern: "Series Name - Volume N" or "Series Name - Vol N" or "Series Name - Vol. N"
	volPattern := regexp.MustCompile(`^(.+?)\s*[-–]\s*[Vv]ol(?:ume)?\.?\s*(\d+(?:\.\d+)?)$`)
	if matches := volPattern.FindStringSubmatch(grouping); len(matches) == 3 {
		seriesName := strings.TrimSpace(matches[1])
		if num, err := strconv.ParseFloat(matches[2], 64); err == nil {
			return seriesInfo{series: seriesName, number: &num}
		}
		return seriesInfo{series: seriesName, number: nil}
	}

	// Pattern: "Series Name (Book N)" or "Series Name (N)"
	parenPattern := regexp.MustCompile(`^(.+?)\s*\((?:[Bb]ook\s+)?(\d+(?:\.\d+)?)\)$`)
	if matches := parenPattern.FindStringSubmatch(grouping); len(matches) == 3 {
		seriesName := strings.TrimSpace(matches[1])
		if num, err := strconv.ParseFloat(matches[2], 64); err == nil {
			return seriesInfo{series: seriesName, number: &num}
		}
		return seriesInfo{series: seriesName, number: nil}
	}

	// If no pattern matches, return empty
	return seriesInfo{}
}
