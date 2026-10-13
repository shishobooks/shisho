package kepub

import (
	"image"
	"image/draw"
)

// shrink scales src down to w×h by area averaging: each output pixel is the
// mean of the source pixels it covers. w and h must not exceed src's size.
// It returns an *image.Gray of src's luma when gray is set, and an opaque
// *image.RGBA otherwise.
//
// It reads src one row at a time, so it needs only the output and a few
// rows of buffers. The kernel scalers in golang.org/x/image/draw hold a
// float64 RGBA buffer of the output width by the source height instead,
// hundreds of megabytes for a large page, once per page being processed.
func shrink(src image.Image, w, h int, gray bool) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	xs := shrinkWeights(sw, w)
	ys := shrinkWeights(sh, h)

	channels := 3
	var out image.Image
	var outPix []uint8
	var outStride int
	if gray {
		channels = 1
		g := image.NewGray(image.Rect(0, 0, w, h))
		out, outPix, outStride = g, g.Pix, g.Stride
	} else {
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		out, outPix, outStride = rgba, rgba.Pix, rgba.Stride
	}

	// row holds one source row, converted to RGBA by image/draw, which has
	// fast paths for the types the decoders return. sums holds that row
	// averaged across, cur the output row being summed, and next the share
	// of the source row that falls into the output row after it.
	row := image.NewRGBA(image.Rect(0, 0, sw, 1))
	sums := make([]float32, w*channels)
	cur := make([]float32, w*channels)
	next := make([]float32, w*channels)
	curY := 0

	emit := func() {
		dst := outPix[curY*outStride:]
		for x := range w {
			if gray {
				dst[x] = toUint8(cur[x])
				continue
			}
			dst[x*4] = toUint8(cur[x*3])
			dst[x*4+1] = toUint8(cur[x*3+1])
			dst[x*4+2] = toUint8(cur[x*3+2])
			dst[x*4+3] = 0xff
		}
	}

	for sy := range sh {
		draw.Draw(row, row.Bounds(), src, image.Pt(b.Min.X, b.Min.Y+sy), draw.Src)
		clear(sums)
		for sx, wt := range xs {
			p := row.Pix[sx*4 : sx*4+4 : sx*4+4]
			if gray {
				// color.GrayModel's weights, on 8-bit values.
				l := float32(19595*uint32(p[0])+38470*uint32(p[1])+7471*uint32(p[2])) / 65536
				sums[wt.i] += wt.w0 * l
				if wt.w1 > 0 {
					sums[wt.i+1] += wt.w1 * l
				}
				continue
			}
			o := wt.i * 3
			sums[o] += wt.w0 * float32(p[0])
			sums[o+1] += wt.w0 * float32(p[1])
			sums[o+2] += wt.w0 * float32(p[2])
			if wt.w1 > 0 {
				sums[o+3] += wt.w1 * float32(p[0])
				sums[o+4] += wt.w1 * float32(p[1])
				sums[o+5] += wt.w1 * float32(p[2])
			}
		}

		wt := ys[sy]
		if wt.i != curY {
			emit()
			cur, next = next, cur
			clear(next)
			curY = wt.i
		}
		for i, s := range sums {
			cur[i] += wt.w0 * s
		}
		if wt.w1 > 0 {
			for i, s := range sums {
				next[i] += wt.w1 * s
			}
		}
	}
	emit()
	return out
}

// shrinkWeight is how one source pixel splits between output pixel i and,
// when it straddles their edge, output pixel i+1, as fractions of an output
// pixel.
type shrinkWeight struct {
	i      int
	w0, w1 float32
}

// shrinkWeights maps each of n source pixels onto m <= n output pixels.
// Measured in units of 1/n of an output pixel, source pixel s spans
// [s*m, (s+1)*m) and output pixel i spans [i*n, (i+1)*n), so a source
// pixel crosses at most one output edge and each output pixel's weights
// sum to 1.
func shrinkWeights(n, m int) []shrinkWeight {
	weights := make([]shrinkWeight, n)
	for s := range weights {
		start, end := s*m, (s+1)*m
		i := start / n
		edge := (i + 1) * n
		if end <= edge || i+1 >= m {
			weights[s] = shrinkWeight{i: i, w0: float32(m) / float32(n)}
			continue
		}
		weights[s] = shrinkWeight{
			i:  i,
			w0: float32(edge-start) / float32(n),
			w1: float32(end-edge) / float32(n),
		}
	}
	return weights
}

func toUint8(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
