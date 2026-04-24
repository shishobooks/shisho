package plugins

import (
	"math"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
)

// convertFieldsToMetadata converts an untyped fields map (from the apply payload) to *mediafile.ParsedMetadata.
func convertFieldsToMetadata(fields map[string]any) *mediafile.ParsedMetadata {
	md := &mediafile.ParsedMetadata{}

	if v, ok := fields["title"].(string); ok {
		md.Title = v
	}
	if v, ok := fields["subtitle"].(string); ok {
		md.Subtitle = v
	}
	if v, ok := fields["description"].(string); ok {
		md.Description = v
	}
	if v, ok := fields["publisher"].(string); ok {
		md.Publisher = v
	}
	if v, ok := fields["imprint"].(string); ok {
		md.Imprint = v
	}
	if v, ok := fields["url"].(string); ok {
		md.URL = v
	}
	if v, ok := fields["series"].(string); ok {
		md.Series = v
	}
	if v, ok := fields["cover_url"].(string); ok {
		md.CoverURL = v
	}

	// Series number
	if v, ok := fields["series_number"].(float64); ok {
		md.SeriesNumber = &v
	}

	// Cover page (0-indexed page number for CBZ/PDF). Only accept finite
	// non-negative integers; reject negative, NaN, and Infinity so they
	// don't propagate to the apply path.
	if v, ok := fields["cover_page"].(float64); ok {
		if !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 {
			cp := int(v)
			md.CoverPage = &cp
		}
	}

	// Release date
	if v, ok := fields["release_date"].(string); ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			t, err = time.Parse(time.RFC3339, v)
		}
		if err == nil {
			md.ReleaseDate = &t
		}
	}

	// Authors: []{ name: string, role: string }
	if v, ok := fields["authors"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				name, _ := m["name"].(string)
				role, _ := m["role"].(string)
				if name != "" {
					md.Authors = append(md.Authors, mediafile.ParsedAuthor{Name: name, Role: role})
				}
			}
		}
	}

	// Narrators: []string
	if v, ok := fields["narrators"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Narrators = append(md.Narrators, s)
			}
		}
	}

	// Genres: []string
	if v, ok := fields["genres"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Genres = append(md.Genres, s)
			}
		}
	}

	// Tags: []string
	if v, ok := fields["tags"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Tags = append(md.Tags, s)
			}
		}
	}

	// Identifiers: []{ type: string, value: string }
	if v, ok := fields["identifiers"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				idType, _ := m["type"].(string)
				idValue, _ := m["value"].(string)
				if idType != "" && idValue != "" {
					md.Identifiers = append(md.Identifiers, mediafile.ParsedIdentifier{Type: idType, Value: idValue})
				}
			}
		}
	}

	// Language
	if v, ok := fields["language"].(string); ok && v != "" {
		md.Language = mediafile.NormalizeLanguage(v)
	}

	// Abridged
	if v, ok := fields["abridged"].(bool); ok {
		md.Abridged = &v
	}

	return md
}
