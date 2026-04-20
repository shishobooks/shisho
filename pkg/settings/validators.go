package settings

import "github.com/shishobooks/shisho/pkg/models"

// ViewerSettingsPayload is the request body for updating viewer settings.
type ViewerSettingsPayload struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
}

// ViewerSettingsResponse is the response for viewer settings.
type ViewerSettingsResponse struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
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
