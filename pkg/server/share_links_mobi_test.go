package server

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertMOBI adds a main MOBI or AZW3 file with the given bytes to book.
func (f *shareLinksFixture) insertMOBI(ctx context.Context, book *models.Book, name, fileType string, content []byte) *models.File {
	f.t.Helper()
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      fileType,
		FileRole:      models.FileRoleMain,
		Filepath:      testgen.WriteFile(f.t, book.Filepath, name, content),
		FilesizeBytes: int64(len(content)),
	}
	_, err := f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(f.t, err)
	return file
}

// A Share Link serves a MOBI or AZW3 with the book's metadata written in,
// and the original when the file cannot be generated.
func TestShareLinks_MOBIDownloads(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	ctx := context.Background()

	content := testgen.BuildMOBI(t, testgen.MOBIOptions{Kind: testgen.MOBIKindCombo, Title: "Title In File", HasCover: true})
	mobiFile := f.insertMOBI(ctx, f.bookA, "Alpha.mobi", models.FileTypeMOBI, content)
	azw3File := f.insertMOBI(ctx, f.bookA, "Alpha.azw3", models.FileTypeAZW3,
		testgen.BuildMOBI(t, testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "Title In File"}))
	// Shaped like a MOBI to the scan, but its header record is cut short.
	broken := append([]byte(nil), content[:200]...)
	brokenFile := f.insertMOBI(ctx, f.bookA, "Broken.mobi", models.FileTypeMOBI, broken)
	link := f.mustCreate(f.admin, f.bookA, `{}`)

	for _, file := range []*models.File{mobiFile, azw3File} {
		rec := f.shareGet(link.Token, shareFilePath(file, "/download"))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, models.FileTypeMimeType(file.FileType), rec.Header().Get("Content-Type"))
		meta, err := mobi.Parse(testgen.WriteFile(t, t.TempDir(), filepath.Base(file.Filepath), rec.Body.Bytes()))
		require.NoError(t, err)
		assert.Equal(t, "Alpha", meta.Title)
	}

	rec := f.shareGet(link.Token, shareFilePath(brokenFile, "/download"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, bytes.Equal(broken, rec.Body.Bytes()), "a file that cannot be generated is served as is")
}
