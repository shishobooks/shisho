package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// readThroughFixture is a user who read one Book, by an author, with a
// Reading Session, a Rating, and a place on To Read.
type readThroughFixture struct {
	db      *bun.DB
	svc     *Service
	library *models.Library
	book    *models.Book
	rt      *models.ReadThrough
}

func newReadThroughFixture(t *testing.T) *readThroughFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	user := &models.User{Username: "reader", PasswordHash: "x", RoleID: 2, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{LibraryID: library.ID, Name: "Ursula K. Le Guin", SortName: "Le Guin, Ursula K.", SortNameSource: models.DataSourceFilepath}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)
	path := filepath.Join(book.Filepath, "book.epub")
	require.NoError(t, os.WriteFile(path, []byte("epub"), 0o600))
	file := setupTestFile(t, db, book, models.FileTypeEPUB, path)

	now := time.Now()
	rt := &models.ReadThrough{CreatedAt: now, UpdatedAt: now, UserID: user.ID, BookID: &book.ID, EndDay: strPtr("2026-03-01"), Outcome: models.ReadThroughOutcomeFinished}
	_, err = db.NewInsert().Model(rt).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.ReadingSession{
		CreatedAt: now, UserID: user.ID, FileID: &file.ID, ReadThroughID: &rt.ID, Source: models.ReadingSourceWebReader,
		StartedAt: now.Add(-time.Hour), EndedAt: now, EndPercentage: 1, Measured: true,
		Kind: models.ReadingKindReading, Duration: 3600,
	}).Exec(ctx)
	require.NoError(t, err)
	rating, place := 16, 1
	_, err = db.NewInsert().Model(&models.UserBook{UserID: user.ID, BookID: book.ID, CreatedAt: now, UpdatedAt: now, Rating: &rating, ToReadPosition: &place}).Exec(ctx)
	require.NoError(t, err)

	return &readThroughFixture{db: db, svc: NewService(db, appsettings.NewService(db)), library: library, book: book, rt: rt}
}

// assertDetached checks the Read-through outlived its Book with a copy of
// its title and authors and kept its Session, while the per-Book record went.
func (f *readThroughFixture) assertDetached(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	rt := &models.ReadThrough{}
	require.NoError(t, f.db.NewSelect().Model(rt).Where("rt.id = ?", f.rt.ID).Scan(ctx))
	assert.Nil(t, rt.BookID)
	require.NotNil(t, rt.BookTitle)
	assert.Equal(t, "Test Book", *rt.BookTitle)
	assert.Equal(t, []string{"Ursula K. Le Guin"}, rt.BookAuthors)
	assert.Equal(t, "2026-03-01", *rt.EndDay)

	sessions, err := f.db.NewSelect().Model((*models.ReadingSession)(nil)).Where("read_through_id = ?", f.rt.ID).Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, sessions)
	userBooks, err := f.db.NewSelect().Model((*models.UserBook)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, userBooks, "the Rating and To Read place go with the Book")
}

func TestDeleteBookAndFiles_KeepsReadThroughs(t *testing.T) {
	t.Parallel()
	f := newReadThroughFixture(t)
	_, err := f.svc.DeleteBookAndFiles(context.Background(), f.book.ID, f.library)
	require.NoError(t, err)
	f.assertDetached(t)
}

func TestDeleteBooksAndFiles_KeepsReadThroughs(t *testing.T) {
	t.Parallel()
	f := newReadThroughFixture(t)
	_, err := f.svc.DeleteBooksAndFiles(context.Background(), []int{f.book.ID}, f.library)
	require.NoError(t, err)
	f.assertDetached(t)
}

func TestDeleteBooksByIDs_KeepsReadThroughs(t *testing.T) {
	t.Parallel()
	f := newReadThroughFixture(t)
	require.NoError(t, f.svc.DeleteBooksByIDs(context.Background(), []int{f.book.ID}))
	f.assertDetached(t)
}
