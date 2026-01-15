package mp4

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
)

// Metadata represents extracted M4B audiobook metadata.
type Metadata struct {
	Title         string
	Subtitle      string                       // from ----:com.apple.iTunes:SUBTITLE or ----:com.pilabor.tone:SUBTITLE
	Authors       []mediafile.ParsedAuthor     // from ©ART (artist)
	Narrators     []string                     // from ©nrt (narrator) or ©cmp (composer)
	Album         string                       // from ©alb
	Series        string                       // parsed from album or ©grp
	SeriesNumber  *float64                     // parsed from album
	Genre         string                       // from ©gen or gnre (original, may be comma-separated)
	Genres        []string                     // parsed from ©gen (comma-separated)
	Tags          []string                     // from ----:com.shisho:tags freeform atom
	Description   string                       // from desc
	Publisher     string                       // from ©pub
	Imprint       string                       // from com.shisho:imprint freeform
	URL           string                       // from com.shisho:url freeform
	ReleaseDate   *time.Time                   // parsed from rldt or ©day
	Comment       string                       // from ©cmt
	Year          string                       // from ©day
	Copyright     string                       // from ©cpy
	Encoder       string                       // from ©too
	CoverData     []byte                       // cover artwork
	CoverMimeType string                       // "image/jpeg" or "image/png"
	Duration      time.Duration                // from mvhd
	Bitrate       int                          // bps from esds
	Chapters      []Chapter                    // chapter list (Phase 3)
	MediaType     int                          // from stik (2 = audiobook)
	Freeform      map[string]string            // freeform (----) atoms like com.apple.iTunes:ASIN
	Identifiers   []mediafile.ParsedIdentifier // parsed identifiers from freeform atoms
	UnknownAtoms  []RawAtom                    // preserved unrecognized atoms from source
}

// RawAtom represents an MP4 atom preserved in its raw form.
type RawAtom struct {
	Type [4]byte // 4-byte atom type code
	Data []byte  // complete atom data including header
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
		Comment:       raw.comment,
		Year:          raw.year,
		Copyright:     raw.copyright,
		Encoder:       raw.encoder,
		CoverData:     raw.coverData,
		CoverMimeType: raw.coverMime,
		MediaType:     int(raw.mediaType),
	}

	// Parse authors from artist field (comma-separated)
	// M4B authors have no specific role (generic author)
	authorNames := splitMultiValue(raw.artist)
	meta.Authors = make([]mediafile.ParsedAuthor, len(authorNames))
	for i, name := range authorNames {
		meta.Authors[i] = mediafile.ParsedAuthor{Name: name, Role: ""}
	}

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

	// Parse genres from genre field (comma-separated)
	if raw.genre != "" {
		meta.Genres = splitMultiValue(raw.genre)
	}

	// Copy freeform atoms and extract subtitle/tags
	if len(raw.freeform) > 0 {
		meta.Freeform = make(map[string]string, len(raw.freeform))
		for k, v := range raw.freeform {
			meta.Freeform[k] = v
		}
		// Extract subtitle from freeform SUBTITLE atom (try iTunes first, then Tone)
		if subtitle, ok := raw.freeform["com.apple.iTunes:SUBTITLE"]; ok {
			meta.Subtitle = subtitle
		} else if subtitle, ok := raw.freeform["com.pilabor.tone:SUBTITLE"]; ok {
			meta.Subtitle = subtitle
		}
		// Extract tags from freeform shisho:tags atom (comma-separated)
		if tagsStr, ok := raw.freeform["com.shisho:tags"]; ok {
			meta.Tags = splitMultiValue(tagsStr)
		}
		// Extract imprint from freeform
		if imp, ok := raw.freeform["com.shisho:imprint"]; ok {
			meta.Imprint = imp
		}
		// Extract URL from freeform
		if url, ok := raw.freeform["com.shisho:url"]; ok {
			meta.URL = url
		}
		// Extract ASIN from freeform - check multiple possible locations
		// com.apple.iTunes:ASIN is the standard iTunes format
		// com.pilabor.tone:AUDIBLE_ASIN is used by tools like tone for Audible files
		asin := ""
		if v, ok := raw.freeform["com.apple.iTunes:ASIN"]; ok {
			asin = v
		} else if v, ok := raw.freeform["com.pilabor.tone:AUDIBLE_ASIN"]; ok {
			asin = v
		}
		asin = strings.TrimSpace(asin)
		if asin != "" {
			meta.Identifiers = append(meta.Identifiers, mediafile.ParsedIdentifier{
				Type:  "asin",
				Value: asin,
			})
		}
	}

	// Set publisher
	meta.Publisher = raw.publisher

	// Parse release date - prefer rldt, fall back to ©day
	var releaseDate *time.Time
	dateStr := raw.releaseDate
	if dateStr == "" {
		dateStr = raw.year
	}
	if dateStr != "" {
		formats := []string{
			"2006-01-02",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
			"2006",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, dateStr); err == nil {
				releaseDate = &t
				break
			}
		}
	}
	meta.ReleaseDate = releaseDate

	// Copy chapters
	meta.Chapters = raw.chapters

	// Copy unknown atoms for preservation
	meta.UnknownAtoms = raw.unknownAtoms

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
