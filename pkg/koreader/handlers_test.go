package koreader

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // KOReader's protocol.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// The requests in these tests have the shapes KOReader v2026.07.1 sent the
// harness's capture server (tools/koreaderharness): its headers, its progress
// and percentage formats, and its device fields.
const (
	koreaderDevice   = "Linux"
	koreaderDeviceID = "3B4045D82B044E0892FAFD7BD4BD187B"
	epubHash         = "dcc8fb3ab2c17a8bc3e90312f91c0823"
	cbzHash          = "a07ac168355942aaab99a3883774b0f3"
	xpointer         = "/body/DocFragment[2]/body/p[20]/text().371"
)

type syncFixture struct {
	t       *testing.T
	ctx     context.Context
	db      *bun.DB
	e       *echo.Echo
	user    *models.User
	lib     *models.Library
	device  *devices.Device
	reading *reading.Service
	devices *devices.Service
	hashes  *fingerprints.Service
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(e, db, books.NewService(db, appsettings.NewService(db)))

	f := &syncFixture{
		t: t, ctx: context.Background(), db: db, e: e,
		reading: reading.NewService(db),
		devices: devices.NewService(db),
		hashes:  fingerprints.NewService(db),
	}
	f.user = f.addUser("reader", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	f.lib = f.addLibrary("Lib")
	f.grantLibrary(f.user, nil)
	f.device, err = f.devices.Create(f.ctx, f.user.ID, "Libra KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)
	return f
}

func (f *syncFixture) addUser(username string, permissions ...models.Permission) *models.User {
	f.t.Helper()
	role := &models.Role{Name: username + "-role", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := f.db.NewInsert().Model(role).Exec(f.ctx)
	require.NoError(f.t, err)
	for _, p := range permissions {
		p.RoleID = role.ID
		_, err = f.db.NewInsert().Model(&p).Exec(f.ctx)
		require.NoError(f.t, err)
	}
	hash, err := auth.HashPassword("secret")
	require.NoError(f.t, err)
	user := &models.User{Username: username, PasswordHash: hash, RoleID: role.ID, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = f.db.NewInsert().Model(user).Exec(f.ctx)
	require.NoError(f.t, err)
	return user
}

func (f *syncFixture) addLibrary(name string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(lib).Exec(f.ctx)
	require.NoError(f.t, err)
	return lib
}

func (f *syncFixture) grantLibrary(user *models.User, libraryID *int) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID, LibraryID: libraryID}).Exec(f.ctx)
	require.NoError(f.t, err)
}

// addFile adds a Book in lib with one main File, mapped from hash delivered
// as its own type.
func (f *syncFixture) addFile(lib *models.Library, fileType, hash string) *models.File {
	f.t.Helper()
	title := fmt.Sprintf("%s-%s", fileType, hash)
	book := &models.Book{
		LibraryID: lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title,
		SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/books/" + title,
	}
	_, err := f.db.NewInsert().Model(book).Exec(f.ctx)
	require.NoError(f.t, err)
	file := &models.File{
		LibraryID: lib.ID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain,
		Filepath: fmt.Sprintf("/books/%s/%d.%s", title, lib.ID, fileType), FilesizeBytes: 1,
	}
	_, err = f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	require.NoError(f.t, f.hashes.RecordDocumentHash(f.ctx, file.ID, fileType, hash))
	return file
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // KOReader's protocol.
	return hex.EncodeToString(sum[:])
}

// serve sends what KOReader sends, signed in with username and the MD5 of
// password when username isn't empty.
func (f *syncFixture) serve(method, path, username, password, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Accept", "application/vnd.koreader.v1+json")
	req.Header.Set("User-Agent", "lua-Spore")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if username != "" {
		req.Header.Set("x-auth-user", username)
		req.Header.Set("x-auth-key", md5Hex(password))
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec
}

func (f *syncFixture) push(document, progress string, percentage float64) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.pushAs(f.device, document, progress, percentage)
}

func (f *syncFixture) pushAs(device *devices.Device, document, progress string, percentage float64) *httptest.ResponseRecorder {
	f.t.Helper()
	body := fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":%v,"device":%q}`,
		koreaderDeviceID, progress, document, percentage, koreaderDevice)
	return f.serve(http.MethodPut, "/koreader/syncs/progress", f.user.Username, device.Credential, body)
}

func (f *syncFixture) pull(document string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.pullAs(f.device, document)
}

