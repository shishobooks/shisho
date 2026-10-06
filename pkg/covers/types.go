package covers

const (
	//tygo:emit export type ThumbnailSize = typeof ThumbnailSize128 | typeof ThumbnailSize256 | typeof ThumbnailSize512 | typeof ThumbnailSize1024 | typeof ThumbnailSize2048;
	ThumbnailSize128  = 128
	ThumbnailSize256  = 256
	ThumbnailSize512  = 512
	ThumbnailSize1024 = 1024
	ThumbnailSize2048 = 2048
)

const (
	//tygo:emit export type ThumbnailAspect = typeof ThumbnailAspectBook | typeof ThumbnailAspectSquare;
	ThumbnailAspectBook   = "book"
	ThumbnailAspectSquare = "square"
)

// ThumbnailRenderKey changes when resizing or encoding settings change.
const ThumbnailRenderKey = "1"

// CoverThumbnailQuery describes optional resizing on API cover endpoints.
// Omitting Size serves the original. An omitted Aspect preserves its proportions.
type CoverThumbnailQuery struct {
	Size      int    `query:"size" json:"size,omitempty" tstype:"ThumbnailSize"`
	Aspect    string `query:"aspect" json:"aspect,omitempty" tstype:"ThumbnailAspect"`
	RenderKey string `query:"r" json:"r,omitempty"`
}
