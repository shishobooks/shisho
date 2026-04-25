package settings

import "github.com/shishobooks/shisho/pkg/models"

// UserSettingsPayload is the request body for updating user settings.
//
// All fields are pointers so clients can send partial updates. Omitting a
// field (or sending it as null) leaves the current value untouched; sending
// a value updates just that field.
//
// Field shape must stay identical to UserSettingsUpdate (same field names,
// same types, same order) — the handler converts between them with
// `UserSettingsUpdate(payload)`. Drifting either one breaks that cast.
type UserSettingsPayload struct {
	PreloadCount *int    `json:"preload_count,omitempty"`
	FitMode      *string `json:"fit_mode,omitempty"`
	EpubFontSize *int    `json:"viewer_epub_font_size,omitempty"`
	EpubTheme    *string `json:"viewer_epub_theme,omitempty"`
	EpubFlow     *string `json:"viewer_epub_flow,omitempty"`
	GallerySize  *string `json:"gallery_size,omitempty"`
}

// UserSettingsResponse is the response for user settings.
type UserSettingsResponse struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
	EpubFontSize int    `json:"viewer_epub_font_size"`
	EpubTheme    string `json:"viewer_epub_theme"`
	EpubFlow     string `json:"viewer_epub_flow"`
	GallerySize  string `json:"gallery_size"`
}

// ValidFitModes returns all valid fit mode values.
func ValidFitModes() []string {
	return []string{models.FitModeHeight, models.FitModeOriginal}
}

// IsValidFitMode returns true if the fit mode is valid.
func IsValidFitMode(mode string) bool {
	for _, valid := range ValidFitModes() {
		if mode == valid {
			return true
		}
	}
	return false
}

// IsValidEpubTheme returns true if the theme is a supported EPUB theme.
func IsValidEpubTheme(theme string) bool {
	switch theme {
	case models.EpubThemeLight, models.EpubThemeDark, models.EpubThemeSepia:
		return true
	}
	return false
}

// IsValidEpubFlow returns true if the flow is a supported EPUB flow mode.
func IsValidEpubFlow(flow string) bool {
	switch flow {
	case models.EpubFlowPaginated, models.EpubFlowScrolled:
		return true
	}
	return false
}

// IsValidGallerySize returns true if the size is a supported gallery size.
func IsValidGallerySize(size string) bool {
	switch size {
	case models.GallerySizeSmall, models.GallerySizeMedium,
		models.GallerySizeLarge, models.GallerySizeExtraLarge:
		return true
	}
	return false
}

// UpdateLibrarySettingsPayload is the request body for PUT /settings/libraries/:library_id.
//
// SortSpec is a pointer so the client can distinguish "unset" (omit field
// from JSON) from "clear the saved default" (send null). A null body
// clears the saved sort; omitting the field leaves it untouched.
//
// max=200 caps the input length defensively at bind time. The longest
// possible legitimate spec — every field at the longest direction —
// fits comfortably under 200 chars (sortspec.MaxLevels=10), so 200 is
// a generous upper bound that still rejects pathological input before
// it reaches sortspec.Parse.
type UpdateLibrarySettingsPayload struct {
	SortSpec *string `json:"sort_spec" validate:"omitempty,max=200"`
}

// LibrarySettingsResponse is the response for GET/PUT /settings/libraries/:library_id.
type LibrarySettingsResponse struct {
	SortSpec *string `json:"sort_spec"`
}
