package kobo

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Kobo cover that exists but cannot be read is a server fault, not a 404
// that tells the device the book has no cover. The stat fails when the
// book's directory cannot be searched; the open fails when the cover file
// itself cannot be read, whether the device asks for a resized copy or the
// original (which echo's c.File would turn into a 404).
func TestHandleCover_UnreadableCoverIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}

	tests := []struct {
		name string
		// lock names the path whose read access is removed, and the mode
		// that restores it.
		lock func(bookDir, coverPath string) (string, os.FileMode)
		w, h string
	}{
		{"stat", func(bookDir, _ string) (string, os.FileMode) { return bookDir, 0o755 }, "0", "0"},
		{"open resized", func(_, coverPath string) (string, os.FileMode) { return coverPath, 0o644 }, "100", "150"},
		{"open original", func(_, coverPath string) (string, os.FileMode) { return coverPath, 0o644 }, "0", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := newSyncPointTestDB(t)
			ctx := context.Background()

			library := &models.Library{Name: "Test Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
			_, err := db.NewInsert().Model(library).Exec(ctx)
			require.NoError(t, err)
			bookDir := filepath.Join(t.TempDir(), "Test Book")
			require.NoError(t, os.MkdirAll(bookDir, 0o755))
			book := &models.Book{
				LibraryID: library.ID, Title: "Test Book", TitleSource: models.DataSourceFilepath,
				SortTitle: "Test Book", SortTitleSource: models.DataSourceFilepath,
				AuthorSource: models.DataSourceFilepath, Filepath: bookDir,
			}
			_, err = db.NewInsert().Model(book).Exec(ctx)
			require.NoError(t, err)

			filePath := filepath.Join(bookDir, "test.epub")
			require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))
			coverFilename := "test.epub.cover.jpg"
			coverPath := filepath.Join(bookDir, coverFilename)
			coverFile, err := os.Create(coverPath)
			require.NoError(t, err)
			require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 100, 150)), nil))
			require.NoError(t, coverFile.Close())
			mimeType := "image/jpeg"
			file := &models.File{
				LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
				Filepath: filePath, FilesizeBytes: 1000, CoverImageFilename: &coverFilename, CoverMimeType: &mimeType,
			}
			_, err = db.NewInsert().Model(file).Exec(ctx)
			require.NoError(t, err)

			lockedPath, restoreMode := tt.lock(bookDir, coverPath)
			require.NoError(t, os.Chmod(lockedPath, 0o000))
			t.Cleanup(func() { _ = os.Chmod(lockedPath, restoreMode) })

			h := &handler{service: NewService(db), bookService: books.NewService(db)}
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			withKeyOwner(c)
			c.SetParamNames("imageId", "w", "h")
			c.SetParamValues(fmt.Sprintf("shisho-%d", file.ID), tt.w, tt.h)

			err = h.handleCover(c)
			require.Error(t, err)
			var codeErr *errcodes.Error
			assert.NotErrorAs(t, err, &codeErr, "want a server fault, got %v", err)
			var httpErr *echo.HTTPError
			assert.NotErrorAs(t, err, &httpErr, "want a server fault, not echo's 404")
		})
	}
}
