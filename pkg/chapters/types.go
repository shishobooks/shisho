package chapters

import "github.com/shishobooks/shisho/pkg/models"

// ChaptersResponse is the response body for the chapter list and replace
// endpoints. ListChapters returns a nested tree, so each chapter carries its
// Children populated.
type ChaptersResponse struct {
	Chapters []*models.Chapter `json:"chapters" tstype:"Chapter[]"`
}

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