func (f *syncFixture) pullAs(device *devices.Device, document string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.serve(http.MethodGet, "/koreader/syncs/progress/"+document, f.user.Username, device.Credential, "")
}

func (f *syncFixture) status(file *models.File) string {
	f.t.Helper()
	var outcome string
	err := f.db.NewSelect().Model((*models.ReadThrough)(nil)).Column("rt.outcome").
		Where("rt.user_id = ? AND rt.book_id = ?", f.user.ID, file.BookID).Scan(f.ctx, &outcome)
	require.NoError(f.t, err)
	return outcome
}

func TestAuth_DeviceCredentialSignsIn(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	rec := f.serve(http.MethodGet, "/koreader/users/auth", "reader", f.device.Credential, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"authorized":"OK"}`, rec.Body.String())

	// Usernames match in any case, as password sign-in does.
	rec = f.serve(http.MethodGet, "/koreader/users/auth", "READER", f.device.Credential, "")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestAuth_Failures(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	opds, err := f.devices.Create(f.ctx, f.user.ID, "Panels", devices.DeviceTypeOPDS)
	require.NoError(t, err)
	other := f.addUser("other", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	otherDevice, err := f.devices.Create(f.ctx, other.ID, "Other", devices.DeviceTypeKOReader)
	require.NoError(t, err)

	tests := map[string]struct{ username, password string }{
		"no credentials":              {"", ""},
		"wrong password":              {"reader", "wrong"},
		"unknown user":                {"nobody", f.device.Credential},
		"account password":            {"reader", "secret"},
		"device of another type":      {"reader", opds.Credential},
		"another user's device":       {"reader", otherDevice.Credential},
		"credential for another user": {"other", f.device.Credential},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := f.serve(http.MethodGet, "/koreader/users/auth", tt.username, tt.password, "")
			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			// KOReader shows body.message as it stands.
			assert.Contains(t, rec.Body.String(), `"message":"`)
		})
	}
}

// The key is the password's MD5, so sending the credential itself fails.
func TestAuth_KeyIsTheCredentialsMD5(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/koreader/users/auth", nil)
	req.Header.Set("x-auth-user", "reader")
	req.Header.Set("x-auth-key", f.device.Credential)
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_RegeneratingTheCredentialStopsTheOldOne(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)

	old := f.device.Credential
	regenerated, err := f.devices.RegenerateCredential(f.ctx, f.user.ID, f.device.ID)
	require.NoError(t, err)

	body := fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":0.5,"device":%q}`, koreaderDeviceID, xpointer, epubHash, koreaderDevice)
	assert.Equal(t, http.StatusUnauthorized, f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", old, body).Code)
	assert.Equal(t, http.StatusOK, f.pushAs(regenerated, epubHash, xpointer, 0.5).Code)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, f.status(file))
}

