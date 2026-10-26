package devices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type handlerFixture struct {
	t  *testing.T
	db *bun.DB
	h  *handler
	e  *echo.Echo
}

func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return &handlerFixture{t: t, db: db, h: newHandler(NewService(db), lists.NewService(db)), e: e}
}

// call runs fn as user with the JSON body and the id param, and returns
// the recorder and fn's error.
func (f *handlerFixture) call(fn echo.HandlerFunc, user *models.User, body, id string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := f.e.NewContext(req, rec)
	auth.SetUser(c, user)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	return rec, fn(c)
}

// libraryUser is a user with access to only libraryIDs.
func libraryUser(id int, libraryIDs ...int) *models.User {
	user := &models.User{ID: id}
	for _, libraryID := range libraryIDs {
		user.LibraryAccess = append(user.LibraryAccess, &models.UserLibraryAccess{UserID: id, LibraryID: intPtr(libraryID)})
	}
	return user
}

func TestHandler_Create_ReturnsTheCredentialAndConnections(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")

	rec, err := f.call(f.h.Create, libraryUser(1), `{"name":" Libra 2 ","device_type":"kobo"}`, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp DeviceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Libra 2", resp.Name)
	assert.Equal(t, DeviceTypeKobo, resp.DeviceType)
	assert.Len(t, resp.Credential, credentialLength)
	assert.Equal(t, []string{ConnectionKoboSync, ConnectionEReaderBrowser}, resp.Connections)
	assert.Equal(t, ScopeAll, resp.ScopeType)
}

func TestHandler_Create_RejectsAnUnknownType(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")

	_, err := f.call(f.h.Create, libraryUser(1), `{"name":"Tablet","device_type":"tablet"}`, "")
	requireStatus(t, err, http.StatusUnprocessableEntity)
	_, err = f.call(f.h.Create, libraryUser(1), `{"name":"  ","device_type":"kobo"}`, "")
	requireStatus(t, err, http.StatusUnprocessableEntity)
}

func TestHandler_Update_SetsScope(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	insertTestUser(t, f.db, 2, "other")
	insertTestLibrary(t, f.db, 1)
	insertTestLibrary(t, f.db, 2)
	ctx := context.Background()
	device, err := f.h.service.Create(ctx, 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)
	listService := lists.NewService(f.db)
	ownList, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: 1, Name: "Mine"})
	require.NoError(t, err)
	otherList, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: 2, Name: "Theirs"})
	require.NoError(t, err)
	user := libraryUser(1, 1)

	rec, err := f.call(f.h.Update, user, `{"scope":{"type":"library","library_id":1}}`, device.ID)
	require.NoError(t, err)
	var resp DeviceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, ScopeLibrary, resp.ScopeType)
	assert.Equal(t, intPtr(1), resp.ScopeLibraryID)
	assert.Equal(t, "Library", resp.ScopeName)

	rec, err = f.call(f.h.Update, user, `{"scope":{"type":"list","list_id":`+itoa(ownList.ID)+`}}`, device.ID)
	require.NoError(t, err)
	resp = DeviceResponse{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, ScopeList, resp.ScopeType)
	assert.Nil(t, resp.ScopeLibraryID)
	assert.Equal(t, intPtr(ownList.ID), resp.ScopeListID)
	assert.Equal(t, "Mine", resp.ScopeName)

	for name, body := range map[string]string{
		"library without access": `{"scope":{"type":"library","library_id":2}}`,
		"missing library":        `{"scope":{"type":"library","library_id":99}}`,
		"library without id":     `{"scope":{"type":"library"}}`,
		"someone else's list":    `{"scope":{"type":"list","list_id":` + itoa(otherList.ID) + `}}`,
		"unknown scope":          `{"scope":{"type":"shelf"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.call(f.h.Update, user, body, device.ID)
			requireStatus(t, err, http.StatusUnprocessableEntity)
		})
	}
}

func TestHandler_OtherUsersDevice_Returns404(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "owner")
	insertTestUser(t, f.db, 2, "other")
	device, err := f.h.service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)

	for name, fn := range map[string]echo.HandlerFunc{
		"retrieve":   f.h.Retrieve,
		"update":     f.h.Update,
		"regenerate": f.h.RegenerateCredential,
		"delete":     f.h.Delete,
		"short url":  f.h.GenerateShortURL,
		"kobo sync":  f.h.ClearKoboSync,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.call(fn, libraryUser(2), `{}`, device.ID)
			requireStatus(t, err, http.StatusNotFound)
		})
	}
}

// The admin listing shows every user's Devices without their credentials,
// and revoking one cuts it off.
func TestHandler_ListAll_HidesCredentialsAndRevokeCutsOff(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "admin")
	insertTestUser(t, f.db, 2, "reader")
	ctx := context.Background()
	readerKobo, err := f.h.service.Create(ctx, 2, "Reader's Kobo", DeviceTypeKobo)
	require.NoError(t, err)
	_, err = f.h.service.Create(ctx, 1, "Admin's Kindle", DeviceTypeKindle)
	require.NoError(t, err)

	rec, err := f.call(f.h.ListAll, libraryUser(1), "", "")
	require.NoError(t, err)
	assert.NotContains(t, rec.Body.String(), readerKobo.Credential)
	assert.NotContains(t, rec.Body.String(), "credential")

	var resp []AdminDeviceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 2)
	assert.Equal(t, "admin", resp[0].Owner.Username)
	assert.Equal(t, "Reader's Kobo", resp[1].Name)
	assert.Equal(t, models.UserRef{ID: 2, Username: "reader"}, resp[1].Owner)
	assert.Equal(t, []string{ConnectionKoboSync, ConnectionEReaderBrowser}, resp[1].Connections)

	rec, err = f.call(f.h.Revoke, libraryUser(1), "", readerKobo.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, err = runDeviceAuth(t, f.h.service, ConnectionKoboSync, readerKobo.Credential)
	requireStatus(t, err, http.StatusUnauthorized)

	_, err = f.call(f.h.Revoke, libraryUser(1), "", readerKobo.ID)
	requireStatus(t, err, http.StatusNotFound)
}

// A Device that cannot be read is a 500, not the 404 a missing one gets.
func TestHandler_UnreadableDevice_Returns500(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	device, err := f.h.service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)
	_, err = f.db.ExecContext(context.Background(), `UPDATE devices SET created_at = 'not a time' WHERE id = ?`, device.ID)
	require.NoError(t, err)

	for name, fn := range map[string]echo.HandlerFunc{
		"retrieve":   f.h.Retrieve,
		"update":     f.h.Update,
		"regenerate": f.h.RegenerateCredential,
		"short url":  f.h.GenerateShortURL,
		"kobo sync":  f.h.ClearKoboSync,
	} {
		_, err := f.call(fn, libraryUser(1), `{}`, device.ID)
		require.Error(t, err, name)
		var ec *errcodes.Error
		assert.NotErrorAs(t, err, &ec, "%s: %v", name, err)
	}

	_, err = f.call(f.h.ListAll, libraryUser(1), "", "")
	require.Error(t, err)
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// A scope whose library or list cannot be checked is a server error, not
// the 422 an inaccessible one gets.
func TestHandler_Update_ScopeCheckFailureReturns500(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ rename, body string }{
		"library": {`ALTER TABLE libraries RENAME TO libraries_gone`, `{"scope":{"type":"library","library_id":1}}`},
		"list":    {`ALTER TABLE lists RENAME TO lists_gone`, `{"scope":{"type":"list","list_id":1}}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHandlerFixture(t)
			insertTestUser(t, f.db, 1, "reader")
			insertTestLibrary(t, f.db, 1)
			device, err := f.h.service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
			require.NoError(t, err)
			_, err = f.db.ExecContext(context.Background(), tc.rename)
			require.NoError(t, err)

			_, err = f.call(f.h.Update, libraryUser(1, 1), tc.body, device.ID)
			require.Error(t, err)
			var ec *errcodes.Error
			assert.NotErrorAs(t, err, &ec, "%v", err)
		})
	}
}

