package worker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a shared Genre, Tag, Series, or Person stamps manual on the
// collection source of every affected Book or File (ADR 0006). The sidecar
// every Scan writes still names the deleted resource, and sidecar outranks
// plugin and embedded sources, so without the stamp the next ordinary Scan
// re-creates and reattaches it. Refresh all metadata and Reset to file
// metadata skip sidecars and may repopulate the collection under their
// usual rules.

const collectionDeleteParserManifest = `{
  "manifestVersion": 1,
  "id": "coltest-parser",
  "name": "Collection Test Parser",
  "version": "1.0.0",
  "capabilities": {"fileParser": {"description": "Parses coltest files", "types": ["coltest"]}}
}`

// The parser stands in for file metadata that names two members of every
// collection, including Tags, which no native test generator emits.
const collectionDeleteParserJS = `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Collection Title",
          authors: [{ name: "Parsed Author" }, { name: "Second Author" }],
          narrators: ["Parsed Narrator", "Second Narrator"],
          genres: ["Parsed Genre", "Second Genre"],
          tags: ["Parsed Tag", "Second Tag"],
          series: "Parsed Series",
          seriesNumber: 1
        };
      }
    }
  };
})();`

const collectionDeleteParserSource = "plugin:test/coltest-parser"

// collectionKind abstracts one collection so every scenario runs against
// Genres, Tags, Series, Authors, and Narrators alike.
type collectionKind struct {
	name string
	// deleteByName deletes the library's resource with this name through its
	// service, the path the delete handler uses.
	deleteByName func(t *testing.T, tc *testContext, libraryID int, name string)
	exists       func(tc *testContext, libraryID int, name string) bool
	names        func(book *models.Book, file *models.File) []string
	source       func(book *models.Book, file *models.File) *string
}

var collectionKinds = []collectionKind{
	{
		name: "genre",
		deleteByName: func(t *testing.T, tc *testContext, libraryID int, name string) {
			t.Helper()
			svc := genres.NewService(tc.db)
			g, err := svc.RetrieveGenre(tc.ctx, genres.RetrieveGenreOptions{Name: &name, LibraryID: &libraryID})
			require.NoError(t, err)
			require.NoError(t, svc.DeleteGenre(tc.ctx, g.ID))
		},
		exists: func(tc *testContext, libraryID int, name string) bool {
			_, err := genres.NewService(tc.db).RetrieveGenre(tc.ctx, genres.RetrieveGenreOptions{Name: &name, LibraryID: &libraryID})
			return err == nil
		},
		names: func(book *models.Book, _ *models.File) []string {
			var out []string
			for _, bg := range book.BookGenres {
				out = append(out, bg.Genre.Name)
			}
			sort.Strings(out) // an unordered set
			return out
		},
		source: func(book *models.Book, _ *models.File) *string { return book.GenreSource },
	},
	{
		name: "tag",
		deleteByName: func(t *testing.T, tc *testContext, libraryID int, name string) {
			t.Helper()
			svc := tags.NewService(tc.db)
			tag, err := svc.RetrieveTag(tc.ctx, tags.RetrieveTagOptions{Name: &name, LibraryID: &libraryID})
			require.NoError(t, err)
			require.NoError(t, svc.DeleteTag(tc.ctx, tag.ID))
		},
		exists: func(tc *testContext, libraryID int, name string) bool {
			_, err := tags.NewService(tc.db).RetrieveTag(tc.ctx, tags.RetrieveTagOptions{Name: &name, LibraryID: &libraryID})
			return err == nil
		},
		names: func(book *models.Book, _ *models.File) []string {
			var out []string
			for _, bt := range book.BookTags {
				out = append(out, bt.Tag.Name)
			}
			sort.Strings(out) // an unordered set
			return out
		},
		source: func(book *models.Book, _ *models.File) *string { return book.TagSource },
	},
	{
		name: "series",
		deleteByName: func(t *testing.T, tc *testContext, libraryID int, name string) {
			t.Helper()
			svc := series.NewService(tc.db)
			s, err := svc.RetrieveSeries(tc.ctx, series.RetrieveSeriesOptions{Name: &name, LibraryID: &libraryID})
			require.NoError(t, err)
			_, err = svc.DeleteSeries(tc.ctx, s.ID)
			require.NoError(t, err)
		},
		exists: func(tc *testContext, libraryID int, name string) bool {
			_, err := series.NewService(tc.db).RetrieveSeries(tc.ctx, series.RetrieveSeriesOptions{Name: &name, LibraryID: &libraryID})
			return err == nil
		},
		names: func(book *models.Book, _ *models.File) []string {
			var out []string
			for _, bs := range book.BookSeries {
				out = append(out, bs.Series.Name)
			}
			return out
		},
		source: func(book *models.Book, _ *models.File) *string { return book.SeriesSource },
	},
	{
		name:         "author",
		deleteByName: deletePersonByName,
		exists:       personExists,
		names: func(book *models.Book, _ *models.File) []string {
			var out []string
			for _, a := range book.Authors {
				out = append(out, a.Person.Name)
			}
			return out
		},
		source: func(book *models.Book, _ *models.File) *string {
			if book.AuthorSource == "" {
				return nil
			}
			s := book.AuthorSource
			return &s
		},
	},
	{
		name:         "narrator",
		deleteByName: deletePersonByName,
		exists:       personExists,
		names: func(_ *models.Book, file *models.File) []string {
			var out []string
			for _, n := range file.Narrators {
				out = append(out, n.Person.Name)
			}
			return out
		},
		source: func(_ *models.Book, file *models.File) *string { return file.NarratorSource },
	},
}