func TestAuth_OwnerWhoLostAccess(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	// A role without books:read can't sync, and KOReader only expects a 401.
	noBooks := f.addUser("nobooks")
	device, err := f.devices.Create(f.ctx, noBooks.ID, "KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, f.serve(http.MethodGet, "/koreader/users/auth", "nobooks", device.Credential, "").Code)

	// Deactivating the owner stops their Device.
	_, err = f.db.NewUpdate().Model(f.user).Set("is_active = FALSE").WherePK().Exec(f.ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, f.serve(http.MethodGet, "/koreader/users/auth", "reader", f.device.Credential, "").Code)
}

func TestAuth_LoadFailureIsAServerFault(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	_, err := f.db.ExecContext(f.ctx, "DROP TABLE devices")
	require.NoError(t, err)
	rec := f.serve(http.MethodGet, "/koreader/users/auth", "reader", "anything", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}

func TestRegister_AlwaysRefusedWithTheWayForward(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	for _, username := range []string{"reader", "brand-new"} {
		body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, md5Hex("p"))
		rec := f.serve(http.MethodPost, "/koreader/users/create", "", "", body)
		// 402 is what KOReader expects for a refusal; it shows body.message.
		require.Equal(t, http.StatusPaymentRequired, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "Devices")
		assert.Contains(t, rec.Body.String(), "KOReader Device")
	}

	exists, err := f.db.NewSelect().Model((*models.User)(nil)).Where("username = ?", "brand-new").Exists(f.ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestProgress_UnknownDocumentHasNoProgress(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	rec := f.pull("00000000000000000000000000000000")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{}`, rec.Body.String())

	// KOReader queues any push that isn't a 200 and retries it for weeks, so
	// a book Shisho doesn't know is answered and logged, not refused. With
	// send_metadata on, it carries what KOReader knows about the book.
	body := `{"device_id":"X","progress":"/body/p","document":"00000000000000000000000000000000","percentage":0.5,"device":"Linux","metadata":{"title":"Some Book","authors":"Someone","filename":"some.epub"}}`
	rec = f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", f.device.Credential, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"document":"00000000000000000000000000000000"`)
}

func TestProgress_SameDeviceRoundTrip(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)

	rec := f.push(epubHash, xpointer, 0.3058)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"document":"`+epubHash+`"`)
	assert.Contains(t, rec.Body.String(), `"timestamp":`)

	rec = f.pull(epubHash)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	// What KOReader sent, as its own server echoes it: KOReader ignores a
	// position carrying its own name and id, so syncing never moves the
	// device that pushed it.
	var pulled map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pulled))
	assert.Contains(t, pulled, "timestamp")
	delete(pulled, "timestamp")
	assert.Equal(t, map[string]any{
		"document": epubHash, "progress": xpointer, "percentage": 0.3058,
		"device": koreaderDevice, "device_id": koreaderDeviceID,
	}, pulled)
}

// A push that leaves out how KOReader names itself gets back the Device's
// identity in Shisho, and no time, so a drifting clock can't make KOReader
// offer to move back.
func TestProgress_SameDeviceWithoutItsNameGetsNoTimestamp(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	body := fmt.Sprintf(`{"progress":%q,"document":%q,"percentage":0.4}`, xpointer, epubHash)
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", f.device.Credential, body).Code)

	assert.JSONEq(t, fmt.Sprintf(`{
		"document": %q, "progress": %q, "percentage": 0.4, "device": "Libra KOReader", "device_id": %q
	}`, epubHash, xpointer, f.device.ID), f.pull(epubHash).Body.String())
}

// A KOReader install that renames itself is still the Device, so its own
// position comes back with whatever it last sent.
func TestProgress_SameDeviceEchoesItsLatestName(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	body := fmt.Sprintf(`{"device_id":"NEW-ID","progress":%q,"document":%q,"percentage":0.4,"device":"Kobo Libra"}`, xpointer, epubHash)
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", f.device.Credential, body).Code)

	pulled := f.pull(epubHash).Body.String()
	assert.Contains(t, pulled, `"device":"Kobo Libra"`)
	assert.Contains(t, pulled, `"device_id":"NEW-ID"`)
}

func TestProgress_ReadingStartsAndFinishesTheBook(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)

	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, f.status(file))

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ReadingSourceKOReader, position.Source)
	require.NotNil(t, position.SourceDeviceID)
	assert.Equal(t, f.device.ID, *position.SourceDeviceID)
	require.NotNil(t, position.XPointer)
	assert.Equal(t, xpointer, *position.XPointer)

	// The last 1% is back matter, and KOReader's continuous view never
	// reports 100%, so 99% is the end.
	require.Equal(t, http.StatusOK, f.push(epubHash, "/body/DocFragment[5]/body/p[30]/text().0", 0.9899).Code)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, f.status(file))
	require.Equal(t, http.StatusOK, f.push(epubHash, "/body/DocFragment[5]/body/p[38]/text().0", 0.99).Code)
	assert.Equal(t, models.ReadThroughOutcomeFinished, f.status(file))

	// Going back within the window is the revert rule, for a push that isn't
	// part of a replay.
	f.agePushes()
	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, f.status(file))
}

func TestProgress_PausedBookResumes(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)
	_, err := f.db.NewUpdate().Model((*models.ReadThrough)(nil)).Set("paused = TRUE").Where("book_id = ?", file.BookID).Exec(f.ctx)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, f.push(epubHash, "/body/DocFragment[3]/body/p[1]/text().0", 0.4).Code)

	var paused bool
	err = f.db.NewSelect().Model((*models.ReadThrough)(nil)).Column("rt.paused").Where("rt.book_id = ?", file.BookID).Scan(f.ctx, &paused)
	require.NoError(t, err)
	assert.False(t, paused)
}

// KOReader counts pages from 1, and a stored page from 0, so the page a
// reader sees carries across exactly.
func TestProgress_PageCarriesToTheWebReader(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeCBZ, cbzHash)

	require.Equal(t, http.StatusOK, f.push(cbzHash, "20", 0.6666).Code)

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	require.NotNil(t, position.Page)
	assert.Equal(t, 19, *position.Page)
	assert.Nil(t, position.XPointer)
}

func TestProgress_PageCarriesFromTheWebReader(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeCBZ, cbzHash)
	page := 24
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Page: &page}, Percentage: 25.0 / 30,
	})
	require.NoError(t, err)

	rec := f.pull(cbzHash)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, `"progress":"25"`)
	// Labelled with the web reader's identity, so KOReader doesn't take it
	// for its own and offers the jump.
	assert.Contains(t, body, `"device":"Shisho web reader"`)
	assert.Contains(t, body, `"device_id":"web_reader"`)
	assert.Contains(t, body, `"timestamp":`)
}

func TestProgress_PageCarriesBetweenDevices(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeCBZ, cbzHash)
	phone, err := f.devices.Create(f.ctx, f.user.ID, "Phone KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, f.pushAs(phone, cbzHash, "12", 0.4).Code)

	body := f.pull(cbzHash).Body.String()
	assert.Contains(t, body, `"progress":"12"`)
	assert.Contains(t, body, `"device":"Phone KOReader"`)
	assert.Contains(t, body, fmt.Sprintf(`"device_id":%q`, phone.ID))
}

func TestProgress_DeletedDeviceIsNotTakenForTheWebReader(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeCBZ, cbzHash)
	phone, err := f.devices.Create(f.ctx, f.user.ID, "Phone KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, f.pushAs(phone, cbzHash, "12", 0.4).Code)
	require.NoError(t, f.devices.Delete(f.ctx, f.user.ID, phone.ID))

	body := f.pull(cbzHash).Body.String()
	assert.Contains(t, body, `"progress":"12"`)
	assert.Contains(t, body, `"device":"KOReader"`)
	assert.Contains(t, body, `"device_id":"koreader"`)
}

// A Device can hold a CBZ and its conversion to EPUB. KOReader would fail
// going to a page in the one or an XPointer in the other.
func TestProgress_SameDevicePositionInAnotherCopysFormHasNoProgress(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeCBZ, cbzHash)
	require.NoError(t, f.hashes.RecordDocumentHash(f.ctx, file.ID, models.FileTypeEPUB, epubHash))

	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)
	assert.Contains(t, f.pull(epubHash).Body.String(), xpointer)
	assert.JSONEq(t, `{}`, f.pull(cbzHash).Body.String())

	require.Equal(t, http.StatusOK, f.push(cbzHash, "20", 0.6).Code)
	assert.Contains(t, f.pull(cbzHash).Body.String(), `"progress":"20"`)
	assert.JSONEq(t, `{}`, f.pull(epubHash).Body.String())
}

// Not parallel: it captures the process's log output.
func TestProgress_UnknownDocumentIsLoggedWithWhatKOReaderSent(t *testing.T) {
	f := newSyncFixture(t)
	var logged bytes.Buffer
	original := logger.Output()
	logger.SetOutput(&logged)
	t.Cleanup(func() { logger.SetOutput(original) })

	const unknown = "00000000000000000000000000000000"
	body := fmt.Sprintf(`{"device_id":"X","progress":"/body/p","document":%q,"percentage":0.5,"device":"Linux","metadata":{"title":"Some Book"}}`, unknown)
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", f.device.Credential, body).Code)
	assert.Contains(t, logged.String(), "koreader document not recognized")
	assert.Contains(t, logged.String(), unknown)
	assert.Contains(t, logged.String(), "Some Book")

	logged.Reset()
	require.Equal(t, http.StatusOK, f.pull(unknown).Code)
	assert.Contains(t, logged.String(), unknown)
}

func TestProgress_NoPositionYetHasNoProgress(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	assert.JSONEq(t, `{}`, f.pull(epubHash).Body.String())
}

func TestProgress_IdenticalBytesResolveToTheFileTheUserCanAccess(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	reader := f.addUser("limited", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	hidden := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	otherLib := f.addLibrary("Other")
	visible := f.addFile(otherLib, models.FileTypeEPUB, epubHash)
	f.grantLibrary(reader, &otherLib.ID)
	device, err := f.devices.Create(f.ctx, reader.ID, "Theirs", devices.DeviceTypeKOReader)
	require.NoError(t, err)

	body := fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":0.3,"device":%q}`, koreaderDeviceID, xpointer, epubHash, koreaderDevice)
	rec := f.serve(http.MethodPut, "/koreader/syncs/progress", "limited", device.Credential, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	position, err := f.reading.RetrievePosition(f.ctx, reader.ID, visible.ID)
	require.NoError(t, err)
	assert.NotNil(t, position)
	position, err = f.reading.RetrievePosition(f.ctx, reader.ID, hidden.ID)
	require.NoError(t, err)
	assert.Nil(t, position)
}

// Two Files the user can reach with the same bytes: the position stays with
// the one already being read, so it doesn't split or jump between them.
func TestProgress_IdenticalBytesPreferTheFileAlreadyBeingRead(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	first := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	second := f.addFile(f.addLibrary("Other"), models.FileTypeEPUB, epubHash)
	cfi := "epubcfi(/6/4!/4/2)"
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: second.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.1,
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, second.ID)
	require.NoError(t, err)
	require.NotNil(t, position.XPointer)
	position, err = f.reading.RetrievePosition(f.ctx, f.user.ID, first.ID)
	require.NoError(t, err)
	assert.Nil(t, position)
}

