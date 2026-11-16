package updatecheck

// UpdateCheckResponse is the GET /update-check response: how the running
// server's version compares with the latest published Shisho release.
//
//nolint:revive // UpdateCheckResponse name is mandated by the {Entity}Response convention (ADR 0004)
type UpdateCheckResponse struct {
	// CurrentVersion is the version the server was built as ("dev" for a
	// development build).
	CurrentVersion string `json:"current_version"`
	// LatestVersion is the newest published release, without the "v". It is
	// empty until a check has succeeded, and stays empty when checks are off.
	LatestVersion string `json:"latest_version"`
	// ReleasesBehind counts the published releases newer than
	// CurrentVersion.
	ReleasesBehind int `json:"releases_behind"`
	// UpdateAvailable is true when a newer release than CurrentVersion is
	// published.
	UpdateAvailable bool `json:"update_available"`
}
