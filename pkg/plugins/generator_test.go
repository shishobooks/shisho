package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOutputGenerator(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "hooks-generator", "hooks-generator"})
	ctx := context.Background()

	// Install the plugin
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "hooks-generator",
		Name:        "Test Generator",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// Load the plugin
	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	// Get the output generator for a known format ID
	gen := mgr.GetOutputGenerator("test-format")
	require.NotNil(t, gen)
	assert.Equal(t, "test-format", gen.SupportedType())

	// Get the output generator for an unknown format ID
	gen = mgr.GetOutputGenerator("unknown-format")
	assert.Nil(t, gen)
}

func TestRegisteredOutputFormats(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "hooks-generator", "hooks-generator"})
	ctx := context.Background()

	// Install the plugin
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "hooks-generator",
		Name:        "Test Generator",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// Load the plugin
	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	// Get registered formats
	formats := mgr.RegisteredOutputFormats()
	require.Len(t, formats, 1)
	assert.Equal(t, "test-format", formats[0].ID)
	assert.Equal(t, "Test Format", formats[0].Name)
	assert.Equal(t, []string{"epub"}, formats[0].SourceTypes)
	assert.Equal(t, "test", formats[0].Scope)
	assert.Equal(t, "hooks-generator", formats[0].PluginID)
}

func TestRegisteredOutputFormats_Empty(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install a plugin that doesn't have outputGenerator
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	// No output generators registered
	formats := mgr.RegisteredOutputFormats()
	assert.Empty(t, formats)
}

func TestPluginGenerator_Generate(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "hooks-generator", "hooks-generator"})
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "hooks-generator",
		Name:        "Test Generator",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	gen := mgr.GetOutputGenerator("test-format")
	require.NotNil(t, gen)

	// Create a source file
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.epub")
	err = os.WriteFile(srcPath, []byte("hello world"), 0644)
	require.NoError(t, err)

	destPath := filepath.Join(tmpDir, "output.test-format")

	book := &models.Book{
		ID:    1,
		Title: "Test Book",
	}
	file := &models.File{
		ID:       1,
		Filepath: srcPath,
		FileType: "epub",
		FileRole: "main",
	}

	// Generate the file
	err = gen.Generate(ctx, srcPath, destPath, book, file)
	require.NoError(t, err)

	// Verify the output
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "generated:hello world", string(data))
}

func TestPluginGenerator_Fingerprint(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "hooks-generator", "hooks-generator"})
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "hooks-generator",
		Name:        "Test Generator",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	gen := mgr.GetOutputGenerator("test-format")
	require.NotNil(t, gen)

	book := &models.Book{
		ID:    1,
		Title: "Test Book",
	}
	file := &models.File{
		ID:       1,
		Filepath: "/some/path.epub",
		FileType: "epub",
		FileRole: "main",
	}

	// Get the fingerprint
	fp, err := gen.Fingerprint(book, file)
	require.NoError(t, err)
	assert.Equal(t, "fp-Test Book-epub", fp)

	// Different book title produces different fingerprint
	book2 := &models.Book{
		ID:    2,
		Title: "Other Book",
	}
	fp2, err := gen.Fingerprint(book2, file)
	require.NoError(t, err)
	assert.Equal(t, "fp-Other Book-epub", fp2)
	assert.NotEqual(t, fp, fp2)
}

