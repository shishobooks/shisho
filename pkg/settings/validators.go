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
