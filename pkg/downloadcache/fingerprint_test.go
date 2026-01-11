package downloadcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeFingerprint(t *testing.T) {
	t.Run("basic book with all fields", func(t *testing.T) {
		book := &models.Book{
			Title:    "Test Book",
			Subtitle: strPtr("A Subtitle"),
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author One"}},
				{SortOrder: 1, Person: &models.Person{Name: "Author Two"}},
			},
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(1), Series: &models.Series{Name: "Series One"}},
			},
		}
		file := &models.File{
			CoverImagePath: nil,
			CoverMimeType:  nil,
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Equal(t, "Test Book", fp.Title)
		assert.Equal(t, "A Subtitle", *fp.Subtitle)
		assert.Len(t, fp.Authors, 2)
		assert.Equal(t, "Author One", fp.Authors[0].Name)
		assert.Equal(t, 0, fp.Authors[0].SortOrder)
		assert.Equal(t, "Author Two", fp.Authors[1].Name)
		assert.Equal(t, 1, fp.Authors[1].SortOrder)
		assert.Len(t, fp.Series, 1)
		assert.Equal(t, "Series One", fp.Series[0].Name)
		assert.InDelta(t, 1.0, *fp.Series[0].Number, 0.001)
		assert.Nil(t, fp.Cover)
	})

	t.Run("book with no authors or series", func(t *testing.T) {
		book := &models.Book{
			Title:      "Simple Book",
			Subtitle:   nil,
			Authors:    nil,
			BookSeries: nil,
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Equal(t, "Simple Book", fp.Title)
		assert.Nil(t, fp.Subtitle)
		assert.Empty(t, fp.Authors)
		assert.Empty(t, fp.Series)
		assert.Nil(t, fp.Cover)
	})

	t.Run("authors are sorted by sort order", func(t *testing.T) {
		book := &models.Book{
			Title: "Multi Author",
			Authors: []*models.Author{
				{SortOrder: 2, Person: &models.Person{Name: "Third"}},
				{SortOrder: 0, Person: &models.Person{Name: "First"}},
				{SortOrder: 1, Person: &models.Person{Name: "Second"}},
			},
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Len(t, fp.Authors, 3)
		assert.Equal(t, "First", fp.Authors[0].Name)
		assert.Equal(t, "Second", fp.Authors[1].Name)
		assert.Equal(t, "Third", fp.Authors[2].Name)
	})

	t.Run("series are sorted by sort order", func(t *testing.T) {
		book := &models.Book{
			Title: "Multi Series",
			BookSeries: []*models.BookSeries{
				{SortOrder: 1, SeriesNumber: pointerutil.Float64(2), Series: &models.Series{Name: "Second Series"}},
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(1), Series: &models.Series{Name: "First Series"}},
			},
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Len(t, fp.Series, 2)
		assert.Equal(t, "First Series", fp.Series[0].Name)
		assert.Equal(t, "Second Series", fp.Series[1].Name)
	})

	t.Run("narrators are sorted by sort order", func(t *testing.T) {
		book := &models.Book{Title: "Audiobook"}
		file := &models.File{
			Narrators: []*models.Narrator{
				{SortOrder: 2, Person: &models.Person{Name: "Third Narrator"}},
				{SortOrder: 0, Person: &models.Person{Name: "First Narrator"}},
				{SortOrder: 1, Person: &models.Person{Name: "Second Narrator"}},
			},
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Len(t, fp.Narrators, 3)
		assert.Equal(t, "First Narrator", fp.Narrators[0].Name)
		assert.Equal(t, 0, fp.Narrators[0].SortOrder)
		assert.Equal(t, "Second Narrator", fp.Narrators[1].Name)
		assert.Equal(t, 1, fp.Narrators[1].SortOrder)
		assert.Equal(t, "Third Narrator", fp.Narrators[2].Name)
		assert.Equal(t, 2, fp.Narrators[2].SortOrder)
	})

	t.Run("file with no narrators", func(t *testing.T) {
		book := &models.Book{Title: "Book"}
		file := &models.File{Narrators: nil}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Empty(t, fp.Narrators)
	})

	t.Run("cover information is included", func(t *testing.T) {
		// Create a temp file to simulate a cover image
		tmpDir := t.TempDir()
		coverPath := filepath.Join(tmpDir, "cover.jpg")
		err := os.WriteFile(coverPath, []byte("fake cover data"), 0644)
		require.NoError(t, err)

		// Get the mod time
		info, err := os.Stat(coverPath)
		require.NoError(t, err)
		modTime := info.ModTime()

		book := &models.Book{Title: "Book with Cover"}
		file := &models.File{
			CoverImagePath: strPtr(coverPath),
			CoverMimeType:  strPtr("image/jpeg"),
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		require.NotNil(t, fp.Cover)
		assert.Equal(t, coverPath, fp.Cover.Path)
		assert.Equal(t, "image/jpeg", fp.Cover.MimeType)
		assert.Equal(t, modTime.Unix(), fp.Cover.ModTime.Unix())
	})

	t.Run("cover with non-existent path uses zero time", func(t *testing.T) {
		book := &models.Book{Title: "Book"}
		file := &models.File{
			CoverImagePath: strPtr("/nonexistent/cover.jpg"),
			CoverMimeType:  strPtr("image/jpeg"),
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		require.NotNil(t, fp.Cover)
		assert.True(t, fp.Cover.ModTime.IsZero())
	})

	t.Run("genres are sorted alphabetically", func(t *testing.T) {
		book := &models.Book{
			Title: "Book with Genres",
			BookGenres: []*models.BookGenre{
				{Genre: &models.Genre{Name: "Science Fiction"}},
				{Genre: &models.Genre{Name: "Adventure"}},
				{Genre: &models.Genre{Name: "Fantasy"}},
			},
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Len(t, fp.Genres, 3)
		assert.Equal(t, "Adventure", fp.Genres[0])
		assert.Equal(t, "Fantasy", fp.Genres[1])
		assert.Equal(t, "Science Fiction", fp.Genres[2])
	})

	t.Run("tags are sorted alphabetically", func(t *testing.T) {
		book := &models.Book{
			Title: "Book with Tags",
			BookTags: []*models.BookTag{
				{Tag: &models.Tag{Name: "Must Read"}},
				{Tag: &models.Tag{Name: "Favorites"}},
				{Tag: &models.Tag{Name: "To Review"}},
			},
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Len(t, fp.Tags, 3)
		assert.Equal(t, "Favorites", fp.Tags[0])
		assert.Equal(t, "Must Read", fp.Tags[1])
		assert.Equal(t, "To Review", fp.Tags[2])
	})

	t.Run("book with no genres or tags", func(t *testing.T) {
		book := &models.Book{
			Title:      "Simple Book",
			BookGenres: nil,
			BookTags:   nil,
		}
		file := &models.File{}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		assert.Empty(t, fp.Genres)
		assert.Empty(t, fp.Tags)
	})
}

func TestFingerprintHash(t *testing.T) {
	t.Run("same fingerprint produces same hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title: "Test",
			Authors: []FingerprintAuthor{
				{Name: "Author", SortOrder: 0},
			},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Authors: []FingerprintAuthor{
				{Name: "Author", SortOrder: 0},
			},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different title produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{Title: "Title One"}
		fp2 := &Fingerprint{Title: "Title Two"}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different subtitle produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{Title: "Test", Subtitle: strPtr("Subtitle One")}
		fp2 := &Fingerprint{Title: "Test", Subtitle: strPtr("Subtitle Two")}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different authors produce different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:   "Test",
			Authors: []FingerprintAuthor{{Name: "Author A", SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:   "Test",
			Authors: []FingerprintAuthor{{Name: "Author B", SortOrder: 0}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different author sort order produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:   "Test",
			Authors: []FingerprintAuthor{{Name: "Author", SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:   "Test",
			Authors: []FingerprintAuthor{{Name: "Author", SortOrder: 1}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different series produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:  "Test",
			Series: []FingerprintSeries{{Name: "Series A", SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:  "Test",
			Series: []FingerprintSeries{{Name: "Series B", SortOrder: 0}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different series number produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:  "Test",
			Series: []FingerprintSeries{{Name: "Series", Number: pointerutil.Float64(1), SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:  "Test",
			Series: []FingerprintSeries{{Name: "Series", Number: pointerutil.Float64(2), SortOrder: 0}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different narrators produce different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:     "Test",
			Narrators: []FingerprintNarrator{{Name: "Narrator A", SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:     "Test",
			Narrators: []FingerprintNarrator{{Name: "Narrator B", SortOrder: 0}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different narrator sort order produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:     "Test",
			Narrators: []FingerprintNarrator{{Name: "Narrator", SortOrder: 0}},
		}
		fp2 := &Fingerprint{
			Title:     "Test",
			Narrators: []FingerprintNarrator{{Name: "Narrator", SortOrder: 1}},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different cover produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title: "Test",
			Cover: &FingerprintCover{Path: "/path/a.jpg", MimeType: "image/jpeg", ModTime: time.Now()},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Cover: &FingerprintCover{Path: "/path/b.jpg", MimeType: "image/jpeg", ModTime: time.Now()},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different cover mod time produces different hash", func(t *testing.T) {
		now := time.Now()
		fp1 := &Fingerprint{
			Title: "Test",
			Cover: &FingerprintCover{Path: "/path/cover.jpg", MimeType: "image/jpeg", ModTime: now},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Cover: &FingerprintCover{Path: "/path/cover.jpg", MimeType: "image/jpeg", ModTime: now.Add(time.Hour)},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different genres produce different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:  "Test",
			Genres: []string{"Fantasy"},
		}
		fp2 := &Fingerprint{
			Title:  "Test",
			Genres: []string{"Science Fiction"},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different tags produce different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title: "Test",
			Tags:  []string{"Must Read"},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Tags:  []string{"Favorites"},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("adding genre produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:  "Test",
			Genres: []string{},
		}
		fp2 := &Fingerprint{
			Title:  "Test",
			Genres: []string{"Fantasy"},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("adding tag produces different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title: "Test",
			Tags:  []string{},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Tags:  []string{"Must Read"},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("hash is 64 characters (SHA256 hex)", func(t *testing.T) {
		fp := &Fingerprint{Title: "Test"}
		hash, err := fp.Hash()

		require.NoError(t, err)
		assert.Len(t, hash, 64)
	})
}

func TestFingerprintEqual(t *testing.T) {
	t.Run("equal fingerprints", func(t *testing.T) {
		fp1 := &Fingerprint{Title: "Test"}
		fp2 := &Fingerprint{Title: "Test"}

		assert.True(t, fp1.Equal(fp2))
	})

	t.Run("unequal fingerprints", func(t *testing.T) {
		fp1 := &Fingerprint{Title: "Test1"}
		fp2 := &Fingerprint{Title: "Test2"}

		assert.False(t, fp1.Equal(fp2))
	})

	t.Run("nil fingerprints are equal", func(t *testing.T) {
		var fp1 *Fingerprint
		var fp2 *Fingerprint

		assert.True(t, fp1.Equal(fp2))
	})

	t.Run("nil and non-nil are not equal", func(t *testing.T) {
		var fp1 *Fingerprint
		fp2 := &Fingerprint{Title: "Test"}

		assert.False(t, fp1.Equal(fp2))
		assert.False(t, fp2.Equal(fp1))
	})
}

func strPtr(s string) *string {
	return &s
}
