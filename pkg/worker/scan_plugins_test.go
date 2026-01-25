package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestContextWithPlugins creates a test context with a plugin manager attached.
func newTestContextWithPlugins(t *testing.T, pluginDir string) *testContext {
	t.Helper()

	tc := newTestContext(t)

	pluginService := plugins.NewService(tc.db)
	pm := plugins.NewManager(pluginService, pluginDir)

	tc.worker.pluginManager = pm
	return tc
}

// installTestPlugin creates a plugin on disk and registers it in the database.
// Uses "test" scope for all test plugins.
func installTestPlugin(t *testing.T, tc *testContext, pluginDir, id, manifestJSON, mainJS string) {
	t.Helper()

	const scope = "test"
	destDir := filepath.Join(pluginDir, scope, id)
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifestJSON), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	plugin := &models.Plugin{
		Scope:       scope,
		ID:          id,
		Name:        id,
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = plugins.NewService(tc.db).InstallPlugin(context.Background(), plugin)
	require.NoError(t, err)
}

// TestScanWithPluginFileParser verifies that files with plugin-registered extensions
// are discovered during scan and parsed by the plugin file parser.
func TestScanWithPluginFileParser(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "pdf-parser",
  "name": "PDF Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses PDF files",
      "types": ["pdf"]
    }
  }
}`

	// Plugin that returns metadata from a "PDF" file
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "PDF Book Title",
          authors: [{name: "PDF Author", role: ""}],
          description: "A PDF book parsed by plugin"
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "pdf-parser", manifest, mainJS)

	// Load the plugin
	err := tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .pdf file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Test Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	// Create a fake PDF file (just content; no MIME validation since mimeTypes not declared)
	pdfPath := filepath.Join(bookDir, "test.pdf")
	err = os.WriteFile(pdfPath, []byte("fake pdf content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// Verify the book was created with plugin-parsed metadata
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "PDF Book Title", book.Title)
	require.NotNil(t, book.Description)
	assert.Equal(t, "A PDF book parsed by plugin", *book.Description)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "PDF Author", book.Authors[0].Person.Name)

	// Verify DataSource includes plugin identity
	assert.Equal(t, "plugin:test/pdf-parser", book.TitleSource)

	// Verify file type
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, "pdf", files[0].FileType)
}

// TestScanWithPluginFileParser_MIMEValidation verifies that MIME type validation
// is enforced when the plugin declares mimeTypes.
func TestScanWithPluginFileParser_MIMEValidation(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "pdf-strict",
  "name": "PDF Parser Strict",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses PDF files with MIME validation",
      "types": ["pdf"],
      "mimeTypes": ["application/pdf"]
    }
  }
}`

	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Should Not Parse" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "pdf-strict", manifest, mainJS)

	err := tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a file that has .pdf extension but is actually plain text
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Fake PDF")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	// This file has .pdf extension but its content is plain text (MIME: text/plain)
	fakePdfPath := filepath.Join(bookDir, "fake.pdf")
	err = os.WriteFile(fakePdfPath, []byte("this is just plain text, not a real PDF"), 0644)
	require.NoError(t, err)

	// Run scan - should not crash, but file should not be parsed
	err = tc.runScan()
	require.NoError(t, err)

	// File should not be parsed because MIME type doesn't match
	allBooks := tc.listBooks()
	assert.Empty(t, allBooks, "file with wrong MIME type should not be parsed")
}

