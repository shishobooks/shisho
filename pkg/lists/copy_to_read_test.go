package lists

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func copyToToRead(t *testing.T, h *handler, user *models.User, listID int) (CopyToToReadResponse, error) {
	t.Helper()
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(listID))
	var resp CopyToToReadResponse
	if err := h.copyToToRead(c); err != nil {
		return resp, err
	}
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp, nil
}

// toReadQueue returns userID's To Read, in order.
func toReadQueue(t *testing.T, db *bun.DB, userID int) []int {
	t.Helper()
	ids := []int{}
	err := db.NewSelect().
		Model((*models.UserBook)(nil)).
		Column("book_id").
		Where("user_id = ? AND to_read_position IS NOT NULL", userID).
		Order("to_read_position ASC").
		Scan(context.Background(), &ids)
	require.NoError(t, err)
	return ids
}

// Copy to To Read appends an ordered List's Books in the List's order,
// skipping Books already queued.
func TestCopyToToRead_KeepsTheListsOrderAndSkipsQueued(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	lib := createTestLibrary(t, db, "Lib")
	a, b, c, queued := createTestBook(t, db, lib.ID, "A"), createTestBook(t, db, lib.ID, "B"), createTestBook(t, db, lib.ID, "C"), createTestBook(t, db, lib.ID, "Queued")
	ctx := t.Context()
	require.NoError(t, reading.NewService(db).AddToRead(ctx, user.ID, queued.ID))

	list, err := h.listsService.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "Queue", IsOrdered: true, DefaultSort: models.ListSortManual})
	require.NoError(t, err)
	require.NoError(t, h.listsService.AddBooks(ctx, AddBooksOptions{ListID: list.ID, BookIDs: []int{a.ID, b.ID, queued.ID, c.ID}, AddedByUserID: user.ID}))
	require.NoError(t, h.listsService.ReorderBooks(ctx, ReorderBooksOptions{ListID: list.ID, BookIDs: []int{c.ID, queued.ID, a.ID, b.ID}}))

	resp, err := copyToToRead(t, h, user, list.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.Added)
	assert.Equal(t, []int{queued.ID, c.ID, a.ID, b.ID}, toReadQueue(t, db, user.ID))

	// Copying again adds nothing.
	resp, err = copyToToRead(t, h, user, list.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Added)
	assert.Equal(t, []int{queued.ID, c.ID, a.ID, b.ID}, toReadQueue(t, db, user.ID))
}

// An unordered List copies in its default sort.
func TestCopyToToRead_UnorderedListUsesItsDefaultSort(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	lib := createTestLibrary(t, db, "Lib")
	zebra, apple := createTestBook(t, db, lib.ID, "Zebra"), createTestBook(t, db, lib.ID, "Apple")
	ctx := t.Context()

	list, err := h.listsService.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "Shelf", DefaultSort: models.ListSortTitleAsc})
	require.NoError(t, err)
	require.NoError(t, h.listsService.AddBooks(ctx, AddBooksOptions{ListID: list.ID, BookIDs: []int{zebra.ID, apple.ID}, AddedByUserID: user.ID}))

	_, err = copyToToRead(t, h, user, list.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{apple.ID, zebra.ID}, toReadQueue(t, db, user.ID))
}

// A viewer of a shared List copies it into their own To Read, leaving out
// Books in libraries they can't reach.
func TestCopyToToRead_SharedListGoesToTheViewersOwnQueue(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	owner := createTestUser(t, db, "owner")
	owner.LibraryAccess = []*models.UserLibraryAccess{{UserID: owner.ID}}
	lib := createTestLibrary(t, db, "Lib")
	otherLib := createTestLibrary(t, db, "Other")
	viewer := createTestUser(t, db, "viewer")
	viewer.LibraryAccess = []*models.UserLibraryAccess{{UserID: viewer.ID, LibraryID: &lib.ID}}
	reachable, unreachable := createTestBook(t, db, lib.ID, "Reachable"), createTestBook(t, db, otherLib.ID, "Unreachable")
	ctx := t.Context()

	list, err := h.listsService.CreateList(ctx, CreateListOptions{UserID: owner.ID, Name: "Shared"})
	require.NoError(t, err)
	require.NoError(t, h.listsService.AddBooks(ctx, AddBooksOptions{ListID: list.ID, BookIDs: []int{reachable.ID, unreachable.ID}, AddedByUserID: owner.ID}))
	_, err = h.listsService.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: viewer.ID, Permission: models.ListPermissionViewer, SharedByUserID: owner.ID})
	require.NoError(t, err)

	resp, err := copyToToRead(t, h, viewer, list.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Added)
	assert.Equal(t, []int{reachable.ID}, toReadQueue(t, db, viewer.ID))
	assert.Empty(t, toReadQueue(t, db, owner.ID))
}

func TestCopyToToRead_ListTheUserCantViewIsNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	owner := createTestUser(t, db, "owner")
	stranger := createTestUser(t, db, "stranger")
	stranger.LibraryAccess = []*models.UserLibraryAccess{{UserID: stranger.ID}}

	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: owner.ID, Name: "Private"})
	require.NoError(t, err)

	for _, id := range []int{list.ID, 999} {
		_, err = copyToToRead(t, h, stranger, id)
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr)
		assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
	}
}

// The 404 is only for a List the user can't view: a failed lookup is a 500.
func TestCopyToToRead_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Queue"})
	require.NoError(t, err)
	_, err = db.Exec("DROP TABLE list_books")
	require.NoError(t, err)

	_, err = copyToToRead(t, h, user, list.ID)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "a failed lookup is not a 4xx")
}
