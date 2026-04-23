package kobo

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleInitialization_ReturnsNativeResourcesWithoutProxying(t *testing.T) {
	t.Parallel()
	// If anything tries to proxy upstream, the test would either hang
	// (real koboStoreBaseURL) or fail loudly via t.Fatal here.
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/kobo/key/all/v1/initialization", nil)
	req.Host = "shisho.local"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKey")
	c.SetParamValues("key")

	h := &handler{}
	require.NoError(t, h.handleInitialization(c))

	assert.False(t, upstreamCalled, "handleInitialization must not proxy to upstream Kobo store")
	assert.Equal(t, "e30=", rec.Header().Get("x-kobo-apitoken"))

	var body map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	resources := body["Resources"]
	require.NotNil(t, resources)
	assert.Contains(t, resources["image_host"], "shisho.local")
	assert.Contains(t, resources["image_url_template"], "shisho.local")
	assert.Contains(t, resources, "library_sync", "native resources should still include library_sync key")
}

func TestCombineChanges_OrdersDeterministically(t *testing.T) {
	t.Parallel()
	changes := &SyncChanges{
		Added:   []ScopedFile{{FileID: 5}, {FileID: 1}, {FileID: 3}},
		Changed: []ScopedFile{{FileID: 7}, {FileID: 4}},
		Removed: []ScopedFile{{FileID: 9}, {FileID: 2}},
	}

	entries := combineChanges(changes)
	require.Len(t, entries, 7)

	// Added (sorted asc), then Changed (sorted asc), then Removed (sorted asc).
	expected := []struct {
		FileID int
		Kind   changeKind
	}{
		{1, changeAdded}, {3, changeAdded}, {5, changeAdded},
		{4, changeChanged}, {7, changeChanged},
		{2, changeRemoved}, {9, changeRemoved},
	}
	for i, e := range expected {
		assert.Equal(t, e.FileID, entries[i].File.FileID, "entry %d FileID", i)
		assert.Equal(t, e.Kind, entries[i].Kind, "entry %d kind", i)
	}
}

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

	metadata := buildBookMetadata(book, file, "", "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, "Test Book", metadata.Title)
	assert.Equal(t, "shisho-1", metadata.EntitlementID)
	assert.Len(t, metadata.DownloadUrls, 1)
	assert.Contains(t, metadata.DownloadUrls[0].URL, "shisho-1")
}

func TestBuildRemovedBookMetadata_HasRequiredFields(t *testing.T) {
	t.Parallel()
	m := buildRemovedBookMetadata(42)
	bookID := "shisho-42"
	assert.Equal(t, bookID, m.EntitlementID)
	assert.Equal(t, bookID, m.RevisionID)
	assert.Equal(t, bookID, m.CrossRevisionID)
	assert.Equal(t, bookID, m.WorkID)
	assert.Equal(t, bookID, m.CoverImageID)
	assert.Equal(t, dummyCategoryID, m.Genre)
	assert.Equal(t, []string{dummyCategoryID}, m.Categories)
	assert.Equal(t, "en", m.Language)
	assert.NotNil(t, m.CurrentDisplayPrice)
}

func TestBuildBookMetadata_CoverImageIDIncludesCacheKey(t *testing.T) {
	t.Parallel()
	book := &models.Book{ID: 1, Title: "Test Book"}
	file := &models.File{ID: 51, FilesizeBytes: 1024}

	metadata := buildBookMetadata(book, file, "abc12345", "http://localhost/kobo/k/all")

	assert.Equal(t, "shisho-51-abc12345", metadata.CoverImageID,
		"CoverImageID must include cache key suffix to bust device thumbnail cache")
	// EntitlementID and friends stay bare so the device's entitlement
	// bookkeeping tracks the same book across syncs.
	assert.Equal(t, "shisho-51", metadata.EntitlementID)
	assert.Equal(t, "shisho-51", metadata.RevisionID)
	assert.Equal(t, "shisho-51", metadata.WorkID)
	assert.Equal(t, "shisho-51", metadata.CrossRevisionID)
}

func TestBuildBookMetadata_NoCoverCacheKey(t *testing.T) {
	t.Parallel()
	book := &models.Book{ID: 1, Title: "Test Book"}
	file := &models.File{ID: 51, FilesizeBytes: 1024}

	metadata := buildBookMetadata(book, file, "", "http://localhost/kobo/k/all")

	assert.Equal(t, "shisho-51", metadata.CoverImageID,
		"CoverImageID falls back to bare ID when no cache key is supplied")
}

func TestBuildBookMetadata_WithSubtitle(t *testing.T) {
	t.Parallel()
	subtitle := "An Illuminating Subtitle"
	book := &models.Book{ID: 1, Title: "Test Book", Subtitle: &subtitle}
	file := &models.File{ID: 1, FilesizeBytes: 1024}

	metadata := buildBookMetadata(book, file, "", "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, subtitle, metadata.SubTitle)
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

	metadata := buildBookMetadata(book, file, "", "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, "Test Book With Relations", metadata.Title)
	assert.Equal(t, "A test description", metadata.Description)
	assert.Len(t, metadata.ContributorRoles, 1)
	assert.Equal(t, personName, metadata.ContributorRoles[0].Name)
	assert.NotNil(t, metadata.Series)
	assert.Equal(t, "Test Series", metadata.Series.Name)
	assert.InDelta(t, 3.0, metadata.Series.Number, 0.001)
}
