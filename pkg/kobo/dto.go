package kobo

import "time"

// All types use PascalCase JSON to match Kobo's API expectations.

// NewEntitlement represents a new or changed book in the sync response.
type NewEntitlement struct {
	NewEntitlement *EntitlementWrapper `json:"NewEntitlement"`
}

// ChangedEntitlement represents a removed book in the sync response.
type ChangedEntitlement struct {
	ChangedEntitlement *EntitlementChangeWrapper `json:"ChangedEntitlement"`
}

// EntitlementWrapper wraps the book entitlement and metadata.
type EntitlementWrapper struct {
	BookEntitlement *BookEntitlement `json:"BookEntitlement"`
	BookMetadata    *BookMetadata    `json:"BookMetadata"`
}

// EntitlementChangeWrapper wraps a change (e.g., removal) to an entitlement.
type EntitlementChangeWrapper struct {
	BookEntitlement *BookEntitlementChange `json:"BookEntitlement"`
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

// SyncToken is the base64-encoded JSON sent/received in X-Kobo-SyncToken header.
type SyncToken struct {
	LastSyncPointID string `json:"lastSyncPointId"`
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
