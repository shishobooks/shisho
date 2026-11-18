package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renaming or merging a resource rewrites the sidecars of the Books that hold
// it, through the real route table, so a later scan doesn't read the old name
// back (ADR 0027).

type sidecarBook struct {
	bookDir   string
	filePath  string
	author    *models.Person
	narrator  *models.Person
	genre     *models.Genre
	tag       *models.Tag
	series    *models.Series
	publisher *models.Publisher
}

func seedSidecarBook(f *resourceDeleteFixture) *sidecarBook {
	f.t.Helper()
	dir := f.t.TempDir()
	bookDir := filepath.Join(dir, "A Book")
	require.NoError(f.t, os.MkdirAll(bookDir, 0o755))
	filePath := filepath.Join(bookDir, "book.m4b")
	require.NoError(f.t, os.WriteFile(filePath, []byte("audio"), 0o644))

	b := &sidecarBook{bookDir: bookDir, filePath: filePath}
	b.author = f.person("Old Author")
	b.narrator = f.person("Old Narrator")
	b.genre = &models.Genre{LibraryID: f.lib.ID, Name: "Old Genre"}
	f.insert(b.genre)
	b.tag = &models.Tag{LibraryID: f.lib.ID, Name: "Old Tag"}
	f.insert(b.tag)
	b.series = &models.Series{LibraryID: f.lib.ID, Name: "Old Series", SortName: "Old Series", NameSource: models.DataSourceFilepath, SortNameSource: models.DataSourceFilepath}
	f.insert(b.series)
	b.publisher = &models.Publisher{LibraryID: f.lib.ID, Name: "Old Press"}
	f.insert(b.publisher)

	book := &models.Book{
		LibraryID: f.lib.ID, Filepath: bookDir, Title: "A Book", TitleSource: models.DataSourceFilepath,
		SortTitle: "A Book", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath,
	}
	f.insert(book)
	file := &models.File{
		LibraryID: f.lib.ID, BookID: book.ID, Filepath: filePath, FileType: models.FileTypeM4B,
		FileRole: models.FileRoleMain, FilesizeBytes: 5, PublisherID: &b.publisher.ID,
	}
	f.insert(file)
	f.insert(&models.Author{BookID: book.ID, PersonID: b.author.ID, SortOrder: 1})
	f.insert(&models.Narrator{FileID: file.ID, PersonID: b.narrator.ID, SortOrder: 1})
	f.insert(&models.BookGenre{BookID: book.ID, GenreID: b.genre.ID})
	f.insert(&models.BookTag{BookID: book.ID, TagID: b.tag.ID})
	f.insert(&models.BookSeries{BookID: book.ID, SeriesID: b.series.ID, SortOrder: 1})
	return b
}

func (b *sidecarBook) sidecars(t *testing.T) (*sidecar.BookSidecar, *sidecar.FileSidecar) {
	t.Helper()
	bookSidecar, err := sidecar.ReadBookSidecar(b.bookDir)
	require.NoError(t, err)
	require.NotNil(t, bookSidecar, "the book sidecar must be written")
	fileSidecar, err := sidecar.ReadFileSidecar(b.filePath)
	require.NoError(t, err)
	require.NotNil(t, fileSidecar, "the file sidecar must be written")
	return bookSidecar, fileSidecar
}