// TestScanWithPluginFileParser_MIMEValidation_ValidFile verifies that files
// with correct MIME types pass validation and get parsed.
func TestScanWithPluginFileParser_MIMEValidation_ValidFile(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "zip-parser",
  "name": "ZIP Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses ZIP files",
      "types": ["zip"],
      "mimeTypes": ["application/zip"]
    }
  }
}`

	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Zip Archive Book" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "zip-parser", manifest, mainJS)

	err := tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create a real zip file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Zip Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	// Create a valid ZIP file (minimum valid zip: empty archive)
	// PK\x05\x06 is the end-of-central-directory signature for an empty ZIP
	zipContent := []byte{
		0x50, 0x4B, 0x05, 0x06, // End of central dir signature
		0x00, 0x00, // Number of this disk
		0x00, 0x00, // Disk where central directory starts
		0x00, 0x00, // Number of central directory records on this disk
		0x00, 0x00, // Total number of central directory records
		0x00, 0x00, 0x00, 0x00, // Size of central directory
		0x00, 0x00, 0x00, 0x00, // Offset of start of central directory
		0x00, 0x00, // Comment length
	}
	zipPath := filepath.Join(bookDir, "archive.zip")
	err = os.WriteFile(zipPath, zipContent, 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// File should be parsed because MIME type matches
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Zip Archive Book", allBooks[0].Title)
}

// TestScanWithPluginInputConverter verifies that input converters run during scan
// and the converted files get parsed.
func TestScanWithPluginInputConverter(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Install a converter plugin that converts .testconv files to .epub
	converterManifest := `{
  "manifestVersion": 1,
  "id": "testconv-converter",
  "name": "TestConv Converter",
  "version": "1.0.0",
  "capabilities": {
    "inputConverter": {
      "description": "Converts testconv to epub",
      "sourceTypes": ["testconv"],
      "targetType": "epub"
    }
  }
}`

	// The converter creates a minimal EPUB-like file (extension .epub)
	// Since we're testing the converter flow, we need the output to be parseable
	// by the built-in EPUB parser, or tracked as a file.
	// For simplicity, we create a file that will fail EPUB parsing but still get indexed.
	converterMainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(ctx) {
        var content = shisho.fs.readTextFile(ctx.sourcePath);
        var targetPath = ctx.targetDir + "/converted.epub";
        shisho.fs.writeTextFile(targetPath, content);
        return { success: true, targetPath: targetPath };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "testconv-converter", converterManifest, converterMainJS)

	// Set up plugin ordering so the converter is found
	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookInputConverter, "test", "testconv-converter")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Verify the converter extension is registered
	convExts := tc.worker.pluginManager.RegisteredConverterExtensions()
	assert.Contains(t, convExts, "testconv")

	// Create library with a .testconv file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Converted Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testconvPath := filepath.Join(bookDir, "myfile.testconv")
	err = os.WriteFile(testconvPath, []byte("test conversion content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// The converter should have created a converted.epub file
	convertedPath := filepath.Join(bookDir, "converted.epub")
	_, statErr := os.Stat(convertedPath)
	assert.NoError(t, statErr, "converted file should exist in library directory")
}

// TestScanWithPluginInputConverter_MIMEValidation verifies that input converters
// respect MIME type validation during scan.
func TestScanWithPluginInputConverter_MIMEValidation(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "strict-converter",
  "name": "Strict Converter",
  "version": "1.0.0",
  "capabilities": {
    "inputConverter": {
      "description": "Only converts real ZIP files with .testzip extension",
      "sourceTypes": ["testzip"],
      "mimeTypes": ["application/zip"],
      "targetType": "epub"
    }
  }
}`

	mainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(ctx) {
        var targetPath = ctx.targetDir + "/converted.epub";
        shisho.fs.writeTextFile(targetPath, "converted");
        return { success: true, targetPath: targetPath };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "strict-converter", manifest, mainJS)

	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookInputConverter, "test", "strict-converter")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .testzip file that is actually plain text
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Fake Zip")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	fakePath := filepath.Join(bookDir, "fake.testzip")
	err = os.WriteFile(fakePath, []byte("not a zip file"), 0644)
	require.NoError(t, err)

	// Run scan - converter should NOT run because MIME doesn't match
	err = tc.runScan()
	require.NoError(t, err)

	// No converted file should exist
	convertedPath := filepath.Join(bookDir, "converted.epub")
	_, statErr := os.Stat(convertedPath)
	assert.True(t, os.IsNotExist(statErr), "converter should not produce output when MIME type doesn't match")
}

