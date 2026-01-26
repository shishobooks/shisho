package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCBZGenerator_Generate(t *testing.T) {
	t.Parallel()
	t.Run("modifies title and series", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:  "Original Title",
			series: "Original Series",
			number: "1",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "New Title",
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(5), Series: &models.Series{Name: "New Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify the destination file exists
		_, err = os.Stat(destPath)
		require.NoError(t, err)

		// Verify the modified metadata
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "New Title", comicInfo.Title)
		assert.Equal(t, "New Series", comicInfo.Series)
		assert.Equal(t, "5", comicInfo.Number)
	})

	t.Run("generates correct author roles", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:  "Test Comic",
			writer: "Original Writer",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
			Authors: []*models.Author{
				{SortOrder: 0, Role: pointerutil.String(models.AuthorRoleWriter), Person: &models.Person{Name: "New Writer"}},
				{SortOrder: 1, Role: pointerutil.String(models.AuthorRolePenciller), Person: &models.Person{Name: "Pencil Artist"}},
				{SortOrder: 2, Role: pointerutil.String(models.AuthorRoleColorist), Person: &models.Person{Name: "Color Artist"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "New Writer", comicInfo.Writer)
		assert.Equal(t, "Pencil Artist", comicInfo.Penciller)
		assert.Equal(t, "Color Artist", comicInfo.Colorist)
	})

	t.Run("handles multiple authors per role", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Multi-Author Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Multi-Author Comic",
			Authors: []*models.Author{
				{SortOrder: 0, Role: pointerutil.String(models.AuthorRoleWriter), Person: &models.Person{Name: "Writer One"}},
				{SortOrder: 1, Role: pointerutil.String(models.AuthorRoleWriter), Person: &models.Person{Name: "Writer Two"}},
				{SortOrder: 2, Role: pointerutil.String(models.AuthorRolePenciller), Person: &models.Person{Name: "Artist One"}},
				{SortOrder: 3, Role: pointerutil.String(models.AuthorRolePenciller), Person: &models.Person{Name: "Artist Two"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Writer One, Writer Two", comicInfo.Writer)
		assert.Equal(t, "Artist One, Artist Two", comicInfo.Penciller)
	})

	t.Run("sets FrontCover type on correct page from File.CoverPage", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Comic With Cover",
			pageCount: 5,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		coverPage := 2 // Third page is the cover
		book := &models.Book{
			Title: "Comic With Cover",
		}
		file := &models.File{
			FileType:  models.FileTypeCBZ,
			CoverPage: &coverPage,
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		require.NotNil(t, comicInfo.Pages)
		require.NotEmpty(t, comicInfo.Pages.Page)

		// Find the page with FrontCover type
		var frontCoverPage *cbzPageInfo
		for i := range comicInfo.Pages.Page {
			if strings.ToLower(comicInfo.Pages.Page[i].Type) == "frontcover" {
				frontCoverPage = &comicInfo.Pages.Page[i]
				break
			}
		}

		require.NotNil(t, frontCoverPage, "should have a FrontCover page")
		assert.Equal(t, "2", frontCoverPage.Image, "FrontCover should be on page 2")
	})

	t.Run("preserves original ComicInfo.xml fields", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Original Title",
			publisher: "Original Publisher",
			genre:     "Action",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "New Title", // Only title changes
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		// Title should be updated
		assert.Equal(t, "New Title", comicInfo.Title)
		// Publisher and Genre should be preserved
		assert.Equal(t, "Original Publisher", comicInfo.Publisher)
		assert.Equal(t, "Action", comicInfo.Genre)
	})

	t.Run("includes all page images in output", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Test Comic",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Modified Title",
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify all page images exist in output
		// Note: Test images are invalid PNG files (just headers) so they pass through unchanged
		// Valid images would be processed (resized/converted) by ProcessImageForEreader
		for i := 0; i < 3; i++ {
			var imageName string
			switch i {
			case 0:
				imageName = "000.png"
			case 1:
				imageName = "001.png"
			case 2:
				imageName = "002.png"
			}
			data := readFileFromCBZ(t, destPath, imageName)
			assert.NotEmpty(t, data, "page %d image should exist", i)
		}
	})

	t.Run("resizes large images to fit e-reader screen", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		// Create image larger than Kobo screen (1264x1680)
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
			pages: []testPage{
				{filename: "page1.jpg", width: 2000, height: 3000, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test Comic"}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Read the resized image
		imgData := readFileFromCBZ(t, destPath, "page1.jpg")

		// Decode and check dimensions
		img, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		require.NoError(t, err)

		// Should fit within Kobo screen dimensions (1264x1680)
		// Height ratio: 1680/3000 = 0.56, Width ratio: 1264/2000 = 0.632
		// Use smaller ratio (0.56) to fit: 2000*0.56=1120, 3000*0.56=1680
		assert.Equal(t, 1680, img.Height, "image height should fit Kobo screen")
		assert.Equal(t, 1120, img.Width, "image width should be proportionally scaled")
	})

	t.Run("converts PNG to JPEG for smaller file size", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		// Create a PNG image
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
			pages: []testPage{
				{filename: "page1.png", width: 800, height: 1200, format: "png"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test Comic"}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// The PNG should be converted to JPEG
		// Original filename was page1.png, should now be page1.jpg
		jpgData := readFileFromCBZ(t, destPath, "page1.jpg")
		assert.NotEmpty(t, jpgData, "converted JPEG should exist")

		// Verify it's a valid JPEG by decoding
		_, format, err := image.DecodeConfig(bytes.NewReader(jpgData))
		require.NoError(t, err)
		assert.Equal(t, "jpeg", format, "image should be JPEG format")
	})

	t.Run("preserves small JPEG images unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		// Create image smaller than Kobo screen (1264x1680)
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
			pages: []testPage{
				{filename: "page1.jpg", width: 800, height: 1200, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test Comic"}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Read the image
		imgData := readFileFromCBZ(t, destPath, "page1.jpg")

		// Decode and check dimensions
		img, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		require.NoError(t, err)

		// Dimensions should be unchanged since image fits within Kobo screen
		assert.Equal(t, 800, img.Width, "image width should be unchanged")
		assert.Equal(t, 1200, img.Height, "image height should be unchanged")
	})

	t.Run("handles CBZ without existing ComicInfo.xml", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pageCount:     3,
			skipComicInfo: true, // No ComicInfo.xml
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "New Title",
			Authors: []*models.Author{
				{SortOrder: 0, Role: pointerutil.String(models.AuthorRoleWriter), Person: &models.Person{Name: "New Writer"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// ComicInfo.xml should be created
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "New Title", comicInfo.Title)
		assert.Equal(t, "New Writer", comicInfo.Writer)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Test Comic",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeCBZ}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		gen := &CBZGenerator{}
		err := gen.Generate(ctx, srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Contains(t, genErr.Message, "cancelled")
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "nonexistent.cbz")
		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Equal(t, models.FileTypeCBZ, genErr.FileType)
	})

	t.Run("returns error for invalid CBZ", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "invalid.cbz")
		err := os.WriteFile(srcPath, []byte("not a zip file"), 0644)
		require.NoError(t, err)

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Contains(t, genErr.Message, "zip")
	})

	t.Run("handles decimal series number", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(1.5), Series: &models.Series{Name: "Test Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "1.5", comicInfo.Number)
	})

	t.Run("authors without role go to Writer field", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Generic Author Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Generic Author Comic",
			Authors: []*models.Author{
				{SortOrder: 0, Role: nil, Person: &models.Person{Name: "Generic Author"}}, // No role
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Generic Author", comicInfo.Writer)
	})

	t.Run("all author roles are mapped correctly", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Full Crew Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Full Crew Comic",
			Authors: []*models.Author{
				{SortOrder: 0, Role: pointerutil.String(models.AuthorRoleWriter), Person: &models.Person{Name: "The Writer"}},
				{SortOrder: 1, Role: pointerutil.String(models.AuthorRolePenciller), Person: &models.Person{Name: "The Penciller"}},
				{SortOrder: 2, Role: pointerutil.String(models.AuthorRoleInker), Person: &models.Person{Name: "The Inker"}},
				{SortOrder: 3, Role: pointerutil.String(models.AuthorRoleColorist), Person: &models.Person{Name: "The Colorist"}},
				{SortOrder: 4, Role: pointerutil.String(models.AuthorRoleLetterer), Person: &models.Person{Name: "The Letterer"}},
				{SortOrder: 5, Role: pointerutil.String(models.AuthorRoleCoverArtist), Person: &models.Person{Name: "The Cover Artist"}},
				{SortOrder: 6, Role: pointerutil.String(models.AuthorRoleEditor), Person: &models.Person{Name: "The Editor"}},
				{SortOrder: 7, Role: pointerutil.String(models.AuthorRoleTranslator), Person: &models.Person{Name: "The Translator"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "The Writer", comicInfo.Writer)
		assert.Equal(t, "The Penciller", comicInfo.Penciller)
		assert.Equal(t, "The Inker", comicInfo.Inker)
		assert.Equal(t, "The Colorist", comicInfo.Colorist)
		assert.Equal(t, "The Letterer", comicInfo.Letterer)
		assert.Equal(t, "The Cover Artist", comicInfo.CoverArtist)
		assert.Equal(t, "The Editor", comicInfo.Editor)
		assert.Equal(t, "The Translator", comicInfo.Translator)
	})

	t.Run("clears series when book has no series", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:  "Original Title",
			series: "Original Series",
			number: "5",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title:      "New Title",
			BookSeries: []*models.BookSeries{}, // No series
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "New Title", comicInfo.Title)
		assert.Empty(t, comicInfo.Series)
		assert.Empty(t, comicInfo.Number)
	})

	t.Run("writes genres to Genre field", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Test Comic",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
			BookGenres: []*models.BookGenre{
				{Genre: &models.Genre{Name: "Action"}},
				{Genre: &models.Genre{Name: "Adventure"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Action, Adventure", comicInfo.Genre)
	})

	t.Run("writes tags to Tags field", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Test Comic",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
			BookTags: []*models.BookTag{
				{Tag: &models.Tag{Name: "Must Read"}},
				{Tag: &models.Tag{Name: "Favorites"}},
			},
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Must Read, Favorites", comicInfo.Tags)
	})

	t.Run("preserves source genres when book has none", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title:     "Test Comic",
			genre:     "Original Genre",
			pageCount: 3,
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Modified Title",
			// No genres
		}
		file := &models.File{FileType: models.FileTypeCBZ}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Original Genre", comicInfo.Genre)
	})

	t.Run("writes GTIN from file identifiers with priority", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		// Create book and file with identifiers
		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			Identifiers: []*models.FileIdentifier{
				{Type: "isbn_13", Value: "9780316769488"},
				{Type: "isbn_10", Value: "0316769487"},
				{Type: "asin", Value: "B08N5WRWNW"},
			},
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse and verify - should use ISBN-13 (highest priority)
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "9780316769488", comicInfo.GTIN)
	})

	t.Run("writes GTIN with isbn_10 when no isbn_13", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			Identifiers: []*models.FileIdentifier{
				{Type: "isbn_10", Value: "0316769487"},
				{Type: "asin", Value: "B08N5WRWNW"},
			},
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "0316769487", comicInfo.GTIN)
	})

	t.Run("writes GTIN with other type when no isbn", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			Identifiers: []*models.FileIdentifier{
				{Type: "other", Value: "1234567890123"},
				{Type: "asin", Value: "B08N5WRWNW"},
			},
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "1234567890123", comicInfo.GTIN)
	})

	t.Run("writes GTIN with asin when only asin available", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			Identifiers: []*models.FileIdentifier{
				{Type: "asin", Value: "B08N5WRWNW"},
			},
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "B08N5WRWNW", comicInfo.GTIN)
	})

	t.Run("does not write GTIN when no identifiers", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Test Comic",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			Identifiers: []*models.FileIdentifier{},
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Empty(t, comicInfo.GTIN)
	})

	t.Run("uses file.Name for title when available", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Original Title",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		name := "Custom Comic Title"
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     &name,
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse and verify title is set to file.Name, not book.Title
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Custom Comic Title", comicInfo.Title)
	})

	t.Run("uses book.Title when file.Name is empty", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Original Title",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		emptyName := ""
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     &emptyName,
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// When file.Name is empty, should fall back to book.Title
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Book Title", comicInfo.Title)
	})

	t.Run("uses book.Title when file.Name is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			title: "Original Title",
		})

		destPath := filepath.Join(tmpDir, "dest.cbz")

		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     nil, // Nil name
		}

		gen := &CBZGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// When file.Name is nil, should fall back to book.Title
		comicInfo := readComicInfoFromCBZ(t, destPath)
		assert.Equal(t, "Book Title", comicInfo.Title)
	})
}

