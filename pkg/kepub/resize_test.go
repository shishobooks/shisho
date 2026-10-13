package kepub

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/draw"
)

// gradientImage returns a w×h image whose channels vary smoothly, so any
// reasonable resampler gives about the same result.
func gradientImage(w, h int, gray bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			r := uint8(255 * x / w)
			g := uint8(255 * y / h)
			b := uint8(255 * (x + y) / (w + h))
			if gray {
				g, b = r, r
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// Resizing a large page allocates little beyond the decoded source, however
// tall the page is. Not parallel: it measures the process's allocations.
func TestProcessImageForEreader_ResizeAllocations(t *testing.T) {
	for _, tc := range []struct {
		name string
		gray bool
	}{{"color", false}, {"gray", true}} {
		t.Run(tc.name, func(t *testing.T) {
			const w, h = 2000, 3000
			page := gradientImage(w, h, tc.gray)
			if tc.gray {
				// Encoded from a gray image, the JPEG decodes as gray.
				g := image.NewGray(page.Bounds())
				draw.Draw(g, g.Bounds(), page, image.Point{}, draw.Src)
				page = g
			}
			var buf bytes.Buffer
			require.NoError(t, jpeg.Encode(&buf, page, &jpeg.Options{Quality: 90}))
			data := buf.Bytes()

			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			processed := ProcessImageForEreader(data, ".jpg")
			runtime.ReadMemStats(&after)

			require.Equal(t, ".jpg", processed.Ext)
			assert.Less(t, processed.Width, w, "the page should have been resized")
			// The decoded JPEG takes at most 3 bytes per pixel.
			allocated := after.TotalAlloc - before.TotalAlloc
			assert.Less(t, allocated, uint64(4*w*h), "allocated %d MB to resize a %dx%d page", allocated>>20, w, h)
		})
	}
}

// shrink gives about what x/image/draw's CatmullRom does, for color and
// gray output, for sizes that do and do not divide evenly, and for a source
// whose bounds do not start at the origin.
func TestShrink_MatchesReferenceScaler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		w, h       int
		dw, dh     int
		gray, crop bool
	}{
		{"color uneven", 997, 1499, 316, 420, false, false},
		{"color halved", 800, 1200, 400, 600, false, false},
		{"gray uneven", 997, 1499, 316, 420, true, false},
		{"color cropped", 997, 1499, 300, 400, false, true},
		{"one row", 640, 1, 320, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := gradientImage(tc.w, tc.h, tc.gray)
			if tc.crop {
				src = src.(*image.RGBA).SubImage(image.Rect(13, 29, tc.w, tc.h))
			}

			got := shrink(src, tc.dw, tc.dh, tc.gray)
			require.Equal(t, image.Rect(0, 0, tc.dw, tc.dh), got.Bounds())

			want := image.NewRGBA(got.Bounds())
			draw.CatmullRom.Scale(want, want.Bounds(), src, src.Bounds(), draw.Src, nil)
			var diff, n float64
			for y := range tc.dh {
				for x := range tc.dw {
					gr, gg, gb, ga := got.At(x, y).RGBA()
					wr, wg, wb, _ := want.At(x, y).RGBA()
					require.Equal(t, uint32(0xffff), ga)
					for _, d := range [][2]uint32{{gr, wr}, {gg, wg}, {gb, wb}} {
						diff += math.Abs(float64(d[0]>>8) - float64(d[1]>>8))
						n++
					}
				}
			}
			assert.Less(t, diff/n, 1.0, "mean difference from CatmullRom per channel")
		})
	}
}

// A flat source stays flat: every output pixel's weights sum to 1.
func TestShrink_PreservesFlatColor(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 1003, 701))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 100, B: 37, A: 255}), image.Point{}, draw.Src)

	got := shrink(src, 311, 217, false).(*image.RGBA)
	for y := range 217 {
		for x := range 311 {
			require.Equal(t, color.RGBA{R: 200, G: 100, B: 37, A: 255}, got.RGBAAt(x, y), "pixel %d,%d", x, y)
		}
	}
	gray := shrink(src, 311, 217, true).(*image.Gray)
	want := color.GrayModel.Convert(color.RGBA{R: 200, G: 100, B: 37, A: 255}).(color.Gray)
	for y := range 217 {
		for x := range 311 {
			require.Equal(t, want, gray.GrayAt(x, y), "pixel %d,%d", x, y)
		}
	}
}