// The Device page edits a Device's rules through PATCH: follow the default,
// follow the preset, or a custom list, which is refused when empty or when
// it delivers a File Type the type can't receive.
func TestHandler_Update_SetsDeliveryRules(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	device, err := f.h.service.Create(context.Background(), 1, "Kindle", DeviceTypeKindle)
	require.NoError(t, err)
	user := libraryUser(1)

	decode := func(rec *httptest.ResponseRecorder) DeviceResponse {
		var resp DeviceResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	rec, err := f.call(f.h.Retrieve, user, "", device.ID)
	require.NoError(t, err)
	resp := decode(rec)
	assert.Equal(t, RulesModePreset, resp.DeliveryRulesMode)
	assert.True(t, resp.HasPreset)
	assert.Equal(t, PresetRules(DeviceTypeKindle), resp.DeliveryRules)
	// The editor only offers what a Kindle can receive.
	for _, option := range resp.DeliveryRuleOptions {
		for _, delivered := range option.DeliveredFileTypes {
			assert.True(t, AllowsFileType(DeviceTypeKindle, delivered), "%s offered as %s", option.SourceFileType, delivered)
		}
	}
	sources := []string{}
	for _, option := range resp.DeliveryRuleOptions {
		sources = append(sources, option.SourceFileType)
	}
	assert.Equal(t, []string{models.FileTypeEPUB, models.FileTypeKePub, models.FileTypeAZW3, models.FileTypeMOBI, models.FileTypeCBZ}, sources)

	rec, err = f.call(f.h.Update, user, `{"delivery_rules":{"mode":"custom","rules":[{"source_file_type":"mobi","delivered_file_type":"mobi"}]}}`, device.ID)
	require.NoError(t, err)
	resp = decode(rec)
	assert.Equal(t, RulesModeCustom, resp.DeliveryRulesMode)
	assert.Equal(t, []delivery.Rule{rule(models.FileTypeMOBI, models.FileTypeMOBI)}, resp.DeliveryRules)

	for name, body := range map[string]string{
		"empty list":       `{"delivery_rules":{"mode":"custom","rules":[]}}`,
		"EPUB on a Kindle": `{"delivery_rules":{"mode":"custom","rules":[{"source_file_type":"epub","delivered_file_type":"epub"}]}}`,
		"unknown mode":     `{"delivery_rules":{"mode":"sometimes"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.call(f.h.Update, user, body, device.ID)
			requireStatus(t, err, http.StatusUnprocessableEntity)
		})
	}

	rec, err = f.call(f.h.Update, user, `{"delivery_rules":{"mode":"default"}}`, device.ID)
	require.NoError(t, err)
	resp = decode(rec)
	assert.Equal(t, RulesModeDefault, resp.DeliveryRulesMode)
	// The built-in default's AZW3 and MOBI rules, without the ones a Kindle
	// can't receive.
	assert.Equal(t, []delivery.Rule{rule(models.FileTypeAZW3, models.FileTypeAZW3), rule(models.FileTypeMOBI, models.FileTypeMOBI)}, resp.DeliveryRules)
}

func TestHandler_Update_ChangesType(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	device, err := f.h.service.Create(context.Background(), 1, "Reader", DeviceTypeBrowser)
	require.NoError(t, err)

	rec, err := f.call(f.h.Update, libraryUser(1), `{"device_type":"kindle"}`, device.ID)
	require.NoError(t, err)
	var resp DeviceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, DeviceTypeKindle, resp.DeviceType)
	assert.Equal(t, RulesModePreset, resp.DeliveryRulesMode)

	_, err = f.call(f.h.Update, libraryUser(1), `{"device_type":"tablet"}`, device.ID)
	requireStatus(t, err, http.StatusUnprocessableEntity)

	// Rules the new type can't follow refuse the whole change, so the type
	// stays as it was.
	_, err = f.call(f.h.Update, libraryUser(1), `{"device_type":"kobo","delivery_rules":{"mode":"custom","rules":[{"source_file_type":"cbz","delivered_file_type":"cbz"}]}}`, device.ID)
	requireStatus(t, err, http.StatusUnprocessableEntity)
	unchanged, err := f.h.service.Retrieve(context.Background(), 1, device.ID)
	require.NoError(t, err)
	assert.Equal(t, DeviceTypeKindle, unchanged.DeviceType)
}

// queryCounter counts the queries a database runs.
type queryCounter struct{ n atomic.Int64 }

func (q *queryCounter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (q *queryCounter) AfterQuery(context.Context, *bun.QueryEvent) { q.n.Add(1) }

// Listing Devices loads every Device's rules in a fixed number of queries,
// however many Devices there are and whichever rules they follow.
func TestHandler_List_RulesQueriesDontGrowWithDevices(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	listQueries := func(customDevices, followingDevices int) int64 {
		f := newHandlerFixture(t)
		insertTestUser(t, f.db, 1, "reader")
		for range customDevices {
			d, err := f.h.service.Create(ctx, 1, "Custom", DeviceTypeOPDS)
			require.NoError(t, err)
			_, err = f.h.service.SetRules(ctx, 1, d.ID, RulesModeCustom, []delivery.Rule{rule(models.FileTypeEPUB, models.FileTypeEPUB)})
			require.NoError(t, err)
		}
		for range followingDevices {
			_, err := f.h.service.Create(ctx, 1, "Following", DeviceTypeBrowser)
			require.NoError(t, err)
		}
		counter := &queryCounter{}
		testdb.AddQueryHook(t, f.db, counter)
		rec, err := f.call(f.h.List, libraryUser(1), "", "")
		require.NoError(t, err)
		var resp []DeviceResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp, customDevices+followingDevices)
		return counter.n.Load()
	}

	assert.Equal(t, listQueries(1, 1), listQueries(4, 4))
}
