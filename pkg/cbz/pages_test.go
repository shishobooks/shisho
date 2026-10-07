package cbz

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pageOrder(t *testing.T, entries ...string) []string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range entries {
		_, err := zw.Create(name)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	var names []string
	for _, f := range PageImages(zr) {
		names = append(names, f.Name)
	}
	return names
}

func TestPageImages(t *testing.T) {
	t.Parallel()

	t.Run("orders unpadded names naturally and skips macOS metadata", func(t *testing.T) {
		t.Parallel()
		got := pageOrder(t,
			"page10.jpg", "page2.jpg", "._page1.jpg", "ComicInfo.xml", "page1.jpg",
			"__MACOSX/._page1.jpg", "__MACOSX/page3.jpg", "page3.jpg",
		)
		assert.Equal(t, []string{"page1.jpg", "page2.jpg", "page3.jpg", "page10.jpg"}, got)
	})

	t.Run("keeps zero-padded names in byte order", func(t *testing.T) {
		t.Parallel()
		got := pageOrder(t, "p010.png", "p002.png", "p001.png", "p100.png")
		assert.Equal(t, []string{"p001.png", "p002.png", "p010.png", "p100.png"}, got)
	})

	t.Run("orders names equal but for leading zeros by bytes, not archive position", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"p01.jpg", "p1.jpg"}, pageOrder(t, "p1.jpg", "p01.jpg"))
		assert.Equal(t, []string{"p01.jpg", "p1.jpg"}, pageOrder(t, "p01.jpg", "p1.jpg"))
	})

	t.Run("orders chapter folders naturally", func(t *testing.T) {
		t.Parallel()
		got := pageOrder(t, "Chapter 10/page1.jpg", "Chapter 2/page1.jpg", "Chapter 1/page2.jpg", "Chapter 1/page1.jpg")
		assert.Equal(t, []string{"Chapter 1/page1.jpg", "Chapter 1/page2.jpg", "Chapter 2/page1.jpg", "Chapter 10/page1.jpg"}, got)
	})
}

func TestIsPageImage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected bool
	}{
		{"page.jpg", true},
		{"page.jpeg", true},
		{"page.JPG", true},
		{"page.png", true},
		{"page.PNG", true},
		{"page.gif", true},
		{"page.webp", true},
		{"Chapter 1/page.jpg", true},
		{"readme.txt", false},
		{"comic.cbz", false},
		{"metadata.xml", false},
		{"._page.jpg", false},
		{"Chapter 1/._page.jpg", false},
		{"__MACOSX/page.jpg", false},
		{"__MACOSX/Chapter 1/page.jpg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, IsPageImage(tt.name))
		})
	}
}

func TestNaturalCompare(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b     string
		expected int
	}{
		{"page1", "page2", -1},
		{"page2", "page10", -1},
		{"page10", "page2", 1},
		{"1", "2", -1},
		{"page001", "page002", -1},
		{"page2", "page2", 0},
		{"page", "page1", -1},
		// Equal but for leading zeros: byte order decides.
		{"p01", "p1", -1},
		{"p1", "p01", 1},
		// Filenames with a leading number (from the title) followed by the page number.
		// Must compare ALL numeric runs, not just the first.
		{"365 Days - c001 - p000.jpg", "365 Days - c001 - p001.jpg", -1},
		{"365 Days - c001 - p001.jpg", "365 Days - c001 - p000.jpg", 1},
		{"365 Days - c001 - p197.jpg", "365 Days - c002 - p000.jpg", -1},
		{"365 Days - c002 - p000.jpg", "365 Days - c001 - p197.jpg", 1},
		{"365 Days - c001 - p002-p003.jpg", "365 Days - c001 - p004.jpg", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, naturalCompare(tt.a, tt.b))
		})
	}
}
