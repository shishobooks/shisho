package reach

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type fixture struct {
	t       *testing.T
	db      *bun.DB
	svc     *Service
	user    *models.User
	devices *devices.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	user := &models.User{Username: "reader", PasswordHash: "x", RoleID: 1, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID}).Exec(ctx)
	require.NoError(t, err)
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	return &fixture{t: t, db: db, svc: NewService(db, books.NewService(db, appsettings.NewService(db))), user: user, devices: devices.NewService(db)}
}

func (f *fixture) library(name string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(f.t, err)
	return lib
}

// book adds a book to lib with a main File of fileType, by author when one
// is named and in a series when one is named.
func (f *fixture) book(lib *models.Library, title, fileType, author, seriesName string) *models.Book {
	f.t.Helper()
	ctx := context.Background()
	b := &models.Book{LibraryID: lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title, SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/" + title}
	_, err := f.db.NewInsert().Model(b).Exec(ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.File{LibraryID: lib.ID, BookID: b.ID, FileType: fileType, FileRole: models.FileRoleMain, Filepath: b.Filepath + "/f." + fileType, FilesizeBytes: 1}).Exec(ctx)
	require.NoError(f.t, err)
	if author != "" {
		p := &models.Person{LibraryID: lib.ID, Name: author + title, SortName: author, SortNameSource: models.DataSourceFilepath}
		_, err = f.db.NewInsert().Model(p).Exec(ctx)
		require.NoError(f.t, err)
		_, err = f.db.NewInsert().Model(&models.Author{BookID: b.ID, PersonID: p.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(f.t, err)
	}
	if seriesName != "" {
		s := &models.Series{LibraryID: lib.ID, Name: seriesName + title, NameSource: models.DataSourceFilepath, SortName: seriesName, SortNameSource: models.DataSourceFilepath}
		_, err = f.db.NewInsert().Model(s).Exec(ctx)
		require.NoError(f.t, err)
		_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: b.ID, SeriesID: s.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(f.t, err)
	}
	return b
}

// device adds a Device following rules as its own list, scoped to scope.
func (f *fixture) device(rules []delivery.Rule, scope *devices.Scope) *devices.Device {
	f.t.Helper()
	ctx := context.Background()
	d, err := f.devices.Create(ctx, f.user.ID, "Reader", devices.DeviceTypeOPDS)
	require.NoError(f.t, err)
	opts := devices.UpdateDeviceOptions{Rules: &devices.RulesChange{Mode: devices.RulesModeCustom, Rules: rules}, Scope: scope}
	d, err = f.devices.Update(ctx, f.user.ID, d.ID, opts)
	require.NoError(f.t, err)
	return d
}

func libraryNames(libs []*models.Library) []string {
	names := []string{}
	for _, lib := range libs {
		names = append(names, lib.Name)
	}
	return names
}

var cbzOnly = []delivery.Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ}}

// Libraries leaves out a library with nothing the rules deliver, and
// Sections a library's series and authors sections with none.
func TestLibrariesAndSections_LeaveOutWhatIsEmpty(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	novels := f.library("Novels")
	comics := f.library("Comics")
	f.book(novels, "Novel", models.FileTypeEPUB, "Writer", "Saga")
	f.book(comics, "Loose Comic", models.FileTypeCBZ, "", "")
	f.book(comics, "Novel In Comics", models.FileTypeEPUB, "Writer", "Saga")

	r, err := f.svc.Resolve(ctx, f.user, f.device(cbzOnly, nil))
	require.NoError(t, err)
	libs, err := f.svc.Libraries(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, []string{"Comics"}, libraryNames(libs))

	// Comics has a deliverable comic, but its only series and author are on
	// an EPUB.
	sections, err := f.svc.Sections(ctx, r, comics.ID)
	require.NoError(t, err)
	assert.Equal(t, LibrarySections{}, sections)

	// The account password's default rules deliver everything.
	r, err = f.svc.Resolve(ctx, f.user, nil)
	require.NoError(t, err)
	libs, err = f.svc.Libraries(ctx, r)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Novels", "Comics"}, libraryNames(libs))
	sections, err = f.svc.Sections(ctx, r, comics.ID)
	require.NoError(t, err)
	assert.Equal(t, LibrarySections{Series: true, Authors: true}, sections)
}

// A list scope counts only the books on the list, and the rules apply to
// the same book: a series or author with a deliverable book elsewhere, and
// another book on the list, is empty.
func TestSections_ListScopeAndRulesApplyToTheSameBook(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Lib")
	onList := f.book(lib, "On List", models.FileTypeEPUB, "Writer", "Saga")
	offList := f.book(lib, "Off List", models.FileTypeCBZ, "", "")
	// Off List shares On List's series and author.
	_, err := f.db.ExecContext(ctx, `INSERT INTO book_series (book_id, series_id, sort_order) SELECT ?, series_id, 2 FROM book_series WHERE book_id = ?`, offList.ID, onList.ID)
	require.NoError(t, err)
	_, err = f.db.ExecContext(ctx, `INSERT INTO authors (book_id, person_id, sort_order) SELECT ?, person_id, 1 FROM authors WHERE book_id = ?`, offList.ID, onList.ID)
	require.NoError(t, err)
	listService := lists.NewService(f.db)
	list, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: f.user.ID, Name: "Shelf"})
	require.NoError(t, err)
	require.NoError(t, listService.AddBooks(ctx, lists.AddBooksOptions{ListID: list.ID, BookIDs: []int{onList.ID}, AddedByUserID: f.user.ID}))

	// Without a list scope, the shared series and author have a CBZ.
	whole, err := f.svc.Resolve(ctx, f.user, f.device(cbzOnly, nil))
	require.NoError(t, err)
	sections, err := f.svc.Sections(ctx, whole, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, LibrarySections{Series: true, Authors: true}, sections)

	r, err := f.svc.Resolve(ctx, f.user, f.device(cbzOnly, &devices.Scope{Type: devices.ScopeList, ListID: &list.ID}))
	require.NoError(t, err)
	sections, err = f.svc.Sections(ctx, r, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, LibrarySections{}, sections)
	libs, err := f.svc.Libraries(ctx, r)
	require.NoError(t, err)
	assert.Empty(t, libs)
	has, err := f.svc.HasBooks(ctx, r, books.ListBooksOptions{})
	require.NoError(t, err)
	assert.False(t, has, "the list's only book is an EPUB")
}

// A scope whose library is gone or that the owner can no longer access
// reaches nothing.
func TestScope_ClosedScopeReachesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Lib")
	b := f.book(lib, "Book", models.FileTypeCBZ, "", "")
	r, err := f.svc.Resolve(ctx, f.user, f.device(cbzOnly, &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &lib.ID}))
	require.NoError(t, err)
	assert.True(t, r.HoldsLibrary(lib.ID))

	// The owner loses access to every library but another one.
	other := f.library("Other")
	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: &other.ID}}
	r, err = f.svc.Resolve(ctx, f.user, r.Device)
	require.NoError(t, err)
	assert.True(t, r.Closed())
	assert.False(t, r.HoldsLibrary(lib.ID))
	holds, err := f.svc.HoldsBook(ctx, r, lib.ID, b.ID)
	require.NoError(t, err)
	assert.False(t, holds)
	has, err := f.svc.HasBooks(ctx, r, books.ListBooksOptions{})
	require.NoError(t, err)
	assert.False(t, has)
	libs, err := f.svc.Libraries(ctx, r)
	require.NoError(t, err)
	assert.Empty(t, libs)
}
