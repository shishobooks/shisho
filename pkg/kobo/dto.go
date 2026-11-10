package kobo

import (
	"encoding/json"
	"time"
)

// All types use PascalCase JSON to match Kobo's API expectations.

// NewEntitlement represents a book added to the device in the sync response.
type NewEntitlement struct {
	NewEntitlement *EntitlementWrapper `json:"NewEntitlement"`
}

// ChangedEntitlement represents a book already on the device whose file or
// metadata changed. It carries the full entitlement and metadata under the
// ids the book was first sent with, so the Kobo re-downloads it in place and
// keeps the reading position. A NewEntitlement here would add a second copy.
type ChangedEntitlement struct {
	ChangedEntitlement *EntitlementWrapper `json:"ChangedEntitlement"`
}

// RemovedEntitlement represents a removed book in the sync response. Kobo
// reads removals from the same ChangedEntitlement key as changes.
type RemovedEntitlement struct {
	ChangedEntitlement *EntitlementChangeWrapper `json:"ChangedEntitlement"`
}

// EntitlementWrapper wraps the book entitlement and metadata, and for a book
// new to the Kobo that someone has read or tracked, its reading state.
type EntitlementWrapper struct {
	BookEntitlement *BookEntitlement `json:"BookEntitlement"`
	BookMetadata    *BookMetadata    `json:"BookMetadata"`
	ReadingState    *ReadingState    `json:"ReadingState,omitempty"`
}

// EntitlementChangeWrapper wraps a change (e.g., removal) to an entitlement.
// BookMetadata carries a stub for removed books — some Kobo firmware versions
// require a metadata object even for removals to deindex cleanly.
type EntitlementChangeWrapper struct {
	BookEntitlement *BookEntitlementChange `json:"BookEntitlement"`
	BookMetadata    *BookMetadata          `json:"BookMetadata,omitempty"`
}

// BookEntitlement contains the full entitlement info for a book.
type BookEntitlement struct {
	Accessibility       string        `json:"Accessibility"`
	ActivePeriod        *ActivePeriod `json:"ActivePeriod"`
	Created             time.Time     `json:"Created"`
	CrossRevisionID     string        `json:"CrossRevisionId"`
	ID                  string        `json:"Id"`
	IsHiddenFromArchive bool          `json:"IsHiddenFromArchive"`
	IsLocked            bool          `json:"IsLocked"`
	IsRemoved           bool          `json:"IsRemoved"`
	LastModified        time.Time     `json:"LastModified"`
	OriginCategory      string        `json:"OriginCategory"`
	RevisionID          string        `json:"RevisionId"`
	Status              string        `json:"Status"`
}

// BookEntitlementChange contains only the fields needed for a change notification.
type BookEntitlementChange struct {
	ID        string `json:"Id"`
	IsRemoved bool   `json:"IsRemoved"`
}

// ActivePeriod indicates when the entitlement became active.
type ActivePeriod struct {
	From time.Time `json:"From"`
}

// BookMetadata contains the metadata for a book visible to the Kobo device.
type BookMetadata struct {
	Categories          []string           `json:"Categories"`
	ContributorRoles    []*ContributorRole `json:"ContributorRoles"`
	Contributors        []string           `json:"Contributors"`
	CoverImageID        string             `json:"CoverImageId"`
	CrossRevisionID     string             `json:"CrossRevisionId"`
	CurrentDisplayPrice *DisplayPrice      `json:"CurrentDisplayPrice"`
	Description         string             `json:"Description"`
	DownloadUrls        []*DownloadURL     `json:"DownloadUrls"`
	EntitlementID       string             `json:"EntitlementId"`
	Genre               string             `json:"Genre"`
	Language            string             `json:"Language"`
	PublicationDate     string             `json:"PublicationDate,omitempty"`
	Publisher           *Publisher         `json:"Publisher,omitempty"`
	RevisionID          string             `json:"RevisionId"`
	Series              *Series            `json:"Series,omitempty"`
	SubTitle            string             `json:"SubTitle,omitempty"`
	Title               string             `json:"Title"`
	WorkID              string             `json:"WorkId"`
}