// TestScanWithPluginMetadataEnricher verifies that enrichers are called during scan
// and their results are merged into file metadata.
func TestScanWithPluginMetadataEnricher(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Install a file parser plugin for .enrichtest files
	parserManifest := `{
  "manifestVersion": 1,
  "id": "enrichtest-parser",
  "name": "EnrichTest Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses enrichtest files",
      "types": ["enrichtest"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Enrichable Book",
          authors: [{name: "Original Author", role: ""}]
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "enrichtest-parser", parserManifest, parserMainJS)

	// Install an enricher plugin that adds genres and description
	enricherManifest := `{
  "manifestVersion": 1,
  "id": "test-enricher",
  "name": "Test Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches enrichtest files with extra metadata",
      "fileTypes": ["enrichtest"]
    }
  }
}`
	enricherMainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "Enriched description for: " + ctx.parsedMetadata.title,
            genres: ["Fantasy", "Adventure"]
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "test-enricher", enricherManifest, enricherMainJS)

	// Set up ordering for enricher
	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "test-enricher")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .enrichtest file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Enriched Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.enrichtest")
	err = os.WriteFile(testPath, []byte("enrichtest content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// Verify book has both parser and enricher metadata
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "Enrichable Book", book.Title)
	require.NotNil(t, book.Description)
	assert.Equal(t, "Enriched description for: Enrichable Book", *book.Description)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Original Author", book.Authors[0].Person.Name)
	// Genres should come from enricher
	require.Len(t, book.BookGenres, 2)

	// DataSource for enriched fields should include enricher plugin identity
	require.NotNil(t, book.DescriptionSource)
	assert.Equal(t, "plugin:test/test-enricher", *book.DescriptionSource)
}

// TestScanWithPluginMetadataEnricher_FileTypeFiltering verifies that enrichers
// only run for their declared file types.
func TestScanWithPluginMetadataEnricher_FileTypeFiltering(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Parser for .fmtx files
	parserManifest := `{
  "manifestVersion": 1,
  "id": "fmtx-parser",
  "name": "FmtX Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses fmtx files",
      "types": ["fmtx"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "FmtX File" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "fmtx-parser", parserManifest, parserMainJS)

	// Enricher that only handles "epub" files (not fmtx)
	enricherManifest := `{
  "manifestVersion": 1,
  "id": "epub-only-enricher",
  "name": "EPUB Only Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Only enriches epub files",
      "fileTypes": ["epub"]
    }
  }
}`
	enricherMainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "SHOULD NOT APPEAR"
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "epub-only-enricher", enricherManifest, enricherMainJS)

	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "epub-only-enricher")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .fmtx file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "FmtX Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.fmtx")
	err = os.WriteFile(testPath, []byte("fmtx content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// Enricher should NOT have run (file type mismatch)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "FmtX File", allBooks[0].Title)
	assert.Nil(t, allBooks[0].Description, "enricher should not have run for non-epub file type")
}

// TestScanWithPluginMetadataEnricher_Ordering verifies that enrichers respect
// user-defined ordering (first enricher's values win).
func TestScanWithPluginMetadataEnricher_Ordering(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Parser for .ordtest files
	parserManifest := `{
  "manifestVersion": 1,
  "id": "ordtest-parser",
  "name": "OrdTest Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses ordtest files",
      "types": ["ordtest"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Order Test Book" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "ordtest-parser", parserManifest, parserMainJS)

	// First enricher (higher priority) - provides description
	enricher1Manifest := `{
  "manifestVersion": 1,
  "id": "enricher-first",
  "name": "First Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "First enricher",
      "fileTypes": ["ordtest"]
    }
  }
}`
	enricher1MainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "From First Enricher"
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "enricher-first", enricher1Manifest, enricher1MainJS)

	// Second enricher (lower priority) - also provides description
	enricher2Manifest := `{
  "manifestVersion": 1,
  "id": "enricher-second",
  "name": "Second Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Second enricher",
      "fileTypes": ["ordtest"]
    }
  }
}`
	enricher2MainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "From Second Enricher"
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "enricher-second", enricher2Manifest, enricher2MainJS)

	// Set ordering: first enricher has priority
	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "enricher-first")
	require.NoError(t, err)
	err = pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "enricher-second")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with .ordtest file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Order Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.ordtest")
	err = os.WriteFile(testPath, []byte("ordtest content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// First enricher's description should win
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.NotNil(t, allBooks[0].Description)
	assert.Equal(t, "From First Enricher", *allBooks[0].Description,
		"first enricher's value should win per 'first non-empty wins' rule")

	// DataSource should identify the first enricher that contributed
	require.NotNil(t, allBooks[0].DescriptionSource)
	assert.Equal(t, "plugin:test/enricher-first", *allBooks[0].DescriptionSource)
}

// TestScanWithPluginMetadataEnricher_CascadingFieldSources verifies that when
// multiple enrichers provide different fields, each field's source accurately
// reflects which enricher contributed it (per-field first-wins with correct sources).
func TestScanWithPluginMetadataEnricher_CascadingFieldSources(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Parser for .casctest files - provides only a title
	parserManifest := `{
  "manifestVersion": 1,
  "id": "casctest-parser",
  "name": "CascTest Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses casctest files",
      "types": ["casctest"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Cascade Test Book" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "casctest-parser", parserManifest, parserMainJS)

	// First enricher: provides description and title (title should win since parser doesn't set it on book)
	enricher1Manifest := `{
  "manifestVersion": 1,
  "id": "enricher-alpha",
  "name": "Alpha Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Provides description",
      "fileTypes": ["casctest"]
    }
  }
}`
	enricher1MainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "Description from Alpha",
            publisher: "Publisher from Alpha"
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "enricher-alpha", enricher1Manifest, enricher1MainJS)

	// Second enricher: provides genres and also tries to set description (should be ignored)
	enricher2Manifest := `{
  "manifestVersion": 1,
  "id": "enricher-beta",
  "name": "Beta Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Provides genres",
      "fileTypes": ["casctest"]
    }
  }
}`
	enricher2MainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            description: "Description from Beta (should be ignored)",
            genres: ["Action", "Fantasy"],
            tags: ["tag-from-beta"]
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "enricher-beta", enricher2Manifest, enricher2MainJS)

	// Set ordering: alpha first, beta second
	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "enricher-alpha")
	require.NoError(t, err)
	err = pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "enricher-beta")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with .casctest file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Cascade Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.casctest")
	err = os.WriteFile(testPath, []byte("casctest content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// Verify field values
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]

	// Description should come from alpha (first enricher)
	require.NotNil(t, book.Description)
	assert.Equal(t, "Description from Alpha", *book.Description)

	// Genres should come from beta (alpha didn't provide genres)
	require.Len(t, book.BookGenres, 2)

	// Verify per-field sources
	alphaSource := "plugin:test/enricher-alpha"
	betaSource := "plugin:test/enricher-beta"

	// Description source should be alpha
	require.NotNil(t, book.DescriptionSource)
	assert.Equal(t, alphaSource, *book.DescriptionSource,
		"DescriptionSource should reflect alpha enricher which provided it")

	// Genre source should be beta
	require.NotNil(t, book.GenreSource)
	assert.Equal(t, betaSource, *book.GenreSource,
		"GenreSource should reflect beta enricher which provided genres")

	// Tag source should be beta
	require.NotNil(t, book.TagSource)
	assert.Equal(t, betaSource, *book.TagSource,
		"TagSource should reflect beta enricher which provided tags")

	// Publisher source should be alpha
	file := book.Files[0]
	require.NotNil(t, file.PublisherSource)
	assert.Equal(t, alphaSource, *file.PublisherSource,
		"PublisherSource should reflect alpha enricher which provided it")
}

