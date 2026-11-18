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
	//tygo:emit export type ReflowableTheme = typeof ReflowableThemeLight | typeof ReflowableThemeDark | typeof ReflowableThemeSepia;
	ReflowableThemeLight = "light"
	ReflowableThemeDark  = "dark"
	ReflowableThemeSepia = "sepia"
)

const (
	//tygo:emit export type ReflowableFlow = typeof ReflowableFlowPaginated | typeof ReflowableFlowScrolled;
	ReflowableFlowPaginated = "paginated"
	ReflowableFlowScrolled  = "scrolled"
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
	ReflowableFontSize int       `bun:"viewer_reflowable_font_size,notnull,default:100" json:"viewer_reflowable_font_size"`
	ReflowableTheme    string    `bun:"viewer_reflowable_theme,notnull,default:'light'" json:"viewer_reflowable_theme" tstype:"ReflowableTheme"`
	ReflowableFlow     string    `bun:"viewer_reflowable_flow,notnull,default:'paginated'" json:"viewer_reflowable_flow" tstype:"ReflowableFlow"`
	GallerySize        string    `bun:",notnull,default:'m'" json:"gallery_size" tstype:"GallerySize"`
	ViewerHideChrome   bool      `bun:",notnull,default:false" json:"viewer_hide_chrome"`
	// DefaultLibraryID is the library `/` opens. Nil opens the first library
	// the user can access.
	DefaultLibraryID *int `json:"default_library_id" tstype:"number | null"`
	// TimeZone is the IANA zone that dates the user's automatic Read-through
	// days. Nil until the browser fills it in; days are then dated in UTC.
	TimeZone *string `json:"time_zone" tstype:"string | null"`
}

// DefaultUserSettings returns a UserSettings with default values.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		ViewerPreloadCount: 3,
		ViewerFitMode:      FitModeHeight,
		ReflowableFontSize: 100,
		ReflowableTheme:    ReflowableThemeLight,
		ReflowableFlow:     ReflowableFlowPaginated,
		GallerySize:        GallerySizeMedium,
		ViewerHideChrome:   false,
	}
}
