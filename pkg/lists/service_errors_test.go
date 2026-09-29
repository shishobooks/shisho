package lists

import (
	"context"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Moving a Book that is not in the list returns a 404 that says so once.
func TestService_MoveBookToPosition_BookNotInListReturnsNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "mover")
	library := createTestLibrary(t, db, "movelib")
	inList := createTestBook(t, db, library.ID, "In List")
	outside := createTestBook(t, db, library.ID, "Outside")
	list, err := svc.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "Ordered", IsOrdered: true})
	require.NoError(t, err)
	require.NoError(t, svc.AddBooks(ctx, AddBooksOptions{ListID: list.ID, BookIDs: []int{inList.ID}, AddedByUserID: user.ID}))

	err = svc.MoveBookToPosition(ctx, list.ID, outside.ID, 1)

	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
	assert.Equal(t, "not_found", ecErr.Code)
	assert.Equal(t, "Book in list not found.", ecErr.Message)
}