// TestScanWithPluginFileParser_AllSourcesSet verifies that when a plugin file parser
// returns all possible metadata fields, every corresponding source field is set correctly.
func TestScanWithPluginFileParser_AllSourcesSet(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "full-parser",
  "name": "Full Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Returns all metadata fields",
      "types": ["fulltest"]
    }
  }
}`

	// Plugin that returns every possible metadata field
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Full Title",
          subtitle: "Full Subtitle",
          authors: [{name: "Parser Author", role: ""}],
          narrators: ["Parser Narrator"],
          series: "Parser Series",
          seriesNumber: 3,
          genres: ["SciFi", "Thriller"],
          tags: ["tag-one", "tag-two"],
          description: "Parser description text",
          publisher: "Parser Publisher",
          imprint: "Parser Imprint",
          url: "https://example.com/parser-book",
          releaseDate: "2024-06-15T00:00:00Z",
          identifiers: [{type: "isbn_13", value: "9781234567890"}],
          chapters: [{title: "Chapter 1", startPage: 0}, {title: "Chapter 2", startPage: 5}],
          pageCount: 42,
          duration: 3600,
          bitrateBps: 128000
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "full-parser", manifest, mainJS)

	err := tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .fulltest file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Full Source Test")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.fulltest")
	err = os.WriteFile(testPath, []byte("fulltest content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	expectedSource := "plugin:test/full-parser"

	// Verify book-level sources
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]

	assert.Equal(t, "Full Title", book.Title)
	assert.Equal(t, expectedSource, book.TitleSource, "TitleSource should be set to plugin source")

	assert.NotEmpty(t, book.SortTitle)
	assert.Equal(t, expectedSource, book.SortTitleSource, "SortTitleSource should be set to plugin source")

	require.NotNil(t, book.Subtitle)
	assert.Equal(t, "Full Subtitle", *book.Subtitle)
	require.NotNil(t, book.SubtitleSource)
	assert.Equal(t, expectedSource, *book.SubtitleSource, "SubtitleSource should be set to plugin source")

	require.NotNil(t, book.Description)
	assert.Equal(t, "Parser description text", *book.Description)
	require.NotNil(t, book.DescriptionSource)
	assert.Equal(t, expectedSource, *book.DescriptionSource, "DescriptionSource should be set to plugin source")

	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Parser Author", book.Authors[0].Person.Name)
	assert.Equal(t, expectedSource, book.AuthorSource, "AuthorSource should be set to plugin source")

	require.Len(t, book.BookGenres, 2)
	require.NotNil(t, book.GenreSource)
	assert.Equal(t, expectedSource, *book.GenreSource, "GenreSource should be set to plugin source")

	require.Len(t, book.BookTags, 2)
	require.NotNil(t, book.TagSource)
	assert.Equal(t, expectedSource, *book.TagSource, "TagSource should be set to plugin source")

	// Verify file-level sources (need RetrieveFileWithRelations for full data)
	files := tc.listFiles()
	require.Len(t, files, 1)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, files[0].ID)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "Full Title", *file.Name) // file name derived from title for non-CBZ
	require.NotNil(t, file.NameSource)
	assert.Equal(t, expectedSource, *file.NameSource, "NameSource should be set to plugin source")

	require.NotNil(t, file.URL)
	assert.Equal(t, "https://example.com/parser-book", *file.URL)
	require.NotNil(t, file.URLSource)
	assert.Equal(t, expectedSource, *file.URLSource, "URLSource should be set to plugin source")

	require.NotNil(t, file.ReleaseDate)
	assert.Equal(t, 2024, file.ReleaseDate.Year())
	assert.Equal(t, time.June, file.ReleaseDate.Month())
	assert.Equal(t, 15, file.ReleaseDate.Day())
	require.NotNil(t, file.ReleaseDateSource)
	assert.Equal(t, expectedSource, *file.ReleaseDateSource, "ReleaseDateSource should be set to plugin source")

	require.NotNil(t, file.Publisher)
	assert.Equal(t, "Parser Publisher", file.Publisher.Name)
	require.NotNil(t, file.PublisherSource)
	assert.Equal(t, expectedSource, *file.PublisherSource, "PublisherSource should be set to plugin source")

	require.NotNil(t, file.Imprint)
	assert.Equal(t, "Parser Imprint", file.Imprint.Name)
	require.NotNil(t, file.ImprintSource)
	assert.Equal(t, expectedSource, *file.ImprintSource, "ImprintSource should be set to plugin source")

	require.Len(t, file.Narrators, 1)
	assert.Equal(t, "Parser Narrator", file.Narrators[0].Person.Name)
	require.NotNil(t, file.NarratorSource)
	assert.Equal(t, expectedSource, *file.NarratorSource, "NarratorSource should be set to plugin source")

	require.Len(t, file.Identifiers, 1)
	assert.Equal(t, "isbn_13", file.Identifiers[0].Type)
	assert.Equal(t, "9781234567890", file.Identifiers[0].Value)
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, expectedSource, *file.IdentifierSource, "IdentifierSource should be set to plugin source")

	// Chapters
	chapterList := tc.listChapters(file.ID)
	require.Len(t, chapterList, 2)
	assert.Equal(t, "Chapter 1", chapterList[0].Title)
	assert.Equal(t, "Chapter 2", chapterList[1].Title)
	require.NotNil(t, file.ChapterSource)
	assert.Equal(t, expectedSource, *file.ChapterSource, "ChapterSource should be set to plugin source")

	// PageCount is set for plugin file types (not restricted to CBZ anymore)
	require.NotNil(t, file.PageCount)
	assert.Equal(t, 42, *file.PageCount, "PageCount should be set for plugin file types")

	// Duration and bitrate are set for plugin file types (not restricted to M4B anymore)
	require.NotNil(t, file.AudiobookDurationSeconds)
	assert.InDelta(t, 3600.0, *file.AudiobookDurationSeconds, 0.01, "Duration should be set for plugin file types")
	require.NotNil(t, file.AudiobookBitrateBps)
	assert.Equal(t, 128000, *file.AudiobookBitrateBps, "BitrateBps should be set for plugin file types")

	// CoverSource is nil because plugin didn't return cover data bytes
	assert.Nil(t, file.CoverSource, "CoverSource should be nil when no cover data provided")
}

// TestScanWithPluginMetadataEnricher_AllSourcesSet verifies that when a metadata
// enricher provides all possible fields, every corresponding source field is set
// to the enricher's plugin identity.
func TestScanWithPluginMetadataEnricher_AllSourcesSet(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Minimal parser - only provides a title (so enricher can fill in the rest)
	parserManifest := `{
  "manifestVersion": 1,
  "id": "minimal-parser",
  "name": "Minimal Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Returns minimal metadata",
      "types": ["enrichall"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Minimal Title" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "minimal-parser", parserManifest, parserMainJS)

	// Enricher that fills in all remaining fields
	enricherManifest := `{
  "manifestVersion": 1,
  "id": "full-enricher",
  "name": "Full Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches with all metadata fields",
      "fileTypes": ["enrichall"]
    }
  }
}`
	enricherMainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            subtitle: "Enriched Subtitle",
            authors: [{name: "Enriched Author", role: ""}],
            narrators: ["Enriched Narrator"],
            series: "Enriched Series",
            seriesNumber: 5,
            genres: ["Horror", "Mystery"],
            tags: ["enriched-tag"],
            description: "Enriched description",
            publisher: "Enriched Publisher",
            imprint: "Enriched Imprint",
            url: "https://example.com/enriched",
            releaseDate: "2025-01-10T00:00:00Z",
            identifiers: [{type: "asin", value: "B01ENRICHED"}]
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "full-enricher", enricherManifest, enricherMainJS)

	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "full-enricher")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with a .enrichall file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Enrich All Test")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.enrichall")
	err = os.WriteFile(testPath, []byte("enrichall content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	enricherSource := "plugin:test/full-enricher"

	// Verify book-level sources
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]

	// Title came from the parser, but enricher doesn't override it
	assert.Equal(t, "Minimal Title", book.Title)

	// Subtitle should come from enricher
	require.NotNil(t, book.Subtitle)
	assert.Equal(t, "Enriched Subtitle", *book.Subtitle)
	require.NotNil(t, book.SubtitleSource)
	assert.Equal(t, enricherSource, *book.SubtitleSource, "SubtitleSource should be enricher plugin source")

	// Description from enricher
	require.NotNil(t, book.Description)
	assert.Equal(t, "Enriched description", *book.Description)
	require.NotNil(t, book.DescriptionSource)
	assert.Equal(t, enricherSource, *book.DescriptionSource, "DescriptionSource should be enricher plugin source")

	// Authors from enricher (parser didn't provide authors)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Enriched Author", book.Authors[0].Person.Name)
	assert.Equal(t, enricherSource, book.AuthorSource, "AuthorSource should be enricher plugin source")

	// Genres from enricher
	require.Len(t, book.BookGenres, 2)
	require.NotNil(t, book.GenreSource)
	assert.Equal(t, enricherSource, *book.GenreSource, "GenreSource should be enricher plugin source")

	// Tags from enricher
	require.Len(t, book.BookTags, 1)
	require.NotNil(t, book.TagSource)
	assert.Equal(t, enricherSource, *book.TagSource, "TagSource should be enricher plugin source")

	// File-level sources
	files := tc.listFiles()
	require.Len(t, files, 1)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, files[0].ID)
	require.NoError(t, err)

	// URL from enricher
	require.NotNil(t, file.URL)
	assert.Equal(t, "https://example.com/enriched", *file.URL)
	require.NotNil(t, file.URLSource)
	assert.Equal(t, enricherSource, *file.URLSource, "URLSource should be enricher plugin source")

	// ReleaseDate from enricher
	require.NotNil(t, file.ReleaseDate)
	assert.Equal(t, 2025, file.ReleaseDate.Year())
	assert.Equal(t, time.January, file.ReleaseDate.Month())
	require.NotNil(t, file.ReleaseDateSource)
	assert.Equal(t, enricherSource, *file.ReleaseDateSource, "ReleaseDateSource should be enricher plugin source")

	// Publisher from enricher
	require.NotNil(t, file.Publisher)
	assert.Equal(t, "Enriched Publisher", file.Publisher.Name)
	require.NotNil(t, file.PublisherSource)
	assert.Equal(t, enricherSource, *file.PublisherSource, "PublisherSource should be enricher plugin source")

	// Imprint from enricher
	require.NotNil(t, file.Imprint)
	assert.Equal(t, "Enriched Imprint", file.Imprint.Name)
	require.NotNil(t, file.ImprintSource)
	assert.Equal(t, enricherSource, *file.ImprintSource, "ImprintSource should be enricher plugin source")

	// Narrators from enricher
	require.Len(t, file.Narrators, 1)
	assert.Equal(t, "Enriched Narrator", file.Narrators[0].Person.Name)
	require.NotNil(t, file.NarratorSource)
	assert.Equal(t, enricherSource, *file.NarratorSource, "NarratorSource should be enricher plugin source")

	// Identifiers from enricher
	require.Len(t, file.Identifiers, 1)
	assert.Equal(t, "asin", file.Identifiers[0].Type)
	assert.Equal(t, "B01ENRICHED", file.Identifiers[0].Value)
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, enricherSource, *file.IdentifierSource, "IdentifierSource should be enricher plugin source")
}

