package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FitMode = typeof FitModeHeight | typeof FitModeOriginal;
	FitModeHeight   = "fit-height"
	FitModeOriginal = "original"
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
	}
}
