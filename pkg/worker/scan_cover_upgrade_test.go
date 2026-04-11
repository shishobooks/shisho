package worker

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

// TestEnricherCoverResolutionGate tests the resolution comparison logic
// that decides whether an enricher cover should replace the current cover.
func TestEnricherCoverResolutionGate(t *testing.T) {
	t.Parallel()

	t.Run("enricher cover larger than current is accepted", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(200, 300)   // 60,000 pixels
		enricher := makeJPEG(800, 1200) // 960,000 pixels

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		assert.Greater(t, enricherRes, currentRes)
	})

	t.Run("enricher cover same size as current is rejected", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(800, 1200)
		enricher := makeJPEG(800, 1200)

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		// enricherResolution <= currentResolution → skip
		assert.LessOrEqual(t, enricherRes, currentRes)
	})

	t.Run("enricher cover smaller than current is rejected", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(800, 1200)
		enricher := makeJPEG(200, 300)

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		assert.LessOrEqual(t, enricherRes, currentRes)
	})

	t.Run("no current cover — enricher always accepted", func(t *testing.T) {
		t.Parallel()
		enricher := makeJPEG(400, 600)

		currentRes := 0 // no cover on disk
		enricherRes := fileutils.ImageResolution(enricher)

		assert.Greater(t, enricherRes, currentRes)
	})

	t.Run("undecodable enricher cover is rejected", func(t *testing.T) {
		t.Parallel()
		enricherRes := fileutils.ImageResolution([]byte("not an image"))
		assert.Equal(t, 0, enricherRes)
	})
}

// TestEnricherCoverPageBasedGuard tests that page-based file types block enricher covers.
func TestEnricherCoverPageBasedGuard(t *testing.T) {
	t.Parallel()

	t.Run("CBZ blocks enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.True(t, models.IsPageBasedFileType(models.FileTypeCBZ))
	})

	t.Run("PDF blocks enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.True(t, models.IsPageBasedFileType(models.FileTypePDF))
	})

	t.Run("EPUB allows enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.False(t, models.IsPageBasedFileType(models.FileTypeEPUB))
	})

	t.Run("M4B allows enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.False(t, models.IsPageBasedFileType(models.FileTypeM4B))
	})
}

// TestEnricherCoverPluginSourceCheck tests that only plugin-sourced covers trigger upgrades.
func TestEnricherCoverPluginSourceCheck(t *testing.T) {
	t.Parallel()

	t.Run("plugin source is detected", func(t *testing.T) {
		t.Parallel()
		md := &mediafile.ParsedMetadata{
			CoverData:     makeJPEG(800, 1200),
			CoverMimeType: "image/jpeg",
			FieldDataSources: map[string]string{
				"cover": models.PluginDataSource("test", "enricher"),
			},
		}
		source := md.SourceForField("cover")
		assert.Greater(t, len(source), len(models.DataSourcePluginPrefix))
		assert.Equal(t, "plugin:", source[:7])
	})

	t.Run("file metadata source is not a plugin", func(t *testing.T) {
		t.Parallel()
		md := &mediafile.ParsedMetadata{
			CoverData:     makeJPEG(800, 1200),
			CoverMimeType: "image/jpeg",
			DataSource:    models.DataSourceEPUBMetadata,
		}
		source := md.SourceForField("cover")
		assert.NotEqual(t, "plugin:", source[:7])
	})
}

// TestUpgradeEnricherCover_RootLevelFile_SyntheticBookPath is a regression
// test for a bug where upgradeEnricherCover silently failed to write the
// upgraded cover when given a synthetic organized-folder bookPath that
// did not yet exist on disk. For root-level new files in libraries with
// OrganizeFileStructure enabled, scanFileCreateNew computes bookPath as
// filepath.Join(libraryPath, organizedFolderName) — a planned path that
// is not created until later in the batch. The cover dir must fall back
// to filepath.Dir(file.Filepath) (the library dir where extractAndSaveCover
// already wrote the embedded cover) in that case.
func TestUpgradeEnricherCover_RootLevelFile_SyntheticBookPath(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Simulate a root-level new file: a real file at the library root,
	// with a synthetic organized-folder bookPath that does not exist.
	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0644))

	// Write a small existing cover next to the file, as extractAndSaveCover
	// would have done before enrichers run.
	existingCoverPath := filepath.Join(libraryDir, "book.epub.cover.jpg")
	require.NoError(t, os.WriteFile(existingCoverPath, makeJPEG(200, 300), 0644))

	// Synthetic bookPath that does not exist on disk (this is what
	// scanFileCreateNew computes for a root-level file).
	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	_, err := os.Stat(syntheticBookPath)
	require.True(t, os.IsNotExist(err), "synthetic bookPath must not exist on disk")

	file := &models.File{
		Filepath: filePath,
		FileType: models.FileTypeEPUB,
	}

	// Plugin-sourced enricher metadata with a larger cover.
	metadata := &mediafile.ParsedMetadata{
		CoverData:     makeJPEG(800, 1200),
		CoverMimeType: "image/jpeg",
		FieldDataSources: map[string]string{
			"cover": models.PluginDataSource("test", "enricher"),
		},
	}

	tc.worker.upgradeEnricherCover(tc.ctx, metadata, file, syntheticBookPath, nil)

	// The upgraded cover must replace the existing one next to the file
	// (the library dir), not be silently dropped into the nonexistent
	// synthetic organized-folder path. Assert by resolution, not just
	// existence, since the pre-upgrade small cover also lives at this
	// path.
	upgradedCoverPath := filepath.Join(libraryDir, "book.epub.cover.jpg")
	upgradedBytes, err := os.ReadFile(upgradedCoverPath)
	require.NoError(t, err, "upgraded cover should exist at the library-dir location")
	upgradedRes := fileutils.ImageResolution(upgradedBytes)
	assert.Greater(t, upgradedRes, 200*300, "cover on disk should be the upgraded (larger) image, not the pre-upgrade small cover")

	// And nothing should have been written under the synthetic path.
	_, err = os.Stat(syntheticBookPath)
	assert.True(t, os.IsNotExist(err), "synthetic bookPath must still not exist after upgrade")
}
