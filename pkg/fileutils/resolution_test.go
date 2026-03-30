package fileutils

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestJPEG(width, height int) []byte {
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

func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestImageResolution(t *testing.T) {
	t.Parallel()

	t.Run("returns width * height for JPEG", func(t *testing.T) {
		t.Parallel()
		data := createTestJPEG(800, 1200)
		assert.Equal(t, 800*1200, ImageResolution(data))
	})

	t.Run("returns width * height for PNG", func(t *testing.T) {
		t.Parallel()
		data := createTestPNG(640, 480)
		assert.Equal(t, 640*480, ImageResolution(data))
	})

	t.Run("returns 0 for invalid data", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageResolution([]byte("not an image")))
	})

	t.Run("returns 0 for empty data", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageResolution(nil))
	})
}

func TestImageFileResolution(t *testing.T) {
	t.Parallel()

	t.Run("returns resolution for JPEG file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.jpg")
		os.WriteFile(path, createTestJPEG(1024, 768), 0644)
		assert.Equal(t, 1024*768, ImageFileResolution(path))
	})

	t.Run("returns resolution for PNG file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.png")
		os.WriteFile(path, createTestPNG(500, 700), 0644)
		assert.Equal(t, 500*700, ImageFileResolution(path))
	})

	t.Run("returns 0 for nonexistent file", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageFileResolution("/nonexistent/path.jpg"))
	})

	t.Run("returns 0 for non-image file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		os.WriteFile(path, []byte("hello"), 0644)
		assert.Equal(t, 0, ImageFileResolution(path))
	})
}
