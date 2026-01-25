package downloadcache

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetOrGenerate(t *testing.T) {
	t.Parallel()
	t.Run("generates new file when cache is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		// Create a test EPUB
		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Original Title", []string{"Original Author"})

		cache := NewCache(cacheDir, 1024*1024*1024) // 1GB

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Test Author"}},
			},
		}
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
		}

		cachedPath, downloadFilename, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		assert.NotEmpty(t, cachedPath)
		assert.Equal(t, "[Test Author] Test Book.epub", downloadFilename)

		// Verify the cached file exists
		_, err = os.Stat(cachedPath)
		require.NoError(t, err)

		// Verify metadata was written
		meta, err := ReadMetadata(cacheDir, file.ID)
		require.NoError(t, err)
		require.NotNil(t, meta)
		assert.Equal(t, file.ID, meta.FileID)
		assert.NotEmpty(t, meta.FingerprintHash)

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("returns cached file when cache is valid", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Test Title", []string{"Test Author"})

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Test Author"}},
			},
		}
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
		}

		// First call - generates the file
		cachedPath1, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Get the file's mod time
		info1, err := os.Stat(cachedPath1)
		require.NoError(t, err)
		modTime1 := info1.ModTime()

		// Second call - should return cached file
		cachedPath2, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Should be the same path
		assert.Equal(t, cachedPath1, cachedPath2)

		// File should not have been modified
		info2, err := os.Stat(cachedPath2)
		require.NoError(t, err)
		assert.Equal(t, modTime1, info2.ModTime())

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("regenerates file when metadata changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Test Title", []string{"Author"})

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{
			Title: "Original Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
		}

		// First call
		_, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Get the original fingerprint hash
		meta1, err := ReadMetadata(cacheDir, file.ID)
		require.NoError(t, err)
		hash1 := meta1.FingerprintHash

		// Change the title
		book.Title = "New Title"

		// Second call - should regenerate
		_, downloadFilename, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		assert.Equal(t, "[Author] New Title.epub", downloadFilename)

		// Fingerprint should have changed
		meta2, err := ReadMetadata(cacheDir, file.ID)
		require.NoError(t, err)
		assert.NotEqual(t, hash1, meta2.FingerprintHash)

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("returns error for unsupported file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{Title: "Test"}
		file := &models.File{
			ID:       1,
			FileType: "unknown",
			Filepath: "/some/path",
		}

		_, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}

func TestCache_ChapterFingerprinting(t *testing.T) {
	t.Parallel()
	t.Run("chapter changes invalidate cache", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Test Title", []string{"Author"})

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Test Author"}},
			},
		}

		// Create file with an initial chapter
		startMs := int64(0)
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
			Chapters: []*models.Chapter{
				{
					Title:            "Original Title",
					SortOrder:        0,
					StartTimestampMs: &startMs,
				},
			},
		}

		// First call - generates the file
		_, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Get the original fingerprint hash
		meta1, err := ReadMetadata(cacheDir, file.ID)
		require.NoError(t, err)
		originalHash := meta1.FingerprintHash

		// Update chapter title directly in file.Chapters model
		file.Chapters[0].Title = "Updated Title"

		// Compute new fingerprint hash
		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)
		newHash, err := fp.Hash()
		require.NoError(t, err)

		// Verify hash differs from original (cache should be invalidated)
		assert.NotEqual(t, originalHash, newHash, "Fingerprint hash should differ when chapter title changes")

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("same chapters hit cache", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Test Title", []string{"Author"})

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Test Author"}},
			},
		}

		// Create file with chapters
		startMs := int64(0)
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
			Chapters: []*models.Chapter{
				{
					Title:            "Chapter One",
					SortOrder:        0,
					StartTimestampMs: &startMs,
				},
			},
		}

		// First call - generates the file
		cachedPath1, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Get the file's mod time to verify it wasn't regenerated
		info1, err := os.Stat(cachedPath1)
		require.NoError(t, err)
		modTime1 := info1.ModTime()

		// Second call without changing chapters
		cachedPath2, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Verify same cached path returned
		assert.Equal(t, cachedPath1, cachedPath2, "Should return same cached path")

		// File should not have been modified (cache hit)
		info2, err := os.Stat(cachedPath2)
		require.NoError(t, err)
		assert.Equal(t, modTime1, info2.ModTime(), "File should not have been regenerated")

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()
	t.Run("removes cached file and metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, "Test Title", []string{"Author"})

		cache := NewCache(cacheDir, 1024*1024*1024)

		book := &models.Book{Title: "Test"}
		file := &models.File{
			ID:       1,
			FileType: models.FileTypeEPUB,
			Filepath: srcPath,
		}

		// Generate a cached file
		cachedPath, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.NoError(t, err)

		// Verify it exists
		_, err = os.Stat(cachedPath)
		require.NoError(t, err)

		// Invalidate
		err = cache.Invalidate(file.ID, file.FileType)
		require.NoError(t, err)

		// Verify file is gone
		_, err = os.Stat(cachedPath)
		assert.True(t, os.IsNotExist(err))

		// Verify metadata is gone
		meta, err := ReadMetadata(cacheDir, file.ID)
		require.NoError(t, err)
		assert.Nil(t, meta)

		// Wait for background cleanup goroutine to finish
		time.Sleep(50 * time.Millisecond)
	})
}

// createTestEPUB creates a minimal valid EPUB file for testing.
func createTestEPUB(t *testing.T, path, title string, authors []string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Add mimetype (must be first and uncompressed)
	mimetypeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mimetypeWriter, err := w.CreateHeader(mimetypeHeader)
	require.NoError(t, err)
	_, err = mimetypeWriter.Write([]byte("application/epub+zip"))
	require.NoError(t, err)

	// Add container.xml
	containerWriter, err := w.Create("META-INF/container.xml")
	require.NoError(t, err)
	_, err = containerWriter.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))
	require.NoError(t, err)

	// Build OPF content
	var authorsXML strings.Builder
	for _, author := range authors {
		authorsXML.WriteString(`    <dc:creator opf:role="aut">`)
		authorsXML.WriteString(author)
		authorsXML.WriteString("</dc:creator>\n")
	}

	opfContent := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>` + title + `</dc:title>
` + authorsXML.String() + `    <dc:identifier id="bookid">test-book-id</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`

	opfWriter, err := w.Create("content.opf")
	require.NoError(t, err)
	_, err = opfWriter.Write([]byte(opfContent))
	require.NoError(t, err)

	// Add a sample chapter
	chapterWriter, err := w.Create("chapter1.xhtml")
	require.NoError(t, err)
	_, err = chapterWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Chapter 1 content</p></body>
</html>`))
	require.NoError(t, err)

	require.NoError(t, w.Close())
}
