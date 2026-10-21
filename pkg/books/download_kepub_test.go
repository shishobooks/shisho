package books

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A device or Share recipient given the original of a bare .kepub gets it
// named .kepub.epub, the only name Kobo's reader and KOReader take as a
// KePub. Download Original keeps the file's own name.
func TestResolveFallbackDownload_BareKePubIsNamedKePubEPUB(t *testing.T) {
	t.Parallel()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	file := &models.File{
		FileType: models.FileTypeKePub,
		FileRole: models.FileRoleSupplement,
		Filepath: "/lib/Book/Book - Author.kepub",
	}

	download, err := ResolveFallbackDownload(c, nil, &models.Book{}, file)
	require.NoError(t, err)
	assert.Equal(t, "Book - Author.kepub.epub", download.Filename)
	assert.Equal(t, "/lib/Book/Book - Author.kepub", download.Path)

	assert.Equal(t, "Book - Author.kepub", OriginalDownload(file).Filename)
}
