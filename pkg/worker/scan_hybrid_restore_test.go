package worker

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hybrid Book whose shared authors and audiobook narrators were never
// filled gets them from embedded metadata on an ordinary Scan, and the Scan
// organizes the folder and the M4B filename to match.
func TestScan_HybridOrganizesRestoredNarrators(t *testing.T) {
	t.Parallel()
	f := newIdentifyRelationshipScanFixture(t, false, true)
	initial, audio := f.retrieve(t)
	libraryPath := filepath.Dir(audio.Filepath)
	testgen.GenerateEPUB(t, libraryPath, "book.epub", testgen.EPUBOptions{
		Title: "Embedded Title", Authors: []string{"Embedded Author One", "Embedded Author Two"},
	})
	require.NoError(t, f.tc.runScan())
	retrieve := func() (*models.Book, *models.File, *models.File) {
		t.Helper()
		require.Len(t, f.tc.listBooks(), 1)
		book, err := f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &initial.ID})
		require.NoError(t, err)
		require.Len(t, book.Files, 2)
		var ebook, audiobook *models.File
		for _, file := range book.Files {
			switch file.FileType {
			case models.FileTypeEPUB:
				ebook = file
			case models.FileTypeM4B:
				audiobook = file
			}
		}
		require.NotNil(t, ebook)
		require.NotNil(t, audiobook)
		return book, ebook, audiobook
	}
	book, _, audio := retrieve()
	_, err := f.tc.db.NewUpdate().Model((*models.Library)(nil)).
		Set("organize_file_structure = ?", true).Where("id = ?", book.LibraryID).Exec(f.tc.ctx)
	require.NoError(t, err)
	// Put authors and narrators in the state of fields that were never filled,
	// or of an install migrated from before Identify clears left a protected
	// empty slot: removed with NULL sources, with the organized folder and
	// sidecars to match. A Scan may fill them from embedded metadata.
	require.NoError(t, f.tc.bookService.DeleteAuthors(f.tc.ctx, book.ID))
	_, err = f.tc.db.NewRaw("UPDATE books SET author_source = NULL WHERE id = ?", book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.bookService.DeleteNarratorsForFile(f.tc.ctx, audio.ID)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("UPDATE files SET narrator_source = NULL WHERE id = ?", audio.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	syncDiskToStoredMetadata(t, f.tc, book.ID)
	book, ebook, audio := retrieve()
	require.Empty(t, book.Authors)
	require.Empty(t, book.AuthorSource)
	require.Empty(t, audio.Narrators)
	require.Nil(t, audio.NarratorSource)
	require.Equal(t, filepath.Join(libraryPath, "Embedded Title", "Embedded Title.m4b"), audio.Filepath)
	// Restore shared authors first. The M4B scan then changes only narrators,
	// so it cannot rely on an author/title change to organize the filename.
	f.ordinaryScan(t, ebook.ID)
	f.ordinaryScan(t, audio.ID)
	book, ebook, audio = retrieve()
	require.Len(t, book.Authors, 2)
	assert.Equal(t, "Embedded Author One", book.Authors[0].Person.Name)
	assert.Equal(t, models.DataSourceEPUBMetadata, book.AuthorSource)
	require.Len(t, audio.Narrators, 2)
	assert.Equal(t, "Embedded Narrator One", audio.Narrators[0].Person.Name)
	assert.Equal(t, models.DataSourceM4BMetadata, *audio.NarratorSource)
	assert.Empty(t, ebook.Narrators)
	assert.Nil(t, ebook.NarratorSource)
	expectedFolder := filepath.Join(libraryPath, "[Embedded Author One] Embedded Title")
	assert.Equal(t, expectedFolder, book.Filepath)
	assert.Equal(t, filepath.Join(expectedFolder, "Embedded Title.epub"), ebook.Filepath)
	assert.Equal(t, filepath.Join(expectedFolder, "Embedded Title {Embedded Narrator One}.m4b"), audio.Filepath)
	require.FileExists(t, ebook.Filepath)
	require.FileExists(t, audio.Filepath)
	bookSidecar, err := sidecar.ReadBookSidecarFromModel(book, audio)
	require.NoError(t, err)
	require.NotNil(t, bookSidecar)
	require.Len(t, bookSidecar.Authors, 2)
	assert.Equal(t, "Embedded Author One", bookSidecar.Authors[0].Name)
	audioSidecar, err := sidecar.ReadFileSidecar(audio.Filepath)
	require.NoError(t, err)
	require.NotNil(t, audioSidecar)
	require.Len(t, audioSidecar.Narrators, 2)
	assert.Equal(t, "Embedded Narrator One", audioSidecar.Narrators[0].Name)
	ebookSidecar, err := sidecar.ReadFileSidecar(ebook.Filepath)
	require.NoError(t, err)
	require.NotNil(t, ebookSidecar)
	assert.Empty(t, ebookSidecar.Narrators)
}
