package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
)

func testLogger() logger.Logger {
	return logger.New()
}

func TestDownloadCoverFromURL_Success(t *testing.T) {
	t.Parallel()

	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/cover.jpg"}
	ok := downloadCoverFromURL(md, testLogger())

	assert.True(t, ok)
	assert.Equal(t, fakeJPEG, md.CoverData)
	assert.Equal(t, "image/jpeg", md.CoverMimeType)
}

func TestDownloadCoverFromURL_CoverDataTakesPrecedence(t *testing.T) {
	t.Parallel()

	existing := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	md := &mediafile.ParsedMetadata{
		CoverData:     existing,
		CoverMimeType: "image/jpeg",
		CoverURL:      "https://should-not-be-called.example.com/cover.jpg",
	}

	ok := downloadCoverFromURL(md, testLogger())

	assert.False(t, ok, "should skip when CoverData already set")
	assert.Equal(t, existing, md.CoverData, "CoverData should be unchanged")
}

func TestDownloadCoverFromURL_NonImageRejected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>Not an image</html>"))
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/not-image"}
	ok := downloadCoverFromURL(md, testLogger())

	assert.False(t, ok, "should reject non-image content type")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_EmptyURL(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{}
	ok := downloadCoverFromURL(md, testLogger())

	assert.False(t, ok, "should return false for empty URL")
}

func TestDownloadCoverFromURL_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/error"}
	ok := downloadCoverFromURL(md, testLogger())

	assert.False(t, ok, "should return false on server error")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}
