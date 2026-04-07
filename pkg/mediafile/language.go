package mediafile

import (
	"golang.org/x/text/language"
)

// NormalizeLanguage validates and normalizes a language tag string.
// It accepts ISO 639-1 (e.g., "en"), ISO 639-2/T (e.g., "eng"), and BCP 47
// tags (e.g., "en-US", "zh-Hans-CN"). Case is normalized (e.g., "EN-us" → "en-US").
// Returns nil for empty input, unrecognized tags, or the undetermined language ("und").
func NormalizeLanguage(s string) *string {
	if s == "" {
		return nil
	}

	tag, err := language.Parse(s)
	if err != nil {
		return nil
	}

	// Reject the undetermined language tag.
	if tag == language.Und {
		return nil
	}

	// Reject tags that contain BCP 47 extensions (e.g., "not-a-language" parses as
	// language "not" with extension singleton "a"). Valid language tags like "en-US"
	// or "zh-Hans-CN" have no extensions.
	if len(tag.Extensions()) > 0 {
		return nil
	}

	normalized := tag.String()
	return &normalized
}
