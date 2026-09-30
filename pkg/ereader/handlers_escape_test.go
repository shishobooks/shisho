package ereader

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hostileTitle       = "<script>alert(1)</script>"
	hostileAuthor      = "<b>x</b>"
	hostileDescription = "<img src=x onerror=alert(1)>"
	hostileSeries      = "<u>series</u>"
	hostileFileName    = "<i>file</i>"
)

type hostileBookFixture struct {
	*eReaderAccessFixture
	book   *models.Book
	person *models.Person
	series *models.Series
}

// newHostileMetadataFixture stores a book in libA whose title, author, series,
// and file name are all HTML, as a crafted EPUB could supply, with the given
// description. The book has no cover, so any <img> on its pages came from
// metadata.
func newHostileMetadataFixture(t *testing.T, description string) *hostileBookFixture {
	t.Helper()
	f := &hostileBookFixture{eReaderAccessFixture: newEReaderAccessFixture(t)}
	ctx := context.Background()
	now := time.Now()
	bookDir := t.TempDir()

	f.book = &models.Book{
		LibraryID:       f.libA.ID,
		Title:           hostileTitle,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       hostileTitle,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Description:     &description,
		Filepath:        bookDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := f.db.NewInsert().Model(f.book).Exec(ctx)
	require.NoError(t, err)

	f.person = &models.Person{
		LibraryID:      f.libA.ID,
		Name:           hostileAuthor,
		SortName:       hostileAuthor,
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = f.db.NewInsert().Model(f.person).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.Author{BookID: f.book.ID, PersonID: f.person.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	f.series = &models.Series{
		LibraryID:      f.libA.ID,
		Name:           hostileSeries,
		NameSource:     models.DataSourceManual,
		SortName:       hostileSeries,
		SortNameSource: models.DataSourceManual,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = f.db.NewInsert().Model(f.series).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: f.book.ID, SeriesID: f.series.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	// One unnamed file, whose download entry falls back to the book title,
	// and one named file.
	fileName := hostileFileName
	for i, name := range []*string{nil, &fileName} {
		file := &models.File{
			LibraryID:     f.libA.ID,
			BookID:        f.book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      filepath.Join(bookDir, fmt.Sprintf("book-%d.epub", i)),
			FilesizeBytes: 1024,
			Name:          name,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		_, err = f.db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}
	return f
}

func (f *hostileBookFixture) page(t *testing.T, path string) string {
	t.Helper()
	rec := f.serve(path)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}

// Metadata from a scanned file is written into the eReader pages as text,
// never as markup.
func TestEReaderPages_EscapeMetadata(t *testing.T) {
	t.Parallel()
	f := newHostileMetadataFixture(t, hostileDescription)

	pages := map[string]string{
		"book":   fmt.Sprintf("/download/%d", f.book.ID),
		"series": fmt.Sprintf("/libraries/%d/series/%d", f.libA.ID, f.series.ID),
		"author": fmt.Sprintf("/libraries/%d/authors/%d", f.libA.ID, f.person.ID),
	}
	bodies := make(map[string]string, len(pages))
	for name, path := range pages {
		body := f.page(t, path)
		bodies[name] = body
		assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;", "%s page shows the title as text", name)
		assert.NotContains(t, body, "<script", "%s page has no script element", name)
		assert.NotContains(t, body, "<img", "%s page has no image, since the book has no cover", name)
		assert.NotContains(t, body, hostileAuthor, "%s page has no raw author markup", name)
		assert.NotContains(t, body, hostileSeries, "%s page has no raw series markup", name)
		assert.NotContains(t, body, hostileFileName, "%s page has no raw file name markup", name)
	}

	assert.Contains(t, bodies["series"], "<h1>&lt;u&gt;series&lt;/u&gt;</h1>")
	assert.Contains(t, bodies["author"], "<h1>&lt;b&gt;x&lt;/b&gt;</h1>")

	book := bodies["book"]
	assert.Contains(t, book, "<h1>&lt;script&gt;alert(1)&lt;/script&gt;</h1>")
	assert.Contains(t, book, "<p>By: &lt;b&gt;x&lt;/b&gt;</p>")
	assert.Contains(t, book, "<p>&lt;img src=x onerror=alert(1)&gt;</p>")
	// Download entries: the unnamed file falls back to the title, the named
	// file shows its own name.
	assert.Contains(t, book, `<div style="font-weight: bold;">&lt;script&gt;alert(1)&lt;/script&gt;</div>`)
	assert.Contains(t, book, `<div style="font-weight: bold;">&lt;i&gt;file&lt;/i&gt;</div>`)
}

// Stored descriptions keep their paragraph and line breaks on the book page.
func TestEReaderDownload_DescriptionKeepsParagraphs(t *testing.T) {
	t.Parallel()
	f := newHostileMetadataFixture(t, "First paragraph\n\nSecond paragraph\nsecond line & more")

	body := f.page(t, fmt.Sprintf("/download/%d", f.book.ID))
	assert.Contains(t, body, "<p>First paragraph</p><p>Second paragraph<br>second line &amp; more</p>")
}

// A description with no text renders no empty paragraph.
func TestEReaderDownload_BlankDescriptionRendersNothing(t *testing.T) {
	t.Parallel()
	f := newHostileMetadataFixture(t, " \n\n ")

	body := f.page(t, fmt.Sprintf("/download/%d", f.book.ID))
	// The description would render between the author line and the first
	// download entry; nothing may appear there.
	const byLine = "<p>By: &lt;b&gt;x&lt;/b&gt;</p>"
	const firstEntry = `<div style="padding: 12px 0;`
	start := strings.Index(body, byLine)
	require.GreaterOrEqual(t, start, 0, "page has the author line")
	start += len(byLine)
	end := strings.Index(body[start:], firstEntry)
	require.GreaterOrEqual(t, end, 0, "page has a download entry after the author line")
	assert.Empty(t, body[start:start+end], "no description element between the author line and the download entry")
}
