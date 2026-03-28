package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupHooksTestManager creates a Manager with a single plugin loaded from testdata.
func setupHooksTestManager(t *testing.T, testdata, pluginID string) (*Manager, *Runtime) {
	t.Helper()

	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Copy testdata files to plugin directory
	destDir := filepath.Join(pluginDir, "test", pluginID)
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	srcDir := filepath.Join("testdata", testdata)
	copyTestdataFile(t, srcDir, destDir, "manifest.json")
	copyTestdataFile(t, srcDir, destDir, "main.js")

	// Create the manager and install/load the plugin
	manager := NewManager(service, pluginDir, "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          pluginID,
		Name:        pluginID + " Plugin",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = manager.LoadPlugin(ctx, "test", pluginID)
	require.NoError(t, err)

	rt := manager.GetRuntime("test", pluginID)
	require.NotNil(t, rt)

	return manager, rt
}

func TestRunInputConverter_Success(t *testing.T) {
	mgr, rt := setupHooksTestManager(t, "hooks-converter", "hooks-converter")
	ctx := context.Background()

	// Create a source file
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "input.pdf")
	err := os.WriteFile(sourcePath, []byte("pdf content"), 0644)
	require.NoError(t, err)

	// Create a target directory
	targetDir := t.TempDir()

	result, err := mgr.RunInputConverter(ctx, rt, sourcePath, targetDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, targetDir+"/output.epub", result.TargetPath)

	// Verify the converted file was created
	content, err := os.ReadFile(result.TargetPath)
	require.NoError(t, err)
	assert.Equal(t, "converted:pdf content", string(content))
}

