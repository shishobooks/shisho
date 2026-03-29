package worker

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
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
