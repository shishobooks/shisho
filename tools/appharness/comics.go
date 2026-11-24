package main

import (
	"archive/zip"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strconv"

	"github.com/pkg/errors"
)

// comicKind is the shape of a fixture comic's pages, for the page reader's
// flows and for trying it by hand against `mise start:qa`.
type comicKind int

const (
	// comicPrinted is a printed comic: portrait pages, with one wide page
	// (two printed pages scanned as one), which stands alone in landscape.
	comicPrinted comicKind = iota + 1
	// comicManga is printed pages that run right to left.
	comicManga
	// comicWebtoon is a cover, then strips several times taller than wide,
	// made for scrolling: the reader opens it scrolled.
	comicWebtoon
)

// comicPage is one page's size and colors.
type comicPage struct {
	width, height int
	background    color.RGBA
}

// comicPages are the pages of a comic of kind.
func comicPages(kind comicKind) []comicPage {
	printed := func(i int) comicPage {
		return comicPage{width: 600, height: 900, background: comicColors[i%len(comicColors)]}
	}
	var pages []comicPage
	switch kind {
	case comicPrinted:
		for i := range 8 {
			page := printed(i)
			if i == 4 {
				page.width = 1200
			}
			pages = append(pages, page)
		}
	case comicManga:
		for i := range 6 {
			pages = append(pages, printed(i))
		}
	case comicWebtoon:
		pages = append(pages, printed(0))
		for i := 1; i < 6; i++ {
			pages = append(pages, comicPage{width: 400, height: 1400, background: comicColors[i%len(comicColors)]})
		}
	}
	return pages
}

var comicColors = []color.RGBA{
	{R: 40, G: 70, B: 140, A: 255},
	{R: 150, G: 60, B: 50, A: 255},
	{R: 50, G: 120, B: 80, A: 255},
	{R: 130, G: 100, B: 40, A: 255},
	{R: 90, G: 60, B: 130, A: 255},
}

// writeComic writes a CBZ of kind whose pages show their page numbers, from
// 1, large enough to read at a glance, with a ComicInfo.xml naming its
// title and penciller, and for manga its Reading Direction.
func writeComic(path, title, author string, kind comicKind) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	zw := zip.NewWriter(f)
	defer func() {
		if cerr := zw.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()

	pages := comicPages(kind)
	manga := "No"
	if kind == comicManga {
		manga = "YesAndRightToLeft"
	}
	info, err := zw.Create("ComicInfo.xml")
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := fmt.Fprintf(info, `<?xml version="1.0" encoding="utf-8"?>
<ComicInfo>
  <Title>%s</Title>
  <Penciller>%s</Penciller>
  <PageCount>%d</PageCount>
  <Manga>%s</Manga>
</ComicInfo>
`, title, author, len(pages), manga); err != nil {
		return errors.WithStack(err)
	}
	for i, page := range pages {
		data, err := pngBytes(numberedPage(page, i+1))
		if err != nil {
			return err
		}
		// Stored, as most CBZs' images are; the reader reads deflated ones
		// too, which its unit tests cover.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("page%03d.png", i+1), Method: zip.Store})
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := w.Write(data); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// numberedPage draws a page of page's color with number in white
// seven-segment digits across its middle, and a white border, so a page's
// edges and number show.
func numberedPage(page comicPage, number int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, page.width, page.height))
	white := image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	draw.Draw(img, img.Bounds(), white, image.Point{}, draw.Src)
	inner := img.Bounds().Inset(12)
	draw.Draw(img, inner, image.NewUniform(page.background), image.Point{}, draw.Src)

	digits := strconv.Itoa(number)
	digitWidth := min(page.width/4, 160)
	digitHeight := digitWidth * 2
	gap := digitWidth / 3
	total := len(digits)*digitWidth + (len(digits)-1)*gap
	x := (page.width - total) / 2
	y := (page.height - digitHeight) / 2
	for _, digit := range digits {
		drawDigit(img, int(digit-'0'), image.Rect(x, y, x+digitWidth, y+digitHeight), white)
		x += digitWidth + gap
	}
	return img
}

// segments are which of a seven-segment digit's segments are lit, in the
// order top, top right, bottom right, bottom, bottom left, top left, middle.
var segments = [10][7]bool{
	{true, true, true, true, true, true, false},
	{false, true, true, false, false, false, false},
	{true, true, false, true, true, false, true},
	{true, true, true, true, false, false, true},
	{false, true, true, false, false, true, true},
	{true, false, true, true, false, true, true},
	{true, false, true, true, true, true, true},
	{true, true, true, false, false, false, false},
	{true, true, true, true, true, true, true},
	{true, true, true, true, false, true, true},
}

func drawDigit(img *image.RGBA, digit int, box image.Rectangle, ink image.Image) {
	t := box.Dx() / 5
	midY := box.Min.Y + box.Dy()/2
	bars := [7]image.Rectangle{
		image.Rect(box.Min.X, box.Min.Y, box.Max.X, box.Min.Y+t),
		image.Rect(box.Max.X-t, box.Min.Y, box.Max.X, midY),
		image.Rect(box.Max.X-t, midY, box.Max.X, box.Max.Y),
		image.Rect(box.Min.X, box.Max.Y-t, box.Max.X, box.Max.Y),
		image.Rect(box.Min.X, midY, box.Min.X+t, box.Max.Y),
		image.Rect(box.Min.X, box.Min.Y, box.Min.X+t, midY),
		image.Rect(box.Min.X, midY-t/2, box.Max.X, midY+t/2),
	}
	for i, lit := range segments[digit] {
		if lit {
			draw.Draw(img, bars[i], ink, image.Point{}, draw.Src)
		}
	}
}
