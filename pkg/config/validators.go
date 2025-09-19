package config

type UpdateConfigPayload struct {
	SyncIntervalMinutes *int `json:"sync_interval_minutes,omitempty" validate:"omitempty,min=1"`
}
