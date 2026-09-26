package people

import (
	"context"
	"fmt"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

const personDeletePluginSource = "plugin:test/enricher"

func createPersonDeleteLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

func createNamedPerson(t *testing.T, svc *Service, lib *models.Library, name string) *models.Person {
	t.Helper()
	p := &models.Person{LibraryID: lib.ID, Name: name}
	require.NoError(t, svc.CreatePerson(context.Background(), p))
	return p
}

// createAuthoredBook inserts a Book with the given author_source whose
// Authors are the listed People, in order, and returns its ID.
func createAuthoredBook(t *testing.T, db *bun.DB, lib *models.Library, source string, personIDs ...int) int {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    source,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for i, personID := range personIDs {
		_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: personID, SortOrder: i + 1}).Exec(ctx)
		require.NoError(t, err)
	}
	return book.ID
}

// createNarratedFile inserts a File with the given narrator_source whose
// Narrators are the listed People, in order, and returns its ID. Its Book
// has no Authors and a plugin author_source, so tests can check that a
// Narrator delete leaves author_source alone.
func createNarratedFile(t *testing.T, db *bun.DB, lib *models.Library, source *string, personIDs ...int) int {
	t.Helper()
	ctx := context.Background()

	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource)
	file := &models.File{
		LibraryID:      lib.ID,
		BookID:         bookID,
		FileType:       models.FileTypeM4B,
		FileRole:       models.FileRoleMain,
		Filepath:       fmt.Sprintf("/tmp/narrated-%d.m4b", bookID),
		FilesizeBytes:  1,
		NarratorSource: source,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	for i, personID := range personIDs {
		_, err = db.NewInsert().Model(&models.Narrator{FileID: file.ID, PersonID: personID, SortOrder: i + 1}).Exec(ctx)
		require.NoError(t, err)
	}
	return file.ID
}

func retrieveAuthoredBook(t *testing.T, db *bun.DB, bookID int) (*models.Book, []int) {
	t.Helper()
	ctx := context.Background()
	book := &models.Book{}
	require.NoError(t, db.NewSelect().Model(book).Where("b.id = ?", bookID).Scan(ctx))
	var personIDs []int
	require.NoError(t, db.NewSelect().Model((*models.Author)(nil)).Column("person_id").Where("book_id = ?", bookID).Order("sort_order").Scan(ctx, &personIDs))
	return book, personIDs
}

func retrieveNarratedFile(t *testing.T, db *bun.DB, fileID int) (*models.File, []int) {
	t.Helper()
	ctx := context.Background()
	file := &models.File{}
	require.NoError(t, db.NewSelect().Model(file).Where("f.id = ?", fileID).Scan(ctx))
	var personIDs []int
	require.NoError(t, db.NewSelect().Model((*models.Narrator)(nil)).Column("person_id").Where("file_id = ?", fileID).Order("sort_order").Scan(ctx, &personIDs))
	return file, personIDs
}

