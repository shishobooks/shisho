package lists

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// addBooksFixture holds a list owned by a user who can access only library A.
type addBooksFixture struct {
	db    *bun.DB
	e     *echo.Echo
	h     *handler
	user  *models.User
	list  *models.List
	bookA *models.Book
	bookB *models.Book
}

func newAddBooksFixture(t *testing.T) *addBooksFixture {
	t.Helper()
	db := testdb.New(t)
	libA := createTestLibrary(t, db, "Library A")
	libB := createTestLibrary(t, db, "Library B")
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: &libA.ID}}
	h := newTestHandler(db)
	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Mine"})
	require.NoError(t, err)
	return &addBooksFixture{
		db:    db,
		e:     newTestEcho(t),
		h:     h,
		user:  user,
		list:  list,
		bookA: createTestBook(t, db, libA.ID, "Alpha"),
		bookB: createTestBook(t, db, libB.ID, "Bravo"),
	}
}

func (f *addBooksFixture) addBooks(t *testing.T, bookIDs ...int) error {
	t.Helper()
	ids := make([]string, len(bookIDs))
	for i, id := range bookIDs {
		ids[i] = strconv.Itoa(id)
	}
	body := `{"book_ids":[` + strings.Join(ids, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := f.e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(f.list.ID))
	auth.SetUser(c, f.user)
	return f.h.addBooks(c)
}

func (f *addBooksFixture) listBookIDs(t *testing.T) []int {
	t.Helper()
	var ids []int
	err := f.db.NewSelect().Model((*models.ListBook)(nil)).Column("book_id").Where("list_id = ?", f.list.ID).Order("book_id").Scan(t.Context(), &ids)
	require.NoError(t, err)
	return ids
}

func TestListsAddBooks_BookInInaccessibleLibrary_Returns403(t *testing.T) {
	t.Parallel()
	f := newAddBooksFixture(t)

	err := f.addBooks(t, f.bookA.ID, f.bookB.ID)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
	assert.Empty(t, f.listBookIDs(t), "a rejected request must add none of its books")

	// Positive control: an accessible book is added.
	require.NoError(t, f.addBooks(t, f.bookA.ID))
	assert.Equal(t, []int{f.bookA.ID}, f.listBookIDs(t))
}

func TestListsAddBooks_UnknownBook_Returns404(t *testing.T) {
	t.Parallel()
	f := newAddBooksFixture(t)

	err := f.addBooks(t, f.bookA.ID, 999999)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
	assert.Equal(t, "Book not found.", codeErr.Message)
	assert.Empty(t, f.listBookIDs(t))
}
