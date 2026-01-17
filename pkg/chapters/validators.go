package chapters

// ChapterInput represents a chapter in API requests.
type ChapterInput struct {
	Title            string         `json:"title"`
	StartPage        *int           `json:"start_page"`
	StartTimestampMs *int64         `json:"start_timestamp_ms"`
	Href             *string        `json:"href"`
	Children         []ChapterInput `json:"children"`
}

// ReplaceChaptersPayload is the request body for replacing chapters.
type ReplaceChaptersPayload struct {
	Chapters []ChapterInput `json:"chapters"`
}