func deletePersonByName(t *testing.T, tc *testContext, libraryID int, name string) {
	t.Helper()
	svc := people.NewService(tc.db)
	p, err := svc.RetrievePerson(tc.ctx, people.RetrievePersonOptions{Name: &name, LibraryID: &libraryID})
	require.NoError(t, err)
	require.NoError(t, svc.DeletePerson(tc.ctx, p.ID))
}

func personExists(tc *testContext, libraryID int, name string) bool {
	_, err := people.NewService(tc.db).RetrievePerson(tc.ctx, people.RetrievePersonOptions{Name: &name, LibraryID: &libraryID})
	return err == nil
}

// newCollectionDeleteFixture scans a one-file library whose parser names two
// members of every collection. The Scan writes the book and file sidecars.
func newCollectionDeleteFixture(t *testing.T) *identifyScanFixture {
	t.Helper()

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "coltest-parser", collectionDeleteParserManifest, collectionDeleteParserJS)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Collection Title")
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.coltest"), []byte("content"), 0644))

	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

// requireSidecarsName checks the precondition that makes a plain delete
// regress: the sidecars the Scan wrote still name the resource.
func requireSidecarsName(t *testing.T, book *models.Book, file *models.File, name string) {
	t.Helper()
	var contents strings.Builder
	for _, path := range []string{sidecar.BookSidecarPath(book.Filepath), sidecar.FileSidecarPath(file.Filepath)} {
		data, err := os.ReadFile(path)
		require.NoError(t, err, "precondition: the Scan wrote %s", path)
		contents.Write(data)
	}
	require.Contains(t, contents.String(), name, "precondition: a sidecar still names the deleted resource")
}

func requireManualSource(t *testing.T, kind collectionKind, book *models.Book, file *models.File) {
	t.Helper()
	source := kind.source(book, file)
	require.NotNil(t, source, "%s source", kind.name)
	require.Equal(t, models.DataSourceManual, *source, "%s source", kind.name)
}

// Deleting one member of each collection, with the sidecars left in place,
// then running an ordinary Scan keeps the deletion and the other member.
// Series come from the parser as a single membership, so that delete
// empties the collection.
func TestDeleteCollectionMember_ThenOrdinaryScan_SidecarPresent(t *testing.T) {
	t.Parallel()

	for _, kind := range collectionKinds {
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()

			f := newCollectionDeleteFixture(t)
			book, file := f.retrieve(t)
			deletedName, remaining := collectionDeleteTarget(t, kind, book, file)
			requireSidecarsName(t, book, file, deletedName)

			kind.deleteByName(t, f.tc, book.LibraryID, deletedName)
			book, file = f.retrieve(t)
			requireManualSource(t, kind, book, file)

			f.ordinaryScan(t, file.ID)

			book, file = f.retrieve(t)
			assert.Equal(t, remaining, kind.names(book, file), "the ordinary Scan keeps the deletion")
			requireManualSource(t, kind, book, file)
			assert.False(t, kind.exists(f.tc, book.LibraryID, deletedName), "the deleted %s %q must not be re-created", kind.name, deletedName)
		})
	}
}