func TestProgress_NoAccessLooksUnknown(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	reader := f.addUser("limited", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	device, err := f.devices.Create(f.ctx, reader.ID, "Theirs", devices.DeviceTypeKOReader)
	require.NoError(t, err)

	body := fmt.Sprintf(`{"device_id":"X","progress":%q,"document":%q,"percentage":0.3,"device":"Linux"}`, xpointer, epubHash)
	rec := f.serve(http.MethodPut, "/koreader/syncs/progress", "limited", device.Credential, body)
	require.Equal(t, http.StatusOK, rec.Code)
	position, err := f.reading.RetrievePosition(f.ctx, reader.ID, file.ID)
	require.NoError(t, err)
	assert.Nil(t, position)

	rec = f.serve(http.MethodGet, "/koreader/syncs/progress/"+epubHash, "limited", device.Credential, "")
	assert.JSONEq(t, `{}`, rec.Body.String())
}

func TestProgress_OutsideTheDevicesScopeLooksUnknown(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	otherLib := f.addLibrary("Other")
	scoped, err := f.devices.Update(f.ctx, f.user.ID, f.device.ID, devices.UpdateDeviceOptions{
		Scope: &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &otherLib.ID},
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, f.pushAs(scoped, epubHash, xpointer, 0.3).Code)
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.Nil(t, position)
	assert.JSONEq(t, `{}`, f.pullAs(scoped, epubHash).Body.String())
}

// A copy downloaded before a metadata edit keeps its hash, which still maps
// to the File next to the hash of what Shisho generates now.
func TestProgress_OlderCopyOfTheSameFileStillSyncs(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	const newerCopy = "11111111111111111111111111111111"
	require.NoError(t, f.hashes.RecordDocumentHash(f.ctx, file.ID, models.FileTypeEPUB, newerCopy))

	require.Equal(t, http.StatusOK, f.push(newerCopy, xpointer, 0.2).Code)
	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.InDelta(t, 0.3, position.Percentage, 1e-9)
}

// An e-reader's clock can't make its updates look stale: the protocol sends
// no time, so an update is dated when it arrives. A web reader update stamped
// by a fast clock counts as now, and the next push still lands.
func TestProgress_DatedOnArrival(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	before := time.Now().Add(-time.Second)
	require.Equal(t, http.StatusOK, f.push(epubHash, xpointer, 0.3).Code)
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.True(t, position.OccurredAt.After(before))
}

func TestProgress_BadPushesAreRejected(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)

	for name, tt := range map[string]struct {
		progress   string
		percentage float64
	}{
		"percentage above 1": {xpointer, 1.5},
		"not a position":     {"chapter one", 0.5},
		"page zero":          {"0", 0.5},
		"negative page":      {"-3", 0.5},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := f.push(epubHash, tt.progress, tt.percentage)
			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		})
	}
}

// A field a later KOReader adds mustn't refuse the push.
func TestProgress_UnknownFieldsAreAccepted(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	body := fmt.Sprintf(`{"device_id":"X","progress":%q,"document":%q,"percentage":0.3,"device":"Linux","something_new":1}`, xpointer, epubHash)
	rec := f.serve(http.MethodPut, "/koreader/syncs/progress", "reader", f.device.Credential, body)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestProgress_ServerFaultsAre500(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	_, err := f.db.ExecContext(f.ctx, "DROP TABLE koreader_document_hashes")
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, f.pull(epubHash).Code)
	assert.Equal(t, http.StatusInternalServerError, f.push(epubHash, xpointer, 0.3).Code)
}