// DisplayPrice represents the price display for a book.
type DisplayPrice struct {
	CurrencyCode string `json:"CurrencyCode"`
	TotalAmount  int    `json:"TotalAmount"`
}

// ContributorRole represents an author/contributor.
type ContributorRole struct {
	Name string `json:"Name"`
}

// DownloadURL provides the download location for a book file.
type DownloadURL struct {
	Format   string `json:"Format"`
	Platform string `json:"Platform"`
	Size     int64  `json:"Size"`
	URL      string `json:"Url"`
}

// Publisher represents a book publisher.
type Publisher struct {
	Name string `json:"Name"`
}

// Series represents a book series.
type Series struct {
	Name        string  `json:"Name"`
	Number      float64 `json:"Number"`
	NumberFloat float64 `json:"NumberFloat"`
}

// syncItemLimit caps the number of entitlement entries returned per sync
// response. Larger libraries are paged across multiple sync requests using
// the X-Kobo-Sync: continue header and the OngoingSyncPointID/Cursor token
// fields.
const syncItemLimit = 100

// SyncToken is the base64-encoded JSON sent/received in X-Kobo-SyncToken header.
//
// On a fresh sync only LastSyncPointID is set (or nothing on the very first
// sync). When pagination is in flight, OngoingSyncPointID identifies the
// in-progress snapshot, PrevSyncPointID is the baseline we're diffing against
// (frozen for the duration of the pagination), and Cursor is the offset into
// the combined Added/Changed/Removed list that we've already emitted.
//
// All fields use omitempty. Tokens written by an older server (which only
// emitted LastSyncPointID) round-trip cleanly through this struct: missing
// pagination fields decode as their zero values, OngoingSyncPointID stays
// empty, and resolveSyncPoint routes the request through the fresh-sync path
// using LastSyncPointID as the prev baseline.
type SyncToken struct {
	LastSyncPointID    string `json:"lastSyncPointId,omitempty"`
	OngoingSyncPointID string `json:"ongoingSyncPointId,omitempty"`
	PrevSyncPointID    string `json:"prevSyncPointId,omitempty"`
	Cursor             int    `json:"cursor,omitempty"`
}

// DeviceAuthRequest is the body sent by the Kobo to POST /v1/auth/device.
type DeviceAuthRequest struct {
	AffiliateName string `json:"AffiliateName"`
	AppVersion    string `json:"AppVersion"`
	ClientKey     string `json:"ClientKey"`
	DeviceID      string `json:"DeviceId"`
	PlatformID    string `json:"PlatformId"`
}

// DeviceAuthResponse is returned by POST /v1/auth/device.
type DeviceAuthResponse struct {
	AccessToken  string `json:"AccessToken"`
	RefreshToken string `json:"RefreshToken"`
	TokenType    string `json:"TokenType"`
	TrackingID   string `json:"TrackingId"`
	UserKey      string `json:"UserKey"`
}

// Timestamp is a time as a Kobo writes it: in UTC, to the second, or from
// some firmware with up to seven fractional digits or without a zone, which
// is UTC.
type Timestamp struct {
	time.Time
}

// timestampLayout is how Shisho writes times to a Kobo.
const timestampLayout = "2006-01-02T15:04:05Z"

// NewTimestamp returns t as a Timestamp.
func NewTimestamp(t time.Time) *Timestamp {
	return &Timestamp{Time: t.UTC()}
}

// MarshalJSON writes t in UTC to the second.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.UTC().Format(timestampLayout) + `"`), nil
}

// UnmarshalJSON reads a Kobo's time. An empty or unreadable one is the
// zero time, which callers treat as absent, so a firmware writing times
// another way never fails the request.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil || s == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	t.Time = time.Time{}
	return nil
}

// at returns t's time, or nil when t is absent.
func (t *Timestamp) at() *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	at := t.Time
	return &at
}

// Statuses a Kobo reports for a book in StatusInfo.
const (
	koboStatusReadyToRead = "ReadyToRead"
	koboStatusReading     = "Reading"
	koboStatusFinished    = "Finished"
)

