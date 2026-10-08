package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // decode the KePub's converted pages
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unpaddedCBZPages lists the page images of the archive built by
// writeUnpaddedCBZ in reading order. Page i is a PNG i+100 pixels wide, so a
// consumer's page i can be identified by decoding its width.
var unpaddedCBZPages = []string{
	"Chapter 1/page1.png",
	"Chapter 1/page2.png",
	"Chapter 1/page3.png",
	"Chapter 1/page4.png",
	"Chapter 1/page5.png",
	"Chapter 1/page6.png",
	"Chapter 1/page7.png",
	"Chapter 1/page8.png",
	"Chapter 1/page9.png",
	"Chapter 1/page10.png",
	"Chapter 2/page1.png",
	"Chapter 2/page2.png",
	"Chapter 10/page1.png",
}

// unpaddedCBZCoverPage is the ComicInfo FrontCover index: Chapter 1/page10.png.
const unpaddedCBZCoverPage = 9

// junkPageWidth marks the resource-fork entries, which are never pages.
const junkPageWidth = 999

func pageWidth(page int) int { return 100 + page }

// writeUnpaddedCBZ writes the pages in reverse so archive order cannot pass
// for reading order, plus macOS resource forks that must not count as pages.
func writeUnpaddedCBZ(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	zw := zip.NewWriter(f)

	write := func(name string, data []byte) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}

	write("ComicInfo.xml", []byte(fmt.Sprintf(
		`<?xml version="1.0"?><ComicInfo><Title>Unpadded</Title><Pages><Page Image="%d" Type="FrontCover"/></Pages></ComicInfo>`,
		unpaddedCBZCoverPage)))
	for i := len(unpaddedCBZPages) - 1; i >= 0; i-- {
		write(unpaddedCBZPages[i], makePNG(pageWidth(i), 150))
	}
	write("Chapter 1/._page1.png", makePNG(junkPageWidth, 150))
	write("__MACOSX/Chapter 1/._page1.png", makePNG(junkPageWidth, 150))
	require.NoError(t, zw.Close())
}

func decodedWidth(t *testing.T, data []byte) int {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	return cfg.Width
}

func readZipEntry(t *testing.T, zipPath, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			r, err := f.Open()
			require.NoError(t, err)
			defer r.Close()
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			return data
		}
	}
	require.Failf(t, "entry not found", "%s has no entry %s", zipPath, name)
	return nil
}

// TestCBZPageOrder_AllConsumersAgree pins one page order for every CBZ
// consumer: the parser (cover index, chapter detection), the reader's page
// cache, scan cover extraction, and KePub conversion all agree on page N for
// an archive with unpadded page names.
func TestCBZPageOrder_AllConsumersAgree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cbzPath := filepath.Join(dir, "unpadded.cbz")
	writeUnpaddedCBZ(t, cbzPath)
	pageCount := len(unpaddedCBZPages)

	// Parser: page count, cover index, and detected chapters.
	parsed, err := cbz.Parse(cbzPath)
	require.NoError(t, err)
	require.NotNil(t, parsed.PageCount)
	assert.Equal(t, pageCount, *parsed.PageCount, "resource forks must not count as pages")
	require.NotNil(t, parsed.CoverPage)
	assert.Equal(t, unpaddedCBZCoverPage, *parsed.CoverPage)
	assert.Equal(t, pageWidth(unpaddedCBZCoverPage), decodedWidth(t, parsed.CoverData),
		"the parsed cover must be the image at the cover index")

	type chapterStart struct {
		Title     string
		StartPage int
	}
	var gotChapters []chapterStart
	for _, ch := range parsed.Chapters {
		require.NotNil(t, ch.StartPage)
		gotChapters = append(gotChapters, chapterStart{ch.Title, *ch.StartPage})
	}
	assert.Equal(t, []chapterStart{
		{"Chapter 1", 0},
		{"Chapter 2", 10},
		{"Chapter 10", 12},
	}, gotChapters)

	// Reader page cache and scan cover extraction serve page i as the i-th
	// image in reading order.
	cache := cbzpages.NewCache(filepath.Join(dir, "pages"))
	coverDir := filepath.Join(dir, "covers")
	require.NoError(t, os.MkdirAll(coverDir, 0755))
	for i := range pageCount {
		cachedPath, _, err := cache.GetPage(cbzPath, 1, i)
		require.NoError(t, err)
		data, err := os.ReadFile(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, pageWidth(i), decodedWidth(t, data), "reader page %d", i)

		base := fmt.Sprintf("page%d.cover", i)
		filename, _, _, err := extractCBZPageCover(cbzPath, coverDir, base, i)
		require.NoError(t, err)
		data, err = os.ReadFile(filepath.Join(coverDir, filename))
		require.NoError(t, err)
		assert.Equal(t, pageWidth(i), decodedWidth(t, data), "scan cover from page %d", i)
	}
	_, _, err = cache.GetPage(cbzPath, 1, pageCount)
	require.ErrorIs(t, err, cbzpages.ErrPageOutOfRange)

	// KePub generated from the stored cover page and chapters.
	file := &models.File{CoverPage: parsed.CoverPage}
	for i, ch := range parsed.Chapters {
		file.Chapters = append(file.Chapters, &models.Chapter{Title: ch.Title, StartPage: ch.StartPage, SortOrder: i})
	}
	book := &models.Book{Title: "Unpadded"}
	kepubPath := filepath.Join(dir, "unpadded.kepub.epub")
	require.NoError(t, filegen.NewKepubCBZGenerator().Generate(context.Background(), cbzPath, kepubPath, book, file))

	kepubPageWidth := func(href string) int {
		xhtml := string(readZipEntry(t, kepubPath, "OEBPS/"+href))
		m := regexp.MustCompile(`<img [^>]*src="(images/[^"]+)"`).FindStringSubmatch(xhtml)
		require.NotNil(t, m, "%s has no page image", href)
		return decodedWidth(t, readZipEntry(t, kepubPath, "OEBPS/"+m[1]))
	}
	for i := range pageCount {
		assert.Equal(t, pageWidth(i), kepubPageWidth(fmt.Sprintf("page%04d.xhtml", i+1)), "KePub page %d", i)
	}

	opf := string(readZipEntry(t, kepubPath, "OEBPS/content.opf"))
	cover := regexp.MustCompile(`href="(images/[^"]+)"[^>]*properties="cover-image"`).FindStringSubmatch(opf)
	require.NotNil(t, cover, "the KePub OPF has no cover-image")
	assert.Equal(t, pageWidth(unpaddedCBZCoverPage), decodedWidth(t, readZipEntry(t, kepubPath, "OEBPS/"+cover[1])),
		"the KePub cover must be the parsed cover page")

	nav := string(readZipEntry(t, kepubPath, "OEBPS/nav.xhtml"))
	links := regexp.MustCompile(`<li><a href="(page\d{4}\.xhtml)">(Chapter \d+)</a></li>`).FindAllStringSubmatch(nav, -1)
	require.Len(t, links, len(gotChapters))
	for i, link := range links {
		assert.Equal(t, gotChapters[i].Title, link[2])
		assert.Equal(t, pageWidth(gotChapters[i].StartPage), kepubPageWidth(link[1]),
			"KePub link for %s must open its first page", link[2])
	}
}
