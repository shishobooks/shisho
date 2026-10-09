package filegen

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGenerator_MOBIAndAZW3(t *testing.T) {
	t.Parallel()
	for _, fileType := range []string{models.FileTypeMOBI, models.FileTypeAZW3} {
		gen, err := GetGenerator(fileType)
		require.NoError(t, err, fileType)
		assert.Equal(t, fileType, gen.SupportedType())
	}
}

func TestMOBIGenerator_Generate(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		kind     testgen.MOBIKind
		fileType string
		filename string
	}{
		"MOBI6": {testgen.MOBIKindMOBI6, models.FileTypeMOBI, "book.mobi"},
		"AZW3":  {testgen.MOBIKindKF8, models.FileTypeAZW3, "book.azw3"},
		"combo": {testgen.MOBIKindCombo, models.FileTypeMOBI, "book.mobi"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			srcPath := testgen.GenerateMOBI(t, dir, tc.filename, testgen.MOBIOptions{
				Kind:     tc.kind,
				Title:    "Original Title",
				Authors:  []string{"Original Author"},
				HasCover: true,
			})
			original, err := os.ReadFile(srcPath)
			require.NoError(t, err)

			cover := pngCover(t)
			coverName := tc.filename + ".cover.png"
			require.NoError(t, os.WriteFile(filepath.Join(dir, coverName), cover, 0o600))

			releaseDate := time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC)
			book := &models.Book{
				Title:       "New Title",
				Description: pointerutil.String("New description."),
				Authors: []*models.Author{
					{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
					{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SeriesNumber: pointerutil.Float64(2), Series: &models.Series{Name: "The Saga"}},
				},
				BookGenres: []*models.BookGenre{
					{Genre: &models.Genre{Name: "Fantasy"}},
					{Genre: &models.Genre{Name: "Adventure"}},
				},
				BookTags: []*models.BookTag{{Tag: &models.Tag{Name: "favorite"}}},
			}
			file := &models.File{
				FileType:           tc.fileType,
				Filepath:           srcPath,
				Publisher:          &models.Publisher{Name: "New House"},
				ReleaseDate:        &releaseDate,
				Language:           pointerutil.String("de"),
				CoverImageFilename: pointerutil.String(coverName),
				CoverMimeType:      pointerutil.String("image/png"),
				Identifiers: []*models.FileIdentifier{
					{Type: "isbn_13", Value: "9780141036144"},
					{Type: "asin", Value: "B00ABCDEFG"},
					{Type: "goodreads", Value: "12345"},
				},
			}

			destPath := filepath.Join(t.TempDir(), "out."+tc.fileType)
			gen, err := GetGenerator(tc.fileType)
			require.NoError(t, err)
			require.NoError(t, gen.Generate(context.Background(), srcPath, destPath, book, file))

			meta, err := mobi.Parse(destPath)
			require.NoError(t, err)
			assert.Equal(t, "New Title", meta.Title, "series is never folded into the title")
			assert.Equal(t, []mediafile.ParsedAuthor{{Name: "First Author"}, {Name: "Second Author"}}, meta.Authors)
			assert.Equal(t, "New description.", meta.Description)
			assert.Equal(t, "New House", meta.Publisher)
			require.NotNil(t, meta.ReleaseDate)
			assert.Equal(t, releaseDate, meta.ReleaseDate.UTC())
			require.NotNil(t, meta.Language)
			assert.Equal(t, "de", *meta.Language)
			assert.Equal(t, []mediafile.ParsedIdentifier{
				{Type: "isbn_13", Value: "9780141036144"},
				{Type: "asin", Value: "B00ABCDEFG"},
			}, meta.Identifiers)
			assert.Equal(t, []string{"Fantasy", "Adventure"}, meta.Genres)
			assert.Equal(t, "image/jpeg", meta.CoverMimeType, "a PNG cover is written as JPEG")
			assert.NotEmpty(t, meta.CoverData)
			assert.Empty(t, meta.Series)
			assert.Empty(t, meta.Tags)

			after, err := os.ReadFile(srcPath)
			require.NoError(t, err)
			assert.Equal(t, original, after, "the source file is never modified")
		})
	}
}

func TestMOBIGenerator_PrefersFileName(t *testing.T) {
	t.Parallel()

	srcPath := testgen.GenerateMOBI(t, t.TempDir(), "book.azw3", testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "Original"})
	destPath := filepath.Join(t.TempDir(), "out.azw3")
	book := &models.Book{Title: "Book Title"}
	file := &models.File{FileType: models.FileTypeAZW3, Filepath: srcPath, Name: pointerutil.String("Edition Name")}

	require.NoError(t, (&MOBIGenerator{fileType: models.FileTypeAZW3}).Generate(context.Background(), srcPath, destPath, book, file))
	meta, err := mobi.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Edition Name", meta.Title)
}

func TestMOBIGenerator_DRMProtectedFails(t *testing.T) {
	t.Parallel()

	srcPath := testgen.GenerateMOBI(t, t.TempDir(), "book.azw3", testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "x", Encrypted: true})
	destPath := filepath.Join(t.TempDir(), "out.azw3")
	err := (&MOBIGenerator{fileType: models.FileTypeAZW3}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "y"}, &models.File{FileType: models.FileTypeAZW3})

	var genErr *GenerationError
	require.ErrorAs(t, err, &genErr)
	_, statErr := os.Stat(destPath)
	assert.True(t, os.IsNotExist(statErr), "no partial file is left behind")
}

func pngCover(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 30, 45))
	for y := range 45 {
		for x := range 30 {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 5), 120, 255}) //nolint:gosec // small test values
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestMOBIGenerator_CoverReadFailures(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		setup   func(t *testing.T, coverPath string)
		wantErr bool
	}{
		"missing cover keeps the file's cover": {setup: func(*testing.T, string) {}},
		"unreadable cover is a fault": {
			setup:   func(t *testing.T, coverPath string) { require.NoError(t, os.Mkdir(coverPath, 0o755)) },
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			srcPath := testgen.GenerateMOBI(t, dir, "book.azw3", testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "x", HasCover: true})
			tc.setup(t, filepath.Join(dir, "book.azw3.cover.jpg"))
			file := &models.File{FileType: models.FileTypeAZW3, Filepath: srcPath, CoverImageFilename: pointerutil.String("book.azw3.cover.jpg")}
			destPath := filepath.Join(t.TempDir(), "out.azw3")

			err := (&MOBIGenerator{fileType: models.FileTypeAZW3}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "y"}, file)
			if tc.wantErr {
				var genErr *GenerationError
				require.ErrorAs(t, err, &genErr)
				return
			}
			require.NoError(t, err)
			before, err := mobi.Parse(srcPath)
			require.NoError(t, err)
			after, err := mobi.Parse(destPath)
			require.NoError(t, err)
			assert.Equal(t, before.CoverData, after.CoverData)
		})
	}
}
