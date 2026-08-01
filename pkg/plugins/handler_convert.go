package plugins

import (
	"math"
	"strings"
	"time"

	"github.com/shishobooks/shisho/pkg/htmlutil"
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
	if v, ok := fields["url"].(string); ok {
		md.URL = v
	}
	if v, ok := fields["series"].(string); ok {
		md.Series = strings.TrimSpace(v)
	}
	if v, ok := fields["cover_url"].(string); ok {
		md.CoverURL = v
	}

	md.SeriesNumber, md.SeriesNumberEnd, md.SeriesNumberUnit = seriesNumberGroupFromFields(fields)

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
	if v, ok := fields["release_date"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				t, err = time.Parse(time.RFC3339, v)
			}
			if err == nil {
				md.ReleaseDate = &t
			}
		}
	}

	// Authors: []{ name: string, role: string }
	if v, ok := fields["authors"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				name, _ := m["name"].(string)
				role, _ := m["role"].(string)
				name = strings.TrimSpace(name)
				role = strings.TrimSpace(role)
				if name != "" {
					md.Authors = append(md.Authors, mediafile.ParsedAuthor{Name: name, Role: role})
				}
			}
		}
	}

	// Narrators: []string
	if v, ok := fields["narrators"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				md.Narrators = append(md.Narrators, strings.TrimSpace(s))
			}
		}
	}

	// Genres: []string
	if v, ok := fields["genres"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				md.Genres = append(md.Genres, strings.TrimSpace(s))
			}
		}
	}

	// Tags: []string
	if v, ok := fields["tags"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				md.Tags = append(md.Tags, strings.TrimSpace(s))
			}
		}
	}

	// Identifiers: []{ type: string, value: string }
	if v, ok := fields["identifiers"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				idType, _ := m["type"].(string)
				idValue, _ := m["value"].(string)
				idType = strings.TrimSpace(idType)
				idValue = strings.TrimSpace(idValue)
				if idType != "" && idValue != "" {
					md.Identifiers = append(md.Identifiers, mediafile.ParsedIdentifier{Type: idType, Value: idValue})
				}
			}
		}
	}

	// Language
	if v, ok := fields["language"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			md.Language = mediafile.NormalizeLanguage(v)
		}
	}

	// Abridged
	if v, ok := fields["abridged"].(bool); ok {
		md.Abridged = &v
	}

	return md
}

// convertFieldsToOverrides extracts apply-path-only signals from the
// untyped fields map. SelectedFields only includes values with a valid outer
// shape. Non-empty collections must contain at least one valid item, so
// malformed plugin output cannot masquerade as an explicit clear.
func convertFieldsToOverrides(fields map[string]any, md *mediafile.ParsedMetadata) *ApplyOverrides {
	selected := make(map[string]bool)
	for _, key := range []string{"title", "subtitle", "publisher", "url"} {
		if _, ok := fields[key].(string); ok {
			selected[key] = true
		}
	}
	if v, ok := fields["description"].(string); ok && (strings.TrimSpace(v) == "" || htmlutil.StripTags(v) != "") {
		selected["description"] = true
	}
	if v, ok := fields["release_date"].(string); ok && (strings.TrimSpace(v) == "" || md.ReleaseDate != nil) {
		selected["release_date"] = true
	}
	if v, ok := fields["language"].(string); ok && (strings.TrimSpace(v) == "" || md.Language != nil) {
		selected["language"] = true
	}
	if v, ok := fields["authors"].([]any); ok && (len(v) == 0 || len(md.Authors) > 0) {
		selected["authors"] = true
	}
	if v, ok := fields["narrators"].([]any); ok && (len(v) == 0 || len(md.Narrators) > 0) {
		selected["narrators"] = true
	}
	if v, ok := fields["genres"].([]any); ok && (len(v) == 0 || len(md.Genres) > 0) {
		selected["genres"] = true
	}
	if v, ok := fields["tags"].([]any); ok && (len(v) == 0 || len(md.Tags) > 0) {
		selected["tags"] = true
	}
	if v, ok := fields["identifiers"].([]any); ok && (len(v) == 0 || len(md.Identifiers) > 0) {
		selected["identifiers"] = true
	}
	if v, ok := fields["abridged"]; ok && (v == nil || isBool(v)) {
		selected["abridged"] = true
	}

	if len(selected) == 0 {
		return nil
	}
	return &ApplyOverrides{SelectedFields: selected}
}

func isBool(v any) bool {
	_, ok := v.(bool)
	return ok
}

// extractSeriesEntries checks whether fields["series"] is an array of
// objects (the multi-series format sent by the identify form). Returns
// nil when the key is absent or is a string (handled by convertFieldsToMetadata).
// Returns a non-nil pointer to an empty slice when the key is an empty array
// (meaning "clear all series").
func extractSeriesEntries(fields map[string]any) *[]SeriesEntry {
	v, ok := fields["series"]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil // scalar string — handled by convertFieldsToMetadata
	}
	entries := make([]SeriesEntry, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entry := SeriesEntry{Name: name}
		groupFields := map[string]any{
			"series_number":      m["number"],
			"series_number_end":  m["series_number_end"],
			"series_number_unit": m["series_number_unit"],
		}
		entry.Number, entry.NumberEnd, entry.SeriesNumberUnit = seriesNumberGroupFromFields(groupFields)
		entries = append(entries, entry)
	}
	if len(arr) > 0 && len(entries) == 0 {
		return nil
	}
	return &entries
}