// ReadingState is a Kobo's reading state for one book: its status, the time
// spent reading it, and its bookmark. Each part has its own LastModified,
// and a Kobo keeps whichever of its own and the server's is later.
type ReadingState struct {
	EntitlementID     string           `json:"EntitlementId"`
	Created           *Timestamp       `json:"Created,omitempty"`
	LastModified      *Timestamp       `json:"LastModified,omitempty"`
	PriorityTimestamp *Timestamp       `json:"PriorityTimestamp,omitempty"`
	StatusInfo        *StatusInfo      `json:"StatusInfo,omitempty"`
	Statistics        *Statistics      `json:"Statistics,omitempty"`
	CurrentBookmark   *CurrentBookmark `json:"CurrentBookmark,omitempty"`
}

// StatusInfo is whether a book is unread, being read, or finished.
type StatusInfo struct {
	LastModified           *Timestamp `json:"LastModified,omitempty"`
	Status                 string     `json:"Status"`
	TimesStartedReading    *int       `json:"TimesStartedReading,omitempty"`
	LastTimeStartedReading *Timestamp `json:"LastTimeStartedReading,omitempty"`
}

// Statistics is the time a Kobo has spent on a book, in minutes since it
// was put on the Kobo, and the time it estimates is left.
type Statistics struct {
	LastModified         *Timestamp `json:"LastModified,omitempty"`
	SpentReadingMinutes  *int       `json:"SpentReadingMinutes,omitempty"`
	RemainingTimeMinutes *int       `json:"RemainingTimeMinutes,omitempty"`
}

// CurrentBookmark is where a Kobo is in a book: how far through the book and
// the chapter, from 0 to 100, and in a KePub, the span it's at.
type CurrentBookmark struct {
	LastModified                 *Timestamp        `json:"LastModified,omitempty"`
	ProgressPercent              *float64          `json:"ProgressPercent,omitempty"`
	ContentSourceProgressPercent *float64          `json:"ContentSourceProgressPercent,omitempty"`
	Location                     *BookmarkLocation `json:"Location,omitempty"`
}

// BookmarkLocation is a span in a KePub: Value is its id, and Source the
// chapter's path in the KePub.
type BookmarkLocation struct {
	Value  string `json:"Value"`
	Type   string `json:"Type"`
	Source string `json:"Source"`
}

// koboSpanLocation is the Type of a KePub span location.
const koboSpanLocation = "KoboSpan"

// UpdateReadingStatesPayload is a Kobo's PUT of a book's reading state.
type UpdateReadingStatesPayload struct {
	ReadingStates []*ReadingState `json:"ReadingStates"`
}

// UpdateReadingStatesResponse answers a reading state PUT, with a result for
// each part the Kobo sent.
type UpdateReadingStatesResponse struct {
	RequestResult string                      `json:"RequestResult"`
	UpdateResults []*ReadingStateUpdateResult `json:"UpdateResults"`
}

// ReadingStateUpdateResult is the result of one book's PUT.
type ReadingStateUpdateResult struct {
	EntitlementID         string        `json:"EntitlementId"`
	CurrentBookmarkResult *UpdateResult `json:"CurrentBookmarkResult,omitempty"`
	StatisticsResult      *UpdateResult `json:"StatisticsResult,omitempty"`
	StatusInfoResult      *UpdateResult `json:"StatusInfoResult,omitempty"`
}

// UpdateResult is the result of one part of a PUT.
type UpdateResult struct {
	Result string `json:"Result"`
}

// updateSucceeded is the result of everything Shisho accepts.
const updateSucceeded = "Success"

// ChangedReadingState is a sync item carrying a book's reading state, sent
// when another reader changed it since the Kobo last heard.
type ChangedReadingState struct {
	ChangedReadingState *ReadingStateWrapper `json:"ChangedReadingState"`
}

// ReadingStateWrapper wraps a reading state in a sync item.
type ReadingStateWrapper struct {
	ReadingState *ReadingState `json:"ReadingState"`
}