// collectionDeleteTarget returns the parser's first member of the
// collection, which the test deletes, and the members expected to remain.
func collectionDeleteTarget(t *testing.T, kind collectionKind, book *models.Book, file *models.File) (string, []string) {
	t.Helper()
	names := kind.names(book, file)
	source := kind.source(book, file)
	require.NotNil(t, source)
	require.Equal(t, collectionDeleteParserSource, *source, "precondition: the parser wins the first Scan")
	if kind.name == "series" {
		require.Equal(t, []string{"Parsed Series"}, names)
		return "Parsed Series", nil
	}
	require.Len(t, names, 2)
	return names[0], names[1:]
}

func TestDeleteCollectionMember_ThenRefreshAllMetadata_Repopulates(t *testing.T) {
	t.Parallel()

	for _, kind := range collectionKinds {
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()

			f := newCollectionDeleteFixture(t)
			book, file := f.retrieve(t)
			before := kind.names(book, file)
			deletedName, _ := collectionDeleteTarget(t, kind, book, file)
			kind.deleteByName(t, f.tc, book.LibraryID, deletedName)

			_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true}, nil)
			require.NoError(t, err)

			book, file = f.retrieve(t)
			assert.Equal(t, before, kind.names(book, file), "Refresh all metadata may repopulate the collection")
			source := kind.source(book, file)
			require.NotNil(t, source)
			assert.Equal(t, collectionDeleteParserSource, *source)
		})
	}
}

func TestDeleteCollectionMember_ThenResetToFileMetadata_Repopulates(t *testing.T) {
	t.Parallel()

	for _, kind := range collectionKinds {
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()

			f := newCollectionDeleteFixture(t)
			book, file := f.retrieve(t)
			before := kind.names(book, file)
			deletedName, _ := collectionDeleteTarget(t, kind, book, file)
			kind.deleteByName(t, f.tc, book.LibraryID, deletedName)

			_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true, SkipPlugins: true, Reset: true}, nil)
			require.NoError(t, err)

			book, file = f.retrieve(t)
			assert.Equal(t, before, kind.names(book, file), "Reset to file metadata may repopulate the collection")
			source := kind.source(book, file)
			require.NotNil(t, source)
			assert.Equal(t, collectionDeleteParserSource, *source)
		})
	}
}

// The same guarantee for collections read from a native file's embedded
// metadata, where the sidecar and the file both name the deleted resource.
// No native test generator emits Tags, so the parser fixture covers them.
func TestDeleteCollectionMember_ThenOrdinaryScan_EmbeddedM4B(t *testing.T) {
	t.Parallel()

	embeddedNames := map[string]string{
		"genre":    "Embedded Genre",
		"series":   "Embedded Series",
		"author":   "Embedded Author",
		"narrator": "Embedded Narrator",
	}
	for _, kind := range collectionKinds {
		deletedName, ok := embeddedNames[kind.name]
		if !ok {
			continue
		}
		t.Run(kind.name, func(t *testing.T) {
			t.Parallel()

			tc, bookDir := newIdentifyScalarClearContext(t)
			testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
				Title:    "Embedded Title",
				Artist:   "Embedded Author",
				Composer: "Embedded Narrator",
				Genre:    "Embedded Genre",
				Grouping: "Embedded Series #1",
			})
			require.NoError(t, tc.runScan())
			f := &identifyScanFixture{tc: tc}

			book, file := f.retrieve(t)
			require.Equal(t, []string{deletedName}, kind.names(book, file), "precondition: the Scan reads embedded metadata")
			source := kind.source(book, file)
			require.NotNil(t, source)
			require.Equal(t, models.DataSourceM4BMetadata, *source)
			requireSidecarsName(t, book, file, deletedName)

			kind.deleteByName(t, tc, book.LibraryID, deletedName)
			f.ordinaryScan(t, file.ID)

			book, file = f.retrieve(t)
			assert.Empty(t, kind.names(book, file), "the ordinary Scan keeps the deletion")
			requireManualSource(t, kind, book, file)
			assert.False(t, kind.exists(tc, book.LibraryID, deletedName), "the deleted %s %q must not be re-created", kind.name, deletedName)
		})
	}
}
