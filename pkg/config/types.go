package config

// ConfigResponse is the GET /config response: the loaded config plus values
// the server derives from it.
//
//nolint:revive // ConfigResponse name is mandated by the {Entity}Response convention (ADR 0004)
type ConfigResponse struct {
	Config `tstype:",extends"`

	// LibraryMonitorEffectiveDelaySeconds is the debounce delay the library
	// monitor uses: library_monitor_delay_seconds raised to
	// MinLibraryMonitorDelaySeconds.
	LibraryMonitorEffectiveDelaySeconds int `json:"library_monitor_effective_delay_seconds"`

	// JWTSecretTooShort is true when jwt_secret is shorter than
	// MinJWTSecretLength. The secret itself is never returned.
	JWTSecretTooShort bool `json:"jwt_secret_too_short"`
}
