package reach

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve calls a reach handler as the fixture's user, with the route's
// :id param, and returns the response.
func (f *fixture) serve(handle func(*handler, echo.Context) error, id, query string) *httptest.ResponseRecorder {
	f.t.Helper()
	h := &handler{service: f.svc, bookService: f.svc.bookService, deviceService: f.devices}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?"+query, nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	auth.SetUser(c, f.user)
	if err := handle(h, c); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

func bookDevicesRoute(h *handler, c echo.Context) error     { return h.bookDevices(c) }
func deliveryPreviewRoute(h *handler, c echo.Context) error { return h.deliveryPreview(c) }

func TestBookDevicesHandler(t *testing.T) {
	t.Parallel()

	t.Run("lists every Device as an array", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		b := f.book(f.library("Lib"), "Book", models.FileTypeEPUB, "", "")
		f.kobo()

		rec := f.serve(bookDevicesRoute, strconv.Itoa(b.ID), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var lines []BookDevice
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lines))
		assert.Len(t, lines, 1)
	})

	t.Run("refuses a book in a library the user can't access", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		allowed := f.library("Allowed")
		hidden := f.library("Hidden")
		b := f.book(hidden, "Book", models.FileTypeEPUB, "", "")
		f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: &allowed.ID}}

		rec := f.serve(bookDevicesRoute, strconv.Itoa(b.ID), "")
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	})

	t.Run("a broken Devices table is a server fault", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		b := f.book(f.library("Lib"), "Book", models.FileTypeEPUB, "", "")
		f.kobo()
		_, err := f.db.ExecContext(context.Background(), `UPDATE devices SET created_at = 'not a time'`)
		require.NoError(t, err)

		rec := f.serve(bookDevicesRoute, strconv.Itoa(b.ID), "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})
}

func TestDeliveryPreviewHandler(t *testing.T) {
	t.Parallel()

	t.Run("a Device that doesn't exist is a 404", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		rec := f.serve(deliveryPreviewRoute, "missing", "")
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("a broken Device row is a server fault, not a 404", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		kobo := f.kobo()
		_, err := f.db.ExecContext(context.Background(), `UPDATE devices SET created_at = 'not a time'`)
		require.NoError(t, err)

		rec := f.serve(deliveryPreviewRoute, kobo.ID, "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("rules a Device can't follow are a 422, a broken rules table a 500", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		kobo := f.kobo()

		rec := f.serve(deliveryPreviewRoute, kobo.ID, "mode=custom&rules=pdf:pdf")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

		_, err := f.db.ExecContext(context.Background(), `DROP TABLE user_delivery_rules`)
		require.NoError(t, err)
		rec = f.serve(deliveryPreviewRoute, kobo.ID, "mode=default")
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("previews proposed rules", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		lib := f.library("Lib")
		f.book(lib, "Novel", models.FileTypeEPUB, "", "")
		f.book(lib, "Comic", models.FileTypeCBZ, "", "")
		kobo := f.kobo()

		rec := f.serve(deliveryPreviewRoute, kobo.ID, "mode=custom&rules=cbz:epub")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var preview DeliveryPreviewResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
		assert.Equal(t, 1, preview.ReceivedBooks)
		assert.Equal(t, []NotReceivedCount{{SourceFileType: models.FileTypeEPUB, Books: 1}}, preview.NotReceived)
	})
}
