package audnexus

// Chapter is a single chapter from Audnexus.
type Chapter struct {
	Title         string `json:"title"`
	StartOffsetMs int64  `json:"start_offset_ms"`
	LengthMs      int64  `json:"length_ms"`
}

// Response is the normalized chapters response returned by the service and
// passed through to the frontend. Field names are snake_case per project
// API conventions; upstream Audnexus uses camelCase and is converted at the
// parse boundary.
type Response struct {
	ASIN                 string    `json:"asin"`
	IsAccurate           bool      `json:"is_accurate"`
	RuntimeLengthMs      int64     `json:"runtime_length_ms"`
	BrandIntroDurationMs int64     `json:"brand_intro_duration_ms"`
	BrandOutroDurationMs int64     `json:"brand_outro_duration_ms"`
	Chapters             []Chapter `json:"chapters"`
}

// audnexusUpstream matches the Audnexus API camelCase JSON shape for decoding.
type audnexusUpstream struct {
	ASIN                 string              `json:"asin"`
	IsAccurate           bool                `json:"isAccurate"`
	RuntimeLengthMs      int64               `json:"runtimeLengthMs"`
	BrandIntroDurationMs int64               `json:"brandIntroDurationMs"`
	BrandOutroDurationMs int64               `json:"brandOutroDurationMs"`
	Chapters             []audnexusUpChapter `json:"chapters"`
}

type audnexusUpChapter struct {
	Title         string `json:"title"`
	StartOffsetMs int64  `json:"startOffsetMs"`
	LengthMs      int64  `json:"lengthMs"`
}
