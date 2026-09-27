// Package sharelinks holds the Share Link feature: anonymous, expiring links
// that grant access to one Book.
package sharelinks

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
)

// SettingsKey is the app_settings key holding the sharing settings document.
const SettingsKey = "sharing"

// Settings is the admin-controlled sharing policy. It lives in the settings
// store rather than server config (ADR 0008). The zero value is the default:
// sharing off and expiration optional.
type Settings struct {
	Enabled           bool `json:"enabled"`
	RequireExpiration bool `json:"require_expiration"`
}

// LoadSettings reads the sharing settings, returning the defaults if none
// have been saved.
func LoadSettings(ctx context.Context, store *appsettings.Service) (Settings, error) {
	var s Settings
	if _, err := store.GetJSON(ctx, SettingsKey, &s); err != nil {
		return Settings{}, errors.WithStack(err)
	}
	return s, nil
}

// SaveSettings persists the sharing settings.
func SaveSettings(ctx context.Context, store *appsettings.Service, s Settings) error {
	return errors.WithStack(store.SetJSON(ctx, SettingsKey, s))
}
