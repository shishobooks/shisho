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
	t.Parallel()
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
			CoverImageFilename: nil,
			CoverMimeType:      nil,
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

	t.Run("cover information is resolved against book directory", func(t *testing.T) {
		// CoverImageFilename stores just the filename; the full path is
		// resolved at runtime against book.Filepath.
		tmpDir := t.TempDir()
		coverFilename := "book.epub.cover.jpg"
		coverAbsPath := filepath.Join(tmpDir, coverFilename)
		err := os.WriteFile(coverAbsPath, []byte("fake cover data"), 0644)
		require.NoError(t, err)

		info, err := os.Stat(coverAbsPath)
		require.NoError(t, err)
		modTime := info.ModTime()

		book := &models.Book{Title: "Book with Cover", Filepath: tmpDir}
		file := &models.File{
			CoverImageFilename: strPtr(coverFilename),
			CoverMimeType:      strPtr("image/jpeg"),
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		require.NotNil(t, fp.Cover)
		assert.Equal(t, coverAbsPath, fp.Cover.Path)
		assert.Equal(t, "image/jpeg", fp.Cover.MimeType)
		assert.Equal(t, modTime.Unix(), fp.Cover.ModTime.Unix())
	})

	t.Run("cover with non-existent filename uses zero time", func(t *testing.T) {
		book := &models.Book{Title: "Book", Filepath: "/nonexistent"}
		file := &models.File{
			CoverImageFilename: strPtr("missing.jpg"),
			CoverMimeType:      strPtr("image/jpeg"),
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

	t.Run("M4B chapters sorted by StartTimestampMs for consistent fingerprinting", func(t *testing.T) {
		book := &models.Book{Title: "Book with Chapters"}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "Third Chapter", SortOrder: 2, StartTimestampMs: int64Ptr(120000)},
				{Title: "First Chapter", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
				{Title: "Second Chapter", SortOrder: 1, StartTimestampMs: int64Ptr(60000)},
			},
		}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		// Chapters should be sorted by StartTimestampMs, matching the order
		// the file generator will emit. SortOrder is re-derived from the
		// sorted position so the fingerprint is stable against DB drift.
		require.Len(t, fp.Chapters, 3)
		assert.Equal(t, "First Chapter", fp.Chapters[0].Title)
		assert.Equal(t, 0, fp.Chapters[0].SortOrder)
		assert.Equal(t, "Second Chapter", fp.Chapters[1].Title)
		assert.Equal(t, 1, fp.Chapters[1].SortOrder)
		assert.Equal(t, "Third Chapter", fp.Chapters[2].Title)
		assert.Equal(t, 2, fp.Chapters[2].SortOrder)
	})

	t.Run("file with no chapters has empty chapters slice", func(t *testing.T) {
		book := &models.Book{Title: "Book"}
		file := &models.File{Chapters: nil}

		fp, err := ComputeFingerprint(book, file)
		require.NoError(t, err)

		// Chapters should be an empty slice, not nil, for consistent JSON serialization
		assert.NotNil(t, fp.Chapters)
		assert.Empty(t, fp.Chapters)
	})
}

func TestFingerprintHash(t *testing.T) {
	t.Parallel()
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

	t.Run("different chapters produce different hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title: "Test",
			Chapters: []FingerprintChapter{
				{Title: "Chapter 1", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
			},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Chapters: []FingerprintChapter{
				{Title: "Chapter A", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
			},
		}

		hash1, err1 := fp1.Hash()
		hash2, err2 := fp2.Hash()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("chapter order affects hash", func(t *testing.T) {
		// Same chapters but in different order should produce different hashes
		fp1 := &Fingerprint{
			Title: "Test",
			Chapters: []FingerprintChapter{
				{Title: "Chapter 1", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
				{Title: "Chapter 2", SortOrder: 1, StartTimestampMs: int64Ptr(60000)},
			},
		}
		fp2 := &Fingerprint{
			Title: "Test",
			Chapters: []FingerprintChapter{
				{Title: "Chapter 2", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
				{Title: "Chapter 1", SortOrder: 1, StartTimestampMs: int64Ptr(60000)},
			},
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
	t.Parallel()
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

func TestComputeFingerprint_IncludesFileName(t *testing.T) {
	t.Parallel()
	name := "Custom Edition Name"
	book := &models.Book{
		Title: "Test Book",
	}
	file := &models.File{
		Name: &name,
	}

	fp, err := ComputeFingerprint(book, file)

	require.NoError(t, err)
	assert.NotNil(t, fp.Name)
	assert.Equal(t, "Custom Edition Name", *fp.Name)
}

func TestComputeFingerprint_DifferentNamesProduceDifferentHashes(t *testing.T) {
	t.Parallel()
	book := &models.Book{Title: "Test Book"}

	name1 := "Edition A"
	file1 := &models.File{Name: &name1}
	fp1, _ := ComputeFingerprint(book, file1)

	name2 := "Edition B"
	file2 := &models.File{Name: &name2}
	fp2, _ := ComputeFingerprint(book, file2)

	hash1, _ := fp1.Hash()
	hash2, _ := fp2.Hash()

	assert.NotEqual(t, hash1, hash2, "Different names should produce different hashes")
}

func TestComputeFingerprint_ChaptersAreIncluded(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Title: "Test Book",
	}
	file := &models.File{
		Chapters: []*models.Chapter{
			{
				Title:            "Chapter 1",
				SortOrder:        0,
				StartTimestampMs: int64Ptr(0),
			},
			{
				Title:            "Chapter 2",
				SortOrder:        1,
				StartTimestampMs: int64Ptr(60000),
			},
			{
				Title:            "Chapter 3",
				SortOrder:        2,
				StartTimestampMs: int64Ptr(120000),
			},
		},
	}

	fp, err := ComputeFingerprint(book, file)
	require.NoError(t, err)

	// Verify chapters are included in fingerprint
	require.Len(t, fp.Chapters, 3)

	// Verify first chapter
	assert.Equal(t, "Chapter 1", fp.Chapters[0].Title)
	assert.Equal(t, 0, fp.Chapters[0].SortOrder)
	require.NotNil(t, fp.Chapters[0].StartTimestampMs)
	assert.Equal(t, int64(0), *fp.Chapters[0].StartTimestampMs)

	// Verify second chapter
	assert.Equal(t, "Chapter 2", fp.Chapters[1].Title)
	assert.Equal(t, 1, fp.Chapters[1].SortOrder)
	require.NotNil(t, fp.Chapters[1].StartTimestampMs)
	assert.Equal(t, int64(60000), *fp.Chapters[1].StartTimestampMs)

	// Verify third chapter
	assert.Equal(t, "Chapter 3", fp.Chapters[2].Title)
	assert.Equal(t, 2, fp.Chapters[2].SortOrder)
	require.NotNil(t, fp.Chapters[2].StartTimestampMs)
	assert.Equal(t, int64(120000), *fp.Chapters[2].StartTimestampMs)
}

func TestComputeFingerprint_ChaptersWithNilOptionalFieldsFingerprintCorrectly(t *testing.T) {
	t.Parallel()
	// This test ensures that chapters with nil optional fields (StartPage, StartTimestampMs, Href)
	// don't cause panics during fingerprinting and compute successfully.
	book := &models.Book{
		Title: "Test Book with Minimal Chapters",
	}
	// FileType: EPUB so chapters sort by SortOrder (EPUB has no natural
	// position field). This pins the input array order in the fingerprint
	// output regardless of which nil optional fields are set.
	file := &models.File{
		FileType: models.FileTypeEPUB,
		Chapters: []*models.Chapter{
			{
				Title:            "Chapter with all nil optional fields",
				SortOrder:        0,
				StartPage:        nil, // CBZ field - nil
				StartTimestampMs: nil, // M4B field - nil
				Href:             nil, // EPUB field - nil
			},
			{
				Title:            "Another minimal chapter",
				SortOrder:        1,
				StartPage:        nil,
				StartTimestampMs: nil,
				Href:             nil,
			},
		},
	}

	// Should not panic and should compute without error
	fp, err := ComputeFingerprint(book, file)
	require.NoError(t, err)

	// Verify chapters are included in fingerprint
	require.Len(t, fp.Chapters, 2)

	// Verify first chapter has nil optional fields preserved (or omitempty behavior)
	assert.Equal(t, "Chapter with all nil optional fields", fp.Chapters[0].Title)
	assert.Equal(t, 0, fp.Chapters[0].SortOrder)
	assert.Nil(t, fp.Chapters[0].StartPage)
	assert.Nil(t, fp.Chapters[0].StartTimestampMs)
	assert.Nil(t, fp.Chapters[0].Href)

	// Verify second chapter
	assert.Equal(t, "Another minimal chapter", fp.Chapters[1].Title)
	assert.Equal(t, 1, fp.Chapters[1].SortOrder)
	assert.Nil(t, fp.Chapters[1].StartPage)
	assert.Nil(t, fp.Chapters[1].StartTimestampMs)
	assert.Nil(t, fp.Chapters[1].Href)

	// Verify hash can be computed without error (proves JSON serialization works with nil fields)
	hash, err := fp.Hash()
	require.NoError(t, err)
	assert.Len(t, hash, 64) // SHA256 hex is 64 characters
}

func TestComputeFingerprint_NestedChaptersAreIncluded(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Title: "Test Book with Nested Chapters",
	}
	// Create a parent chapter with nested children. FileType: M4B so the
	// sort uses StartTimestampMs; the fixture data is already in timestamp
	// order, so sorted output matches the input array.
	file := &models.File{
		FileType: models.FileTypeM4B,
		Chapters: []*models.Chapter{
			{
				Title:            "Part 1",
				SortOrder:        0,
				StartTimestampMs: int64Ptr(0),
				Children: []*models.Chapter{
					{
						Title:            "Chapter 1.1",
						SortOrder:        0,
						StartTimestampMs: int64Ptr(1000),
						Children: []*models.Chapter{
							{
								Title:            "Section 1.1.1",
								SortOrder:        0,
								StartTimestampMs: int64Ptr(1100),
							},
							{
								Title:            "Section 1.1.2",
								SortOrder:        1,
								StartTimestampMs: int64Ptr(1200),
							},
						},
					},
					{
						Title:            "Chapter 1.2",
						SortOrder:        1,
						StartTimestampMs: int64Ptr(2000),
					},
				},
			},
			{
				Title:            "Part 2",
				SortOrder:        1,
				StartTimestampMs: int64Ptr(60000),
				Children: []*models.Chapter{
					{
						Title:            "Chapter 2.1",
						SortOrder:        0,
						StartTimestampMs: int64Ptr(61000),
					},
				},
			},
		},
	}

	fp, err := ComputeFingerprint(book, file)
	require.NoError(t, err)

	// Verify top-level chapters
	require.Len(t, fp.Chapters, 2)

	// Verify Part 1
	assert.Equal(t, "Part 1", fp.Chapters[0].Title)
	assert.Equal(t, 0, fp.Chapters[0].SortOrder)
	require.NotNil(t, fp.Chapters[0].StartTimestampMs)
	assert.Equal(t, int64(0), *fp.Chapters[0].StartTimestampMs)

	// Verify Part 1 has children
	require.Len(t, fp.Chapters[0].Children, 2)

	// Verify Chapter 1.1
	assert.Equal(t, "Chapter 1.1", fp.Chapters[0].Children[0].Title)
	assert.Equal(t, 0, fp.Chapters[0].Children[0].SortOrder)
	require.NotNil(t, fp.Chapters[0].Children[0].StartTimestampMs)
	assert.Equal(t, int64(1000), *fp.Chapters[0].Children[0].StartTimestampMs)

	// Verify Chapter 1.1 has nested children (depth 3)
	require.Len(t, fp.Chapters[0].Children[0].Children, 2)
	assert.Equal(t, "Section 1.1.1", fp.Chapters[0].Children[0].Children[0].Title)
	assert.Equal(t, 0, fp.Chapters[0].Children[0].Children[0].SortOrder)
	assert.Equal(t, "Section 1.1.2", fp.Chapters[0].Children[0].Children[1].Title)
	assert.Equal(t, 1, fp.Chapters[0].Children[0].Children[1].SortOrder)

	// Verify Chapter 1.2
	assert.Equal(t, "Chapter 1.2", fp.Chapters[0].Children[1].Title)
	assert.Equal(t, 1, fp.Chapters[0].Children[1].SortOrder)

	// Verify Part 2
	assert.Equal(t, "Part 2", fp.Chapters[1].Title)
	assert.Equal(t, 1, fp.Chapters[1].SortOrder)

	// Verify Part 2 has children
	require.Len(t, fp.Chapters[1].Children, 1)
	assert.Equal(t, "Chapter 2.1", fp.Chapters[1].Children[0].Title)
	assert.Equal(t, 0, fp.Chapters[1].Children[0].SortOrder)
}

func TestComputeFingerprint_ChapterOrderStableAgainstSortOrderDrift(t *testing.T) {
	t.Parallel()

	// Regression: the fingerprint used to sort chapters by SortOrder. After
	// the file generators started sorting by natural position (StartPage /
	// StartTimestampMs), the fingerprint must also sort by natural position
	// so that two DB states representing the same produced file hash to the
	// same fingerprint.

	book := &models.Book{Title: "Book"}

	t.Run("PDF: sort_order drift does not change the fingerprint", func(t *testing.T) {
		t.Parallel()
		startPage0 := 0
		startPage3 := 3
		startPage7 := 7

		// "Normalized" state: SortOrder matches StartPage order.
		normalized := &models.File{
			FileType: models.FileTypePDF,
			Chapters: []*models.Chapter{
				{Title: "A", SortOrder: 0, StartPage: &startPage0},
				{Title: "B", SortOrder: 1, StartPage: &startPage3},
				{Title: "C", SortOrder: 2, StartPage: &startPage7},
			},
		}

		// "Drifted" state: same chapters, same pages, but SortOrder is
		// reversed. The file generator produces the same output for both.
		drifted := &models.File{
			FileType: models.FileTypePDF,
			Chapters: []*models.Chapter{
				{Title: "C", SortOrder: 0, StartPage: &startPage7},
				{Title: "B", SortOrder: 1, StartPage: &startPage3},
				{Title: "A", SortOrder: 2, StartPage: &startPage0},
			},
		}

		fpNorm, err := ComputeFingerprint(book, normalized)
		require.NoError(t, err)
		fpDrift, err := ComputeFingerprint(book, drifted)
		require.NoError(t, err)

		hNorm, err := fpNorm.Hash()
		require.NoError(t, err)
		hDrift, err := fpDrift.Hash()
		require.NoError(t, err)
		assert.Equal(t, hNorm, hDrift, "PDF fingerprint must be stable against SortOrder drift")

		// Also pin the expected output order explicitly.
		require.Len(t, fpDrift.Chapters, 3)
		assert.Equal(t, "A", fpDrift.Chapters[0].Title)
		assert.Equal(t, 0, fpDrift.Chapters[0].SortOrder)
		assert.Equal(t, "B", fpDrift.Chapters[1].Title)
		assert.Equal(t, 1, fpDrift.Chapters[1].SortOrder)
		assert.Equal(t, "C", fpDrift.Chapters[2].Title)
		assert.Equal(t, 2, fpDrift.Chapters[2].SortOrder)
	})

	t.Run("M4B: sort_order drift does not change the fingerprint", func(t *testing.T) {
		t.Parallel()
		normalized := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "A", SortOrder: 0, StartTimestampMs: int64Ptr(0)},
				{Title: "B", SortOrder: 1, StartTimestampMs: int64Ptr(3000)},
				{Title: "C", SortOrder: 2, StartTimestampMs: int64Ptr(7000)},
			},
		}
		drifted := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "C", SortOrder: 0, StartTimestampMs: int64Ptr(7000)},
				{Title: "A", SortOrder: 1, StartTimestampMs: int64Ptr(0)},
				{Title: "B", SortOrder: 2, StartTimestampMs: int64Ptr(3000)},
			},
		}

		fpNorm, _ := ComputeFingerprint(book, normalized)
		fpDrift, _ := ComputeFingerprint(book, drifted)
		hNorm, _ := fpNorm.Hash()
		hDrift, _ := fpDrift.Hash()
		assert.Equal(t, hNorm, hDrift, "M4B fingerprint must be stable against SortOrder drift")
	})

	t.Run("CBZ: sort_order drift does not change the fingerprint", func(t *testing.T) {
		t.Parallel()
		startPage0 := 0
		startPage3 := 3
		normalized := &models.File{
			FileType: models.FileTypeCBZ,
			Chapters: []*models.Chapter{
				{Title: "A", SortOrder: 0, StartPage: &startPage0},
				{Title: "B", SortOrder: 1, StartPage: &startPage3},
			},
		}
		drifted := &models.File{
			FileType: models.FileTypeCBZ,
			Chapters: []*models.Chapter{
				{Title: "B", SortOrder: 0, StartPage: &startPage3},
				{Title: "A", SortOrder: 1, StartPage: &startPage0},
			},
		}

		fpNorm, _ := ComputeFingerprint(book, normalized)
		fpDrift, _ := ComputeFingerprint(book, drifted)
		hNorm, _ := fpNorm.Hash()
		hDrift, _ := fpDrift.Hash()
		assert.Equal(t, hNorm, hDrift, "CBZ fingerprint must be stable against SortOrder drift")
	})

	t.Run("EPUB: still sorts by SortOrder (no natural position field)", func(t *testing.T) {
		t.Parallel()
		// EPUB chapters use href; there's no natural numeric order, so
		// SortOrder remains the source of truth — different SortOrder
		// values do produce different fingerprints.
		a := "a.xhtml"
		b := "b.xhtml"
		state1 := &models.File{
			FileType: models.FileTypeEPUB,
			Chapters: []*models.Chapter{
				{Title: "A", SortOrder: 0, Href: &a},
				{Title: "B", SortOrder: 1, Href: &b},
			},
		}
		state2 := &models.File{
			FileType: models.FileTypeEPUB,
			Chapters: []*models.Chapter{
				{Title: "B", SortOrder: 0, Href: &b},
				{Title: "A", SortOrder: 1, Href: &a},
			},
		}
		fp1, _ := ComputeFingerprint(book, state1)
		fp2, _ := ComputeFingerprint(book, state2)
		h1, _ := fp1.Hash()
		h2, _ := fp2.Hash()
		assert.NotEqual(t, h1, h2, "EPUB fingerprint must reflect SortOrder changes")
	})
}

func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