// Deleting a shared Person is a deliberate user action, so every Book the
// Person authored gets a manual author_source and every File the Person
// narrated gets a manual narrator_source, whatever the prior source and even
// when other People remain. An ordinary Scan then leaves those collections
// alone instead of re-creating the Person from a sidecar or the file
// (ADR 0006).
func TestDeletePerson_StampsManualSourceOnEveryAffectedBookAndFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	deleted := createNamedPerson(t, svc, lib, "Deleted Person")
	kept := createNamedPerson(t, svc, lib, "Kept Person")

	priorSources := []*string{
		testgen.StringPtr(models.DataSourceManual),
		testgen.StringPtr(models.DataSourceSidecar),
		testgen.StringPtr(personDeletePluginSource),
		testgen.StringPtr(models.DataSourcePlugin),
		testgen.StringPtr(models.DataSourceM4BMetadata),
		testgen.StringPtr(models.DataSourceFilepath),
		nil,
	}
	var emptiedBooks, emptiedFiles []int
	for _, source := range priorSources {
		authorSource := ""
		if source != nil {
			authorSource = *source
		}
		emptiedBooks = append(emptiedBooks, createAuthoredBook(t, db, lib, authorSource, deleted.ID))
		emptiedFiles = append(emptiedFiles, createNarratedFile(t, db, lib, source, deleted.ID))
	}
	partialBook := createAuthoredBook(t, db, lib, personDeletePluginSource, deleted.ID, kept.ID)
	partialFile := createNarratedFile(t, db, lib, testgen.StringPtr(personDeletePluginSource), kept.ID, deleted.ID)
	untouchedBook := createAuthoredBook(t, db, lib, personDeletePluginSource, kept.ID)
	untouchedFile := createNarratedFile(t, db, lib, testgen.StringPtr(personDeletePluginSource), kept.ID)

	require.NoError(t, svc.DeletePerson(ctx, deleted.ID))

	for i, bookID := range emptiedBooks {
		book, personIDs := retrieveAuthoredBook(t, db, bookID)
		assert.Empty(t, personIDs, "prior source %d: the Author row is removed", i)
		assert.Equal(t, models.DataSourceManual, book.AuthorSource, "prior source %d: author_source is stamped", i)
	}
	for i, fileID := range emptiedFiles {
		file, personIDs := retrieveNarratedFile(t, db, fileID)
		assert.Empty(t, personIDs, "prior source %d: the Narrator row is removed", i)
		if assert.NotNil(t, file.NarratorSource, "prior source %d: narrator_source is stamped", i) {
			assert.Equal(t, models.DataSourceManual, *file.NarratorSource, "prior source %d", i)
		}

		// The File's Book never had the Person as an Author.
		book, _ := retrieveAuthoredBook(t, db, file.BookID)
		assert.Equal(t, personDeletePluginSource, book.AuthorSource, "prior source %d: a Narrator delete leaves author_source alone", i)
	}

	book, personIDs := retrieveAuthoredBook(t, db, partialBook)
	assert.Equal(t, []int{kept.ID}, personIDs, "the remaining Author is kept")
	assert.Equal(t, models.DataSourceManual, book.AuthorSource, "a partially changed Author collection is stamped too")

	file, personIDs := retrieveNarratedFile(t, db, partialFile)
	assert.Equal(t, []int{kept.ID}, personIDs, "the remaining Narrator is kept")
	if assert.NotNil(t, file.NarratorSource) {
		assert.Equal(t, models.DataSourceManual, *file.NarratorSource, "a partially changed Narrator collection is stamped too")
	}

	book, personIDs = retrieveAuthoredBook(t, db, untouchedBook)
	assert.Equal(t, []int{kept.ID}, personIDs)
	assert.Equal(t, personDeletePluginSource, book.AuthorSource, "Books the Person did not author keep their source")

	file, personIDs = retrieveNarratedFile(t, db, untouchedFile)
	assert.Equal(t, []int{kept.ID}, personIDs)
	assert.Equal(t, testgen.StringPtr(personDeletePluginSource), file.NarratorSource, "Files the Person did not narrate keep their source")

	_, err := svc.RetrievePerson(ctx, RetrievePersonOptions{ID: &deleted.ID})
	require.Error(t, err, "the Person itself is deleted")
}

// A Person who authored and narrated nothing deletes cleanly and touches no
// Book or File.
func TestDeletePerson_Unused_TouchesNothing(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	unused := createNamedPerson(t, svc, lib, "Unused Person")
	other := createNamedPerson(t, svc, lib, "Other Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource, other.ID)
	fileID := createNarratedFile(t, db, lib, testgen.StringPtr(personDeletePluginSource), other.ID)

	require.NoError(t, svc.DeletePerson(ctx, unused.ID))

	book, personIDs := retrieveAuthoredBook(t, db, bookID)
	assert.Equal(t, []int{other.ID}, personIDs)
	assert.Equal(t, personDeletePluginSource, book.AuthorSource)

	file, personIDs := retrieveNarratedFile(t, db, fileID)
	assert.Equal(t, []int{other.ID}, personIDs)
	assert.Equal(t, testgen.StringPtr(personDeletePluginSource), file.NarratorSource)
}

// Merging re-points Authors and Narrators at the target. It is not a clear,
// so Books and Files keep their existing sources.
func TestMergePeople_KeepsBookAndFileSources(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createPersonDeleteLibrary(t, db)
	source := createNamedPerson(t, svc, lib, "Source Person")
	target := createNamedPerson(t, svc, lib, "Target Person")
	bookID := createAuthoredBook(t, db, lib, personDeletePluginSource, source.ID)
	fileID := createNarratedFile(t, db, lib, testgen.StringPtr(models.DataSourceM4BMetadata), source.ID)

	require.NoError(t, svc.MergePeople(ctx, target.ID, source.ID))

	book, personIDs := retrieveAuthoredBook(t, db, bookID)
	assert.Equal(t, []int{target.ID}, personIDs)
	assert.Equal(t, personDeletePluginSource, book.AuthorSource)

	file, personIDs := retrieveNarratedFile(t, db, fileID)
	assert.Equal(t, []int{target.ID}, personIDs)
	assert.Equal(t, testgen.StringPtr(models.DataSourceM4BMetadata), file.NarratorSource)
}