func TestRenameRewritesSidecars(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		path  func(b *sidecarBook) string
		check func(t *testing.T, bs *sidecar.BookSidecar, fs *sidecar.FileSidecar)
	}{
		{"author", func(b *sidecarBook) string { return fmt.Sprintf("/api/people/%d", b.author.ID) }, func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			require.Len(t, bs.Authors, 1)
			assert.Equal(t, "New Name", bs.Authors[0].Name)
		}},
		{"narrator", func(b *sidecarBook) string { return fmt.Sprintf("/api/people/%d", b.narrator.ID) }, func(t *testing.T, _ *sidecar.BookSidecar, fs *sidecar.FileSidecar) {
			require.Len(t, fs.Narrators, 1)
			assert.Equal(t, "New Name", fs.Narrators[0].Name)
		}},
		{"genre", func(b *sidecarBook) string { return fmt.Sprintf("/api/genres/%d", b.genre.ID) }, func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			assert.Equal(t, []string{"New Name"}, bs.Genres)
		}},
		{"tag", func(b *sidecarBook) string { return fmt.Sprintf("/api/tags/%d", b.tag.ID) }, func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			assert.Equal(t, []string{"New Name"}, bs.Tags)
		}},
		{"series", func(b *sidecarBook) string { return fmt.Sprintf("/api/series/%d", b.series.ID) }, func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			require.Len(t, bs.Series, 1)
			assert.Equal(t, "New Name", bs.Series[0].Name)
		}},
		{"publisher", func(b *sidecarBook) string { return fmt.Sprintf("/api/publishers/%d", b.publisher.ID) }, func(t *testing.T, _ *sidecar.BookSidecar, fs *sidecar.FileSidecar) {
			require.NotNil(t, fs.Publisher)
			assert.Equal(t, "New Name", *fs.Publisher)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			b := seedSidecarBook(f)

			f.request(http.MethodPatch, tt.path(b), `{"name": "New Name"}`, http.StatusOK)

			bs, fs := b.sidecars(t)
			tt.check(t, bs, fs)
		})
	}
}

func TestMergeRewritesSidecars(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		target func(f *resourceDeleteFixture) int
		source func(b *sidecarBook) int
		path   string
		check  func(t *testing.T, bs *sidecar.BookSidecar, fs *sidecar.FileSidecar)
	}{
		{"person", func(f *resourceDeleteFixture) int { return f.person("Target").ID }, func(b *sidecarBook) int { return b.author.ID }, "people", func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			require.Len(t, bs.Authors, 1)
			assert.Equal(t, "Target", bs.Authors[0].Name)
		}},
		{"genre", func(f *resourceDeleteFixture) int {
			g := &models.Genre{LibraryID: f.lib.ID, Name: "Target"}
			f.insert(g)
			return g.ID
		}, func(b *sidecarBook) int { return b.genre.ID }, "genres", func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			assert.Equal(t, []string{"Target"}, bs.Genres)
		}},
		{"tag", func(f *resourceDeleteFixture) int {
			tg := &models.Tag{LibraryID: f.lib.ID, Name: "Target"}
			f.insert(tg)
			return tg.ID
		}, func(b *sidecarBook) int { return b.tag.ID }, "tags", func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			assert.Equal(t, []string{"Target"}, bs.Tags)
		}},
		{"series", func(f *resourceDeleteFixture) int {
			s := &models.Series{LibraryID: f.lib.ID, Name: "Target", SortName: "Target", NameSource: models.DataSourceFilepath, SortNameSource: models.DataSourceFilepath}
			f.insert(s)
			return s.ID
		}, func(b *sidecarBook) int { return b.series.ID }, "series", func(t *testing.T, bs *sidecar.BookSidecar, _ *sidecar.FileSidecar) {
			require.Len(t, bs.Series, 1)
			assert.Equal(t, "Target", bs.Series[0].Name)
		}},
		{"publisher", func(f *resourceDeleteFixture) int {
			p := &models.Publisher{LibraryID: f.lib.ID, Name: "Target"}
			f.insert(p)
			return p.ID
		}, func(b *sidecarBook) int { return b.publisher.ID }, "publishers", func(t *testing.T, _ *sidecar.BookSidecar, fs *sidecar.FileSidecar) {
			require.NotNil(t, fs.Publisher)
			assert.Equal(t, "Target", *fs.Publisher)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			b := seedSidecarBook(f)
			target := tt.target(f)

			f.request(http.MethodPost, fmt.Sprintf("/api/%s/%d/merge", tt.path, target), fmt.Sprintf(`{"source_id": %d}`, tt.source(b)), http.StatusNoContent)

			bs, fs := b.sidecars(t)
			tt.check(t, bs, fs)
		})
	}
}