// Helper types and functions for testing

// testPage defines a page image for CBZ test creation.
type testPage struct {
	filename string
	width    int
	height   int
	format   string // "jpeg" or "png"
}

type testCBZOptions struct {
	title         string
	series        string
	number        string
	writer        string
	penciller     string
	inker         string
	colorist      string
	letterer      string
	coverArtist   string
	editor        string
	translator    string
	publisher     string
	genre         string
	tags          string
	pageCount     int
	skipComicInfo bool
	pages         []testPage // If specified, creates real images instead of dummy headers
}

func createTestCBZ(t *testing.T, path string, opts testCBZOptions) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Default page count
	pageCount := opts.pageCount
	if pageCount <= 0 {
		pageCount = 3
	}

	// Add ComicInfo.xml unless skipped
	if !opts.skipComicInfo {
		comicInfoWriter, err := w.Create("ComicInfo.xml")
		require.NoError(t, err)

		var comicInfoXML strings.Builder
		comicInfoXML.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ComicInfo>
`)
		if opts.title != "" {
			comicInfoXML.WriteString("  <Title>" + opts.title + "</Title>\n")
		}
		if opts.series != "" {
			comicInfoXML.WriteString("  <Series>" + opts.series + "</Series>\n")
		}
		if opts.number != "" {
			comicInfoXML.WriteString("  <Number>" + opts.number + "</Number>\n")
		}
		if opts.writer != "" {
			comicInfoXML.WriteString("  <Writer>" + opts.writer + "</Writer>\n")
		}
		if opts.penciller != "" {
			comicInfoXML.WriteString("  <Penciller>" + opts.penciller + "</Penciller>\n")
		}
		if opts.inker != "" {
			comicInfoXML.WriteString("  <Inker>" + opts.inker + "</Inker>\n")
		}
		if opts.colorist != "" {
			comicInfoXML.WriteString("  <Colorist>" + opts.colorist + "</Colorist>\n")
		}
		if opts.letterer != "" {
			comicInfoXML.WriteString("  <Letterer>" + opts.letterer + "</Letterer>\n")
		}
		if opts.coverArtist != "" {
			comicInfoXML.WriteString("  <CoverArtist>" + opts.coverArtist + "</CoverArtist>\n")
		}
		if opts.editor != "" {
			comicInfoXML.WriteString("  <Editor>" + opts.editor + "</Editor>\n")
		}
		if opts.translator != "" {
			comicInfoXML.WriteString("  <Translator>" + opts.translator + "</Translator>\n")
		}
		if opts.publisher != "" {
			comicInfoXML.WriteString("  <Publisher>" + opts.publisher + "</Publisher>\n")
		}
		if opts.genre != "" {
			comicInfoXML.WriteString("  <Genre>" + opts.genre + "</Genre>\n")
		}
		if opts.tags != "" {
			comicInfoXML.WriteString("  <Tags>" + opts.tags + "</Tags>\n")
		}
		comicInfoXML.WriteString("</ComicInfo>")

		_, err = comicInfoWriter.Write([]byte(comicInfoXML.String()))
		require.NoError(t, err)
	}

	// Add page images
	if len(opts.pages) > 0 {
		// Use specified pages with real images
		for _, page := range opts.pages {
			imgData := createTestImage(page.width, page.height, page.format)
			pageWriter, err := w.Create(page.filename)
			require.NoError(t, err)
			_, err = pageWriter.Write(imgData)
			require.NoError(t, err)
		}
	} else {
		// Add dummy page images (PNG headers) - legacy behavior
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		for i := 0; i < pageCount; i++ {
			var pageName string
			switch {
			case i == 0:
				pageName = "000.png"
			case i == 1:
				pageName = "001.png"
			case i == 2:
				pageName = "002.png"
			case i < 10:
				pageName = "00" + string(rune('0'+i)) + ".png"
			case i < 100:
				pageName = "0" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + ".png"
			default:
				pageName = string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10)) + ".png"
			}
			pageWriter, err := w.Create(pageName)
			require.NoError(t, err)
			_, err = pageWriter.Write(pngHeader)
			require.NoError(t, err)
		}
	}

	require.NoError(t, w.Close())
}

// createTestImage creates a test image with the specified dimensions.
func createTestImage(width, height int, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a solid color
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		_ = png.Encode(&buf, img)
	default: // jpeg
		_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	return buf.Bytes()
}

func readComicInfoFromCBZ(t *testing.T, path string) *cbzComicInfo {
	t.Helper()

	data := readFileFromCBZ(t, path, "ComicInfo.xml")

	var comicInfo cbzComicInfo
	err := xml.Unmarshal(data, &comicInfo)
	require.NoError(t, err)

	return &comicInfo
}

func readFileFromCBZ(t *testing.T, cbzPath, fileName string) []byte {
	t.Helper()

	r, err := zip.OpenReader(cbzPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if strings.EqualFold(f.Name, fileName) {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("file %s not found in CBZ", fileName)
	return nil
}