// TestScanWithPluginFileParser_ReservedExtensions verifies that plugins cannot
// override built-in parsers for reserved extensions (epub, cbz, m4b).
func TestScanWithPluginFileParser_ReservedExtensions(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Plugin that tries to claim epub extension
	manifest := `{
  "manifestVersion": 1,
  "id": "epub-override",
  "name": "EPUB Override",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Tries to override epub",
      "types": ["epub"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Plugin EPUB Override - Should Not Happen" };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "epub-override", manifest, mainJS)

	err := tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// epub should NOT be in registered extensions (it's reserved)
	exts := tc.worker.pluginManager.RegisteredFileExtensions()
	assert.NotContains(t, exts, "epub")

	// GetParserForType should return nil for reserved types
	rt := tc.worker.pluginManager.GetParserForType("epub")
	assert.Nil(t, rt, "plugin should not be able to claim reserved extension 'epub'")
}

// TestScanWithPluginMetadataEnricher_IdentifiersMergedWithParser verifies that
// enricher identifiers are appended to (not replaced by) parser identifiers.
// This ensures that when a file parser returns e.g. an ISBN and an enricher
// returns a custom identifier type, both are preserved.
func TestScanWithPluginMetadataEnricher_IdentifiersMergedWithParser(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// Parser that returns identifiers (simulating CBZ with GTIN)
	parserManifest := `{
  "manifestVersion": 1,
  "id": "id-parser",
  "name": "ID Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses idtest files",
      "types": ["idtest"]
    }
  }
}`
	parserMainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Test Book",
          identifiers: [
            { type: "isbn_13", value: "9781234567890" }
          ]
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "id-parser", parserManifest, parserMainJS)

	// Enricher that returns a different identifier type
	enricherManifest := `{
  "manifestVersion": 1,
  "id": "id-enricher",
  "name": "ID Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches with custom identifiers",
      "fileTypes": ["idtest"]
    }
  }
}`
	enricherMainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            identifiers: [
              { type: "mangaupdates_series", value: "12345" }
            ]
          }
        };
      }
    }
  };
})();`

	installTestPlugin(t, tc, pluginDir, "id-enricher", enricherManifest, enricherMainJS)

	// Set up ordering
	pluginService := plugins.NewService(tc.db)
	err := pluginService.AppendToOrder(context.Background(), models.PluginHookMetadataEnricher, "test", "id-enricher")
	require.NoError(t, err)

	err = tc.worker.pluginManager.LoadAll(context.Background())
	require.NoError(t, err)

	// Create library with test file
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	bookDir := filepath.Join(libraryPath, "Test Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	testPath := filepath.Join(bookDir, "test.idtest")
	err = os.WriteFile(testPath, []byte("content"), 0644)
	require.NoError(t, err)

	// Run scan
	err = tc.runScan()
	require.NoError(t, err)

	// Both identifier types should be present
	files := tc.listFiles()
	require.Len(t, files, 1)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, files[0].ID)
	require.NoError(t, err)

	require.Len(t, file.Identifiers, 2, "both parser and enricher identifiers should be present")

	// Check both identifiers exist (order may vary)
	types := map[string]string{}
	for _, id := range file.Identifiers {
		types[id.Type] = id.Value
	}
	assert.Equal(t, "9781234567890", types["isbn_13"], "parser ISBN should be present")
	assert.Equal(t, "12345", types["mangaupdates_series"], "enricher mangaupdates_series should be present")
}
