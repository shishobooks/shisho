package kobo

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncToken_Encode(t *testing.T) {
	t.Parallel()
	token := SyncToken{LastSyncPointID: "test-id-123"}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(tokenJSON)

	// Verify it can be decoded
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	var parsed SyncToken
	err = json.Unmarshal(decoded, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test-id-123", parsed.LastSyncPointID)
}

func TestFitDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		srcW, srcH       int
		maxW, maxH       int
		expectW, expectH int
	}{
		{"smaller than max", 100, 150, 200, 300, 100, 150},
		{"width constrained", 400, 300, 200, 300, 200, 150},
		{"height constrained", 300, 600, 300, 400, 200, 400},
		{"both constrained (width wins)", 800, 600, 400, 400, 400, 300},
		{"both constrained (height wins)", 600, 800, 400, 400, 300, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := fitDimensions(tt.srcW, tt.srcH, tt.maxW, tt.maxH)
			assert.Equal(t, tt.expectW, w, "width")
			assert.Equal(t, tt.expectH, h, "height")
		})
	}
}

func TestBuildBookMetadata(t *testing.T) {
	t.Parallel()
	// This tests that buildBookMetadata doesn't panic with minimal data
	book := &models.Book{
		ID:    1,
		Title: "Test Book",
	}
	file := &models.File{
		ID:            1,
		FilesizeBytes: 1024,
	}

	metadata := buildBookMetadata(book, file, "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, "Test Book", metadata.Title)
	assert.Equal(t, "shisho-1", metadata.EntitlementID)
	assert.Len(t, metadata.DownloadUrls, 1)
	assert.Contains(t, metadata.DownloadUrls[0].URL, "shisho-1")
}

func TestBuildBookMetadata_WithRelations(t *testing.T) {
	t.Parallel()
	personName := "Test Author"
	description := "A test description"
	seriesNumber := 3.0

	book := &models.Book{
		ID:          1,
		Title:       "Test Book With Relations",
		Description: &description,
		Authors: []*models.Author{
			{Person: &models.Person{Name: personName}},
		},
		BookSeries: []*models.BookSeries{
			{
				Series:       &models.Series{Name: "Test Series"},
				SeriesNumber: &seriesNumber,
			},
		},
	}
	file := &models.File{
		ID:            1,
		FilesizeBytes: 2048,
	}

	metadata := buildBookMetadata(book, file, "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, "Test Book With Relations", metadata.Title)
	assert.Equal(t, "A test description", metadata.Description)
	assert.Len(t, metadata.ContributorRoles, 1)
	assert.Equal(t, personName, metadata.ContributorRoles[0].Name)
	assert.NotNil(t, metadata.Series)
	assert.Equal(t, "Test Series", metadata.Series.Name)
	assert.InDelta(t, 3.0, metadata.Series.Number, 0.001)
}