func TestRunInputConverter_Failure(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Create a converter plugin that returns success: false
	destDir := filepath.Join(pluginDir, "test", "fail-converter")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "fail-converter",
  "name": "Fail Converter",
  "version": "1.0.0",
  "capabilities": {
    "inputConverter": {
      "description": "Converter that fails",
      "sourceTypes": ["pdf"],
      "targetType": "epub"
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(context) {
        return { success: false, targetPath: "" };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	manager := NewManager(service, pluginDir, "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "fail-converter",
		Name:        "Fail Converter",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = manager.LoadPlugin(ctx, "test", "fail-converter")
	require.NoError(t, err)

	rt := manager.GetRuntime("test", "fail-converter")
	require.NotNil(t, rt)

	result, err := manager.RunInputConverter(ctx, rt, "/nonexistent/source.pdf", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Empty(t, result.TargetPath)
}

func TestRunFileParser_AllFields(t *testing.T) {
	mgr, rt := setupHooksTestManager(t, "hooks-parser", "hooks-parser")
	ctx := context.Background()

	md, err := mgr.RunFileParser(ctx, rt, "/some/file.pdf", "pdf")
	require.NoError(t, err)
	require.NotNil(t, md)

	// String fields
	assert.Equal(t, "Test Book", md.Title)
	assert.Equal(t, "A Subtitle", md.Subtitle)
	assert.Equal(t, "Test Series", md.Series)
	assert.Equal(t, "A test book description", md.Description)
	assert.Equal(t, "Test Publisher", md.Publisher)
	assert.Equal(t, "Test Imprint", md.Imprint)
	assert.Equal(t, "https://example.com/book", md.URL)
	assert.Equal(t, "image/jpeg", md.CoverMimeType)

	// Authors
	require.Len(t, md.Authors, 2)
	assert.Equal(t, mediafile.ParsedAuthor{Name: "Author One", Role: "writer"}, md.Authors[0])
	assert.Equal(t, mediafile.ParsedAuthor{Name: "Author Two", Role: ""}, md.Authors[1])

	// Narrators
	assert.Equal(t, []string{"Narrator One", "Narrator Two"}, md.Narrators)

	// SeriesNumber
	require.NotNil(t, md.SeriesNumber)
	assert.InDelta(t, 2.5, *md.SeriesNumber, 0.001)

	// Genres and Tags
	assert.Equal(t, []string{"Fiction", "Fantasy"}, md.Genres)
	assert.Equal(t, []string{"epic", "adventure"}, md.Tags)

	// ReleaseDate
	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2023, md.ReleaseDate.Year())
	assert.Equal(t, time.June, md.ReleaseDate.Month())
	assert.Equal(t, 15, md.ReleaseDate.Day())

	// CoverData
	require.Len(t, md.CoverData, 4)
	assert.Equal(t, byte(0xFF), md.CoverData[0])
	assert.Equal(t, byte(0xD8), md.CoverData[1])

	// CoverPage
	require.NotNil(t, md.CoverPage)
	assert.Equal(t, 0, *md.CoverPage)

	// Duration (3661.5 seconds = 1h1m1.5s)
	assert.Equal(t, time.Duration(3661500000000), md.Duration)

	// BitrateBps
	assert.Equal(t, 128000, md.BitrateBps)

	// PageCount
	require.NotNil(t, md.PageCount)
	assert.Equal(t, 42, *md.PageCount)

	// Identifiers
	require.Len(t, md.Identifiers, 2)
	assert.Equal(t, mediafile.ParsedIdentifier{Type: "isbn_13", Value: "9781234567890"}, md.Identifiers[0])
	assert.Equal(t, mediafile.ParsedIdentifier{Type: "asin", Value: "B01ABCDEFG"}, md.Identifiers[1])

	// Chapters
	require.Len(t, md.Chapters, 2)
	assert.Equal(t, "Chapter 1", md.Chapters[0].Title)
	require.NotNil(t, md.Chapters[0].StartPage)
	assert.Equal(t, 0, *md.Chapters[0].StartPage)
	require.Len(t, md.Chapters[0].Children, 1)
	assert.Equal(t, "Section 1.1", md.Chapters[0].Children[0].Title)
	require.NotNil(t, md.Chapters[0].Children[0].StartPage)
	assert.Equal(t, 2, *md.Chapters[0].Children[0].StartPage)
	assert.Equal(t, "Chapter 2", md.Chapters[1].Title)
	require.NotNil(t, md.Chapters[1].StartPage)
	assert.Equal(t, 10, *md.Chapters[1].StartPage)
}

func TestRunFileParser_MinimalFields(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Create a parser that returns only title
	destDir := filepath.Join(pluginDir, "test", "minimal-parser")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "minimal-parser",
  "name": "Minimal Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Minimal parser",
      "types": ["txt"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(context) {
        return { title: "Minimal Title" };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	manager := NewManager(service, pluginDir, "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "minimal-parser",
		Name:        "Minimal Parser",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = manager.LoadPlugin(ctx, "test", "minimal-parser")
	require.NoError(t, err)

	rt := manager.GetRuntime("test", "minimal-parser")
	require.NotNil(t, rt)

	md, err := manager.RunFileParser(ctx, rt, "/some/file.txt", "txt")
	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, "Minimal Title", md.Title)
	assert.Empty(t, md.Subtitle)
	assert.Nil(t, md.Authors)
	assert.Nil(t, md.Narrators)
	assert.Empty(t, md.Series)
	assert.Nil(t, md.SeriesNumber)
	assert.Nil(t, md.Genres)
	assert.Nil(t, md.Tags)
	assert.Empty(t, md.Description)
	assert.Empty(t, md.Publisher)
	assert.Nil(t, md.ReleaseDate)
	assert.Nil(t, md.CoverData)
	assert.Nil(t, md.CoverPage)
	assert.Nil(t, md.PageCount)
	assert.Nil(t, md.Identifiers)
	assert.Nil(t, md.Chapters)
}

func TestRunMetadataSearch_ReturnsResults(t *testing.T) {
	mgr, rt := setupHooksTestManager(t, "hooks-enricher", "hooks-enricher")
	ctx := context.Background()

	searchCtx := map[string]interface{}{
		"query": "My Book",
		"book": map[string]interface{}{
			"title":   "My Book",
			"authors": []string{"Author A"},
		},
		"file": map[string]interface{}{
			"fileType": "epub",
			"filePath": "/library/book.epub",
		},
	}

	resp, err := mgr.RunMetadataSearch(ctx, rt, searchCtx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Results, 1)

	result := resp.Results[0]
	assert.Equal(t, "Search: My Book", result.Title)
	require.Len(t, result.Authors, 1)
	assert.Equal(t, "Search Author", result.Authors[0].Name)
	assert.Equal(t, "writer", result.Authors[0].Role)
	assert.Equal(t, "Search Publisher", result.Publisher)
	require.Len(t, result.Identifiers, 1)
	assert.Equal(t, "goodreads", result.Identifiers[0].Type)
}

func TestRunOutputGenerator_Success(t *testing.T) {
	mgr, rt := setupHooksTestManager(t, "hooks-generator", "hooks-generator")
	ctx := context.Background()

	// Create source file
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.epub")
	err := os.WriteFile(sourcePath, []byte("epub content"), 0644)
	require.NoError(t, err)

	// Dest file path
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.format")

	bookCtx := map[string]interface{}{
		"title":   "Test Book",
		"authors": []string{"Author A"},
	}
	fileCtx := map[string]interface{}{
		"fileType": "epub",
		"filePath": sourcePath,
	}

	err = mgr.RunOutputGenerator(ctx, rt, sourcePath, destPath, bookCtx, fileCtx)
	require.NoError(t, err)

	// Verify the output file was created
	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "generated:epub content", string(content))
}

func TestRunFingerprint(t *testing.T) {
	mgr, rt := setupHooksTestManager(t, "hooks-generator", "hooks-generator")
	_ = mgr

	bookCtx := map[string]interface{}{
		"title":   "My Book",
		"authors": []string{"Author A"},
	}
	fileCtx := map[string]interface{}{
		"fileType": "epub",
		"filePath": "/library/book.epub",
	}

	fp, err := mgr.RunFingerprint(rt, bookCtx, fileCtx)
	require.NoError(t, err)
	assert.Equal(t, "fp-My Book-epub", fp)

	// Call again to verify stability
	fp2, err := mgr.RunFingerprint(rt, bookCtx, fileCtx)
	require.NoError(t, err)
	assert.Equal(t, fp, fp2)
}

func TestRunInputConverter_NoHook(t *testing.T) {
	// Use the parser plugin which has no converter hook
	mgr, rt := setupHooksTestManager(t, "hooks-parser", "hooks-parser")
	ctx := context.Background()

	_, err := mgr.RunInputConverter(ctx, rt, "/source.pdf", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have an inputConverter hook")
}

func TestRunFileParser_NoHook(t *testing.T) {
	// Use the converter plugin which has no parser hook
	mgr, rt := setupHooksTestManager(t, "hooks-converter", "hooks-converter")
	ctx := context.Background()

	_, err := mgr.RunFileParser(ctx, rt, "/file.pdf", "pdf")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a fileParser hook")
}

func TestRunMetadataSearch_NoHook(t *testing.T) {
	// Use the converter plugin which has no enricher hook
	mgr, rt := setupHooksTestManager(t, "hooks-converter", "hooks-converter")
	ctx := context.Background()

	_, err := mgr.RunMetadataSearch(ctx, rt, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a metadataEnricher hook")
}

func TestRunMetadataSearch_NewFields(t *testing.T) {
	t.Parallel()
	mgr, rt := setupHooksTestManager(t, "hooks-enricher", "hooks-enricher")
	ctx := context.Background()

	searchCtx := map[string]interface{}{
		"query": "My Book",
		"book": map[string]interface{}{
			"title":   "My Book",
			"authors": []string{"Author A"},
		},
		"file": map[string]interface{}{
			"fileType": "epub",
			"filePath": "/library/book.epub",
		},
	}

	resp, err := mgr.RunMetadataSearch(ctx, rt, searchCtx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Results, 1)

	result := resp.Results[0]

	// Existing fields still work
	assert.Equal(t, "Search: My Book", result.Title)
	require.Len(t, result.Authors, 1)
	assert.Equal(t, "Search Author", result.Authors[0].Name)
	assert.Equal(t, "writer", result.Authors[0].Role)
	assert.Equal(t, "Search Publisher", result.Publisher)

	// Scalar fields
	assert.Equal(t, "A Search Subtitle", result.Subtitle)
	assert.Equal(t, "Search Series", result.Series)
	require.NotNil(t, result.SeriesNumber)
	assert.InDelta(t, 2.5, *result.SeriesNumber, 0.001)
	assert.Equal(t, "Search Imprint", result.Imprint)
	assert.Equal(t, "https://example.com/book", result.URL)
	assert.Equal(t, "https://example.com/cover.jpg", result.CoverURL)

	// Array fields
	assert.Equal(t, []string{"Fiction", "Fantasy"}, result.Genres)
	assert.Equal(t, []string{"epic", "adventure"}, result.Tags)
	assert.Equal(t, []string{"Narrator One", "Narrator Two"}, result.Narrators)
}

func TestRunMetadataSearch_NoNewFields(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	destDir := filepath.Join(pluginDir, "test", "minimal-enricher")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "minimal-enricher",
  "name": "Minimal Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Minimal",
      "fileTypes": ["epub"],
      "fields": ["title"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(context) {
        return { results: [{ title: "Just Title" }] };
      },
      enrich: function(context) {
        return { modified: false };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	manager := NewManager(service, pluginDir, "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "minimal-enricher",
		Name:        "Minimal Enricher",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = manager.LoadPlugin(ctx, "test", "minimal-enricher")
	require.NoError(t, err)

	rt := manager.GetRuntime("test", "minimal-enricher")
	require.NotNil(t, rt)

	searchCtx := map[string]interface{}{
		"query": "Test",
		"book":  map[string]interface{}{"title": "Test"},
		"file":  map[string]interface{}{"fileType": "epub"},
	}

	resp, err := manager.RunMetadataSearch(ctx, rt, searchCtx)
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)

	result := resp.Results[0]
	assert.Equal(t, "Just Title", result.Title)
	assert.Empty(t, result.Subtitle)
	assert.Empty(t, result.Series)
	assert.Nil(t, result.SeriesNumber)
	assert.Nil(t, result.Genres)
	assert.Nil(t, result.Tags)
	assert.Nil(t, result.Narrators)
}

func TestSearchResultCarriesAllMetadata(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Create a plugin where search() returns all metadata directly
	destDir := filepath.Join(pluginDir, "test", "full-search-enricher")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "full-search-enricher",
  "name": "Full Search Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Search carries all metadata",
      "fileTypes": ["epub"],
      "fields": ["title", "description", "genres", "cover"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(context) {
        return {
          results: [{
            title: "Full Title",
            description: "Full description",
            genres: ["SciFi"],
            coverUrl: "https://example.com/cover.jpg",
            authors: [{ name: "Full Author", role: "writer" }],
            imprint: "Full Imprint",
            url: "https://example.com/book"
          }]
        };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	manager := NewManager(service, pluginDir, "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "full-search-enricher",
		Name:        "Full Search Enricher",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = manager.LoadPlugin(ctx, "test", "full-search-enricher")
	require.NoError(t, err)

	rt := manager.GetRuntime("test", "full-search-enricher")
	require.NotNil(t, rt)

	searchCtx := map[string]interface{}{
		"query": "Test",
		"book":  map[string]interface{}{"title": "Test"},
		"file":  map[string]interface{}{"fileType": "epub"},
	}

	searchResp, err := manager.RunMetadataSearch(ctx, rt, searchCtx)
	require.NoError(t, err)
	require.Len(t, searchResp.Results, 1)

	md := searchResp.Results[0]
	assert.Equal(t, "Full Title", md.Title)
	assert.Equal(t, "Full description", md.Description)
	assert.Equal(t, []string{"SciFi"}, md.Genres)
	assert.Equal(t, "https://example.com/cover.jpg", md.CoverURL)
	require.Len(t, md.Authors, 1)
	assert.Equal(t, "Full Author", md.Authors[0].Name)
	assert.Equal(t, "writer", md.Authors[0].Role)
	assert.Equal(t, "Full Imprint", md.Imprint)
	assert.Equal(t, "https://example.com/book", md.URL)
}

func TestRunOutputGenerator_NoHook(t *testing.T) {
	// Use the converter plugin which has no generator hook
	mgr, rt := setupHooksTestManager(t, "hooks-converter", "hooks-converter")
	ctx := context.Background()

	err := mgr.RunOutputGenerator(ctx, rt, "/source", "/dest", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have an outputGenerator hook")
}

func TestRunFingerprint_NoHook(t *testing.T) {
	// Use the converter plugin which has no generator hook
	mgr, rt := setupHooksTestManager(t, "hooks-converter", "hooks-converter")

	_, err := mgr.RunFingerprint(rt, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have an outputGenerator hook")
}
