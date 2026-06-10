package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FitMode = typeof FitModeHeight | typeof FitModeWidth;
	FitModeHeight = "fit-height"
	FitModeWidth  = "fit-width"
)

const (
	//tygo:emit export type EpubTheme = typeof EpubThemeLight | typeof EpubThemeDark | typeof EpubThemeSepia;
	EpubThemeLight = "light"
	EpubThemeDark  = "dark"
	EpubThemeSepia = "sepia"
)

const (
	//tygo:emit export type EpubFlow = typeof EpubFlowPaginated | typeof EpubFlowScrolled;
	EpubFlowPaginated = "paginated"
	EpubFlowScrolled  = "scrolled"
)

const (
	//tygo:emit export type GallerySize = typeof GallerySizeSmall | typeof GallerySizeMedium | typeof GallerySizeLarge | typeof GallerySizeExtraLarge;
	GallerySizeSmall      = "s"
	GallerySizeMedium     = "m"
	GallerySizeLarge      = "l"
	GallerySizeExtraLarge = "xl"
)

const (
	// The tygo:emit lines mirror PlaybackSpeeds (below) into the generated
	// TS (a const array plus a union type) so the player's speed menu and
	// the backend validation share the same set of values. tygo only
	// processes const/type declarations, so the directives live here rather
	// than on the var. Keep the emitted list in sync with PlaybackSpeeds.
	//
	//tygo:emit export const PlaybackSpeeds = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3] as const;
	//tygo:emit export type PlaybackSpeed = (typeof PlaybackSpeeds)[number];
	PlaybackSpeedDefault = 1.0
)

// PlaybackSpeeds lists the allowed audiobook playback speed steps, in
// ascending order.
var PlaybackSpeeds = []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3}

type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings,alias:us" tstype:"-"`

	ID                 int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID             int       `bun:",notnull,unique" json:"user_id"`
	ViewerPreloadCount int       `bun:",notnull,default:3" json:"viewer_preload_count"`
	ViewerFitMode      string    `bun:",notnull,default:'fit-height'" json:"viewer_fit_mode" tstype:"FitMode"`
	EpubFontSize       int       `bun:"viewer_epub_font_size,notnull,default:100" json:"viewer_epub_font_size"`
	EpubTheme          string    `bun:"viewer_epub_theme,notnull,default:'light'" json:"viewer_epub_theme" tstype:"EpubTheme"`
	EpubFlow           string    `bun:"viewer_epub_flow,notnull,default:'paginated'" json:"viewer_epub_flow" tstype:"EpubFlow"`
	GallerySize        string    `bun:",notnull,default:'m'" json:"gallery_size" tstype:"GallerySize"`
	ViewerHideChrome   bool      `bun:",notnull,default:false" json:"viewer_hide_chrome"`
	PlaybackSpeed      float64   `bun:"viewer_playback_speed,notnull,default:1.0" json:"viewer_playback_speed" tstype:"PlaybackSpeed"`
}

// DefaultUserSettings returns a UserSettings with default values.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		ViewerPreloadCount: 3,
		ViewerFitMode:      FitModeHeight,
		EpubFontSize:       100,
		EpubTheme:          EpubThemeLight,
		EpubFlow:           EpubFlowPaginated,
		GallerySize:        GallerySizeMedium,
		ViewerHideChrome:   false,
		PlaybackSpeed:      PlaybackSpeedDefault,
	}
}