func TestPluginGenerator_NotLoaded(t *testing.T) {
	mgr, _ := setupTestManager(t)
	ctx := context.Background()

	// Create a generator for a plugin that's not loaded
	gen := NewPluginGenerator(mgr, "test", "nonexistent", "mobi")

	book := &models.Book{ID: 1, Title: "Test"}
	file := &models.File{ID: 1, FileType: "epub", FileRole: "main"}

	// Generate should fail
	err := gen.Generate(ctx, "/src", "/dest", book, file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not loaded")

	// Fingerprint should fail
	_, err = gen.Fingerprint(book, file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not loaded")
}

func TestBuildBookContext(t *testing.T) {
	t.Run("nil book returns nil", func(t *testing.T) {
		ctx := BuildBookContext(nil)
		assert.Nil(t, ctx)
	})

	t.Run("basic book fields", func(t *testing.T) {
		subtitle := "A Subtitle"
		description := "A Description"
		book := &models.Book{
			ID:          1,
			Title:       "My Book",
			Subtitle:    &subtitle,
			Description: &description,
		}

		ctx := BuildBookContext(book)
		assert.Equal(t, 1, ctx["id"])
		assert.Equal(t, "My Book", ctx["title"])
		assert.Equal(t, "A Subtitle", ctx["subtitle"])
		assert.Equal(t, "A Description", ctx["description"])
	})

	t.Run("book with authors", func(t *testing.T) {
		book := &models.Book{
			ID:    1,
			Title: "My Book",
			Authors: []*models.Author{
				{SortOrder: 1, Person: &models.Person{Name: "Author B"}},
				{SortOrder: 0, Person: &models.Person{Name: "Author A"}},
			},
		}

		ctx := BuildBookContext(book)
		authors := ctx["authors"].([]map[string]interface{})
		require.Len(t, authors, 2)
		// Should be sorted by SortOrder
		assert.Equal(t, "Author A", authors[0]["name"])
		assert.Equal(t, "Author B", authors[1]["name"])
	})

	t.Run("book with series", func(t *testing.T) {
		num := 3.0
		book := &models.Book{
			ID:    1,
			Title: "My Book",
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: &num, Series: &models.Series{Name: "My Series"}},
			},
		}

		ctx := BuildBookContext(book)
		series := ctx["series"].([]map[string]interface{})
		require.Len(t, series, 1)
		assert.Equal(t, "My Series", series[0]["name"])
		assert.InDelta(t, 3.0, series[0]["number"], 0.001)
	})

	t.Run("book with genres and tags", func(t *testing.T) {
		book := &models.Book{
			ID:    1,
			Title: "My Book",
			BookGenres: []*models.BookGenre{
				{Genre: &models.Genre{Name: "Fantasy"}},
				{Genre: &models.Genre{Name: "Adventure"}},
			},
			BookTags: []*models.BookTag{
				{Tag: &models.Tag{Name: "magic"}},
			},
		}

		ctx := BuildBookContext(book)
		genres := ctx["genres"].([]string)
		assert.Contains(t, genres, "Fantasy")
		assert.Contains(t, genres, "Adventure")
		tags := ctx["tags"].([]string)
		assert.Equal(t, []string{"magic"}, tags)
	})
}

func TestBuildFileContext(t *testing.T) {
	t.Run("nil file returns nil", func(t *testing.T) {
		ctx := BuildFileContext(nil)
		assert.Nil(t, ctx)
	})

	t.Run("basic file fields", func(t *testing.T) {
		name := "My Edition"
		url := "https://example.com"
		file := &models.File{
			ID:            1,
			Filepath:      "/path/to/file.epub",
			FileType:      "epub",
			FileRole:      "main",
			FilesizeBytes: 1024,
			Name:          &name,
			URL:           &url,
			Publisher:     &models.Publisher{Name: "My Publisher"},
			Imprint:       &models.Imprint{Name: "My Imprint"},
		}

		ctx := BuildFileContext(file)
		assert.Equal(t, 1, ctx["id"])
		assert.Equal(t, "/path/to/file.epub", ctx["filepath"])
		assert.Equal(t, "epub", ctx["fileType"])
		assert.Equal(t, "main", ctx["fileRole"])
		assert.Equal(t, int64(1024), ctx["filesizeBytes"])
		assert.Equal(t, "My Edition", ctx["name"])
		assert.Equal(t, "https://example.com", ctx["url"])
		assert.Equal(t, "My Publisher", ctx["publisher"])
		assert.Equal(t, "My Imprint", ctx["imprint"])
	})

	t.Run("file with narrators", func(t *testing.T) {
		file := &models.File{
			ID:       1,
			FileType: "m4b",
			FileRole: "main",
			Narrators: []*models.Narrator{
				{SortOrder: 1, Person: &models.Person{Name: "Narrator B"}},
				{SortOrder: 0, Person: &models.Person{Name: "Narrator A"}},
			},
		}

		ctx := BuildFileContext(file)
		narrators := ctx["narrators"].([]string)
		require.Len(t, narrators, 2)
		// Should be sorted by SortOrder
		assert.Equal(t, "Narrator A", narrators[0])
		assert.Equal(t, "Narrator B", narrators[1])
	})

	t.Run("file with identifiers", func(t *testing.T) {
		file := &models.File{
			ID:       1,
			FileType: "epub",
			FileRole: "main",
			Identifiers: []*models.FileIdentifier{
				{Type: "isbn", Value: "1234567890"},
				{Type: "asin", Value: "B00000001"},
			},
		}

		ctx := BuildFileContext(file)
		identifiers := ctx["identifiers"].([]map[string]interface{})
		require.Len(t, identifiers, 2)
		assert.Equal(t, "isbn", identifiers[0]["type"])
		assert.Equal(t, "1234567890", identifiers[0]["value"])
	})

	t.Run("file with release date", func(t *testing.T) {
		releaseDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
		file := &models.File{
			ID:          1,
			FileType:    "epub",
			FileRole:    "main",
			ReleaseDate: &releaseDate,
		}

		ctx := BuildFileContext(file)
		assert.Equal(t, "2024-03-15", ctx["releaseDate"])
	})
}
