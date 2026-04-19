package plugins

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePersistTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

// stubBookStoreForPersist is a minimal bookStore implementation used by
// persistMetadata cover-write tests. Only UpdateFile and RetrieveBook
// need to produce meaningful results for the cover-only persist path.
type stubBookStoreForPersist struct {
	book *models.Book
}

func (s *stubBookStoreForPersist) UpdateBook(_ context.Context, _ *models.Book, _ []string) error {
	return nil
}

func (s *stubBookStoreForPersist) RetrieveBook(_ context.Context, _ int) (*models.Book, error) {
	return s.book, nil
}

func (s *stubBookStoreForPersist) UpdateFile(_ context.Context, _ *models.File, _ []string) error {
	return nil
}

func (s *stubBookStoreForPersist) DeleteNarratorsForFile(_ context.Context, _ int) (int, error) {
	return 0, nil
}

func (s *stubBookStoreForPersist) CreateNarrator(_ context.Context, _ *models.Narrator) error {
	return nil
}

func (s *stubBookStoreForPersist) OrganizeBookFiles(_ context.Context, _ *models.Book) error {
	return nil
}

// TestPersistMetadata_CoverWrite_RootLevelFile_SyntheticBookPath is a
// regression test for a bug where persistMetadata wrote plugin-provided
// cover data unconditionally to book.Filepath as the cover directory. For
// root-level new files, scanFileCreateNew computes a synthetic organized-
// folder bookPath that does not exist on disk, so os.WriteFile failed with
// "no such file or directory" and the cover was silently dropped. The fix
// uses fileutils.ResolveCoverDirForWrite, which falls back to the file's
// parent directory when book.Filepath does not resolve to a real directory.
func TestPersistMetadata_CoverWrite_RootLevelFile_SyntheticBookPath(t *testing.T) {
	t.Parallel()

	// A real file lives at the library root.
	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	// Synthetic book.Filepath that does NOT exist on disk, mirroring what
	// scanFileCreateNew computes for root-level new files in libraries with
	// OrganizeFileStructure enabled.
	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	_, err := os.Stat(syntheticBookPath)
	require.True(t, os.IsNotExist(err), "synthetic bookPath must not exist on disk")

	file := &models.File{
		ID:       1,
		BookID:   1,
		Filepath: filePath,
		FileType: models.FileTypeEPUB,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  syntheticBookPath,
		Files:     []*models.File{file},
	}

	h := &handler{
		enrich: &enrichDeps{
			bookStore: &stubBookStoreForPersist{book: book},
		},
	}

	md := &mediafile.ParsedMetadata{
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	// persistMetadata will also attempt to write book/file sidecars under
	// the synthetic bookPath and log warnings when those writes fail. That
	// is expected: the test deliberately uses a nonexistent bookPath, and
	// sidecar failures are non-fatal. The assertions below only cover the
	// cover-write path.
	err = h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// Cover must land next to the file in the library dir, not under the
	// nonexistent synthetic organized-folder path.
	expectedCoverPath := filepath.Join(libraryDir, "book.epub.cover.jpg")
	_, err = os.Stat(expectedCoverPath)
	require.NoError(t, err, "cover should be written next to the file at library dir")

	// Nothing should have been written under the synthetic path.
	_, err = os.Stat(syntheticBookPath)
	assert.True(t, os.IsNotExist(err), "synthetic bookPath must still not exist after persist")

	// CoverImageFilename should be set (to the filename only, not a full path).
	require.NotNil(t, file.CoverImageFilename, "CoverImageFilename should be set on the file")
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
}

// stubPageExtractor records calls and returns a fixed (filename, mimeType).
// Set `wantErr` to simulate a failed extraction.
type stubPageExtractor struct {
	calls    []stubPageExtractorCall
	filename string
	mimeType string
	wantErr  error
}

type stubPageExtractorCall struct {
	FileID       int
	BookFilepath string
	Page         int
}

func (s *stubPageExtractor) ExtractCoverPage(file *models.File, bookFilepath string, page int, _ logger.Logger) (string, string, error) {
	s.calls = append(s.calls, stubPageExtractorCall{FileID: file.ID, BookFilepath: bookFilepath, Page: page})
	if s.wantErr != nil {
		return "", "", s.wantErr
	}
	return s.filename, s.mimeType, nil
}

func TestPersistMetadata_CoverPage_CBZ_HappyPath(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID:        1,
		BookID:    1,
		Filepath:  filePath,
		FileType:  models.FileTypeCBZ,
		PageCount: &pageCount,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  libraryDir,
		Files:     []*models.File{file},
	}

	extractor := &stubPageExtractor{filename: "comic.cbz.cover.jpg", mimeType: "image/jpeg"}

	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			pageExtractor: extractor,
		},
	}

	page := 3
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	require.Len(t, extractor.calls, 1)
	assert.Equal(t, 1, extractor.calls[0].FileID)
	assert.Equal(t, 3, extractor.calls[0].Page)

	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 3, *file.CoverPage)
	require.NotNil(t, file.CoverImageFilename)
	assert.Equal(t, "comic.cbz.cover.jpg", *file.CoverImageFilename)
	require.NotNil(t, file.CoverMimeType)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	require.NotNil(t, file.CoverSource)
	assert.Equal(t, models.PluginDataSource("test", "plugin-id"), *file.CoverSource)
}
