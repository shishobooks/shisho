package koreaderplugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oldCopy is the hash of a copy downloaded before Shisho recorded hashes,
// which no server recognizes.
const oldCopy = "0ddc0b7a11c0b7a11c0b7a11c0b7a11c"

// unrecognizedAbout is what KOReader knows of oldCopy.
func unrecognizedAbout() UnrecognizedPayload {
	return UnrecognizedPayload{
		Document: oldCopy, Title: "The Left Hand of Darkness", Authors: []string{"Ursula K. Le Guin"},
		Identifiers: []string{"isbn:9780441478125"}, Language: "en", FileType: "epub", ReadingSeconds: 7200,
	}
}

// web calls handler as user, with the route's params, as the Device page
// does.
func (f *syncFixture) web(handler echo.HandlerFunc, method, target string, user *models.User, body string, params ...string) (*httptest.ResponseRecorder, error) {
	f.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := f.e.NewContext(req, rec)
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names = append(names, params[i])
		values = append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	loaded, err := auth.LoadUser(f.ctx, f.db, auth.LoadUserOptions{ID: &user.ID})
	require.NoError(f.t, err)
	auth.SetUser(c, loaded)
	return rec, handler(c)
}

func (f *syncFixture) listUnrecognizedAs(user *models.User, device *devices.Device) ([]UnrecognizedBook, error) {
	f.t.Helper()
	rec, err := f.web(f.h.unrecognized, http.MethodGet, "/", user, "", "id", device.ID)
	if err != nil {
		return nil, err
	}
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var books []UnrecognizedBook
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &books))
	return books, nil
}

func (f *syncFixture) listUnrecognized() []UnrecognizedBook {
	f.t.Helper()
	books, err := f.listUnrecognizedAs(f.user, f.device)
	require.NoError(f.t, err)
	return books
}

func (f *syncFixture) linkAs(user *models.User, device *devices.Device, fileID int) error {
	f.t.Helper()
	body, err := json.Marshal(LinkPayload{FileID: fileID})
	require.NoError(f.t, err)
	_, err = f.web(f.h.linkUnrecognized, http.MethodPost, "/", user, string(body), "id", device.ID, "document", oldCopy)
	return err
}

func (f *syncFixture) dismissAs(user *models.User, device *devices.Device) error {
	f.t.Helper()
	_, err := f.web(f.h.dismissUnrecognized, http.MethodPost, "/", user, "", "id", device.ID, "document", oldCopy)
	return err
}

func (f *syncFixture) searchCandidates(query string) []LinkCandidate {
	f.t.Helper()
	rec, err := f.web(f.h.linkCandidates, http.MethodGet, "/?q="+query, f.user, "", "id", f.device.ID)
	require.NoError(f.t, err)
	var candidates []LinkCandidate
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &candidates))
	return candidates
}

func (f *syncFixture) lookupAs(device *devices.Device, documents ...string) map[string]string {
	f.t.Helper()
	body, err := json.Marshal(LookupPayload{Documents: documents})
	require.NoError(f.t, err)
	user := &models.User{}
	require.NoError(f.t, f.db.NewSelect().Model(user).Where("id = ?", device.UserID).Scan(f.ctx))
	rec := f.do(request{path: "/koreader/plugin/lookup", body: string(body), username: user.Username, credential: device.Credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp LookupResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	states := map[string]string{}
	for _, d := range resp.Documents {
		states[d.Document] = d.State
	}
	return states
}

// addBook adds a Book to lib by author with a main File of fileType
// carrying identifiers, indexed for search.
func (f *syncFixture) addBook(lib *models.Library, title, author, fileType string, identifiers ...string) *models.File {
	f.t.Helper()
	book := &models.Book{
		LibraryID: lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title,
		SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/books/" + title,
	}
	_, err := f.db.NewInsert().Model(book).Exec(f.ctx)
	require.NoError(f.t, err)
	person := &models.Person{LibraryID: lib.ID, Name: author, SortName: author, SortNameSource: models.DataSourceFilepath}
	_, err = f.db.NewInsert().Model(person).On("CONFLICT (name, library_id) DO UPDATE").Set("sort_name = EXCLUDED.sort_name").Returning("id").Exec(f.ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}).Exec(f.ctx)
	require.NoError(f.t, err)
	file := &models.File{
		LibraryID: lib.ID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain,
		Filepath: "/books/" + title + "/book." + fileType, FilesizeBytes: 1,
	}
	_, err = f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	for _, value := range identifiers {
		_, err = f.db.NewInsert().Model(&models.FileIdentifier{
			FileID: file.ID, Type: models.IdentifierTypeISBN13, Value: value, Source: models.DataSourceFilepath,
		}).Exec(f.ctx)
		require.NoError(f.t, err)
	}
	require.NoError(f.t, search.NewService(f.db).ReindexBookByID(f.ctx, book.ID))
	return file
}

func (f *syncFixture) addLibrary(name string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(lib).Exec(f.ctx)
	require.NoError(f.t, err)
	return lib
}

// heldOldCopy sends a day of reading in oldCopy, which no server
// recognizes: pages, a finish, four stars, and where the reader stopped.
func (f *syncFixture) heldOldCopy() {
	f.t.Helper()
	at := f.day1.Add(30 * time.Minute)
	f.upload(UploadPayload{
		Pages: pages(oldCopy, 1, 30, f.day1),
		Events: []EventPayload{
			{ID: "old-position", Type: PluginEventPosition, Document: oldCopy, At: at.Unix(), Progress: xpointerA, Percentage: ptr(0.3)},
			{ID: "old-status", Type: PluginEventStatus, Document: oldCopy, At: at.Unix(), Status: KOReaderStatusFinished},
			{ID: "old-rating", Type: PluginEventRating, Document: oldCopy, At: at.Unix(), Rating: 4},
		},
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	})
}

// heldRows is how many page rows are held for oldCopy.
func (f *syncFixture) heldRows() int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*models.KOReaderPageRow)(nil)).
		Where("document = ? AND file_id IS NULL AND NOT grouped", oldCopy).Count(f.ctx)
	require.NoError(f.t, err)
	return int(n)
}

// heldEvents is how many events are held for oldCopy.
func (f *syncFixture) heldEvents() int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*models.KOReaderHeldEvent)(nil)).Where("document = ?", oldCopy).Count(f.ctx)
	require.NoError(f.t, err)
	return int(n)
}

func TestUnrecognized_DataIsHeldAndAcknowledged(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	f.heldOldCopy()

	assert.Empty(t, f.sessions(f.user), "held data counts toward nothing")
	assert.Equal(t, 30, f.heldRows())
	assert.Equal(t, 3, f.heldEvents(), "positions, statuses, and Ratings are held too")
	assert.Equal(t, DocumentHeld, f.lookupAs(f.device, oldCopy)[oldCopy])

	books := f.listUnrecognized()
	require.Len(t, books, 1)
	book := books[0]
	assert.Equal(t, oldCopy, book.Document)
	require.NotNil(t, book.Title)
	assert.Equal(t, "The Left Hand of Darkness", *book.Title)
	assert.Equal(t, []string{"Ursula K. Le Guin"}, book.Authors)
	assert.Equal(t, 7200, book.ReadingSeconds)
	assert.Nil(t, book.LinkedAt)
}

func TestLookup_SaysWhatTheServerKnows(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()

	states := f.lookupAs(f.device, syncHash, oldCopy, "ffffffffffffffffffffffffffffffff")

	assert.Equal(t, map[string]string{
		syncHash: DocumentRecognized, oldCopy: DocumentHeld, "ffffffffffffffffffffffffffffffff": DocumentUnknown,
	}, states)
}

func TestLink_AppliesHeldDataAtOnce(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)

	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	sessions := f.sessionsOf(file)
	require.Len(t, sessions, 1, "its reading appears without a sync")
	assert.True(t, sessions[0].Measured)
	rts := f.readThroughs(file)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Equal(t, ptr(16), f.ratingUnits(file))

	stored, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.True(t, stored.IsZero(), "a linked copy keeps a percentage only")
	assert.InDelta(t, 0.3, stored.Percentage, 1e-9)

	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	assert.Equal(t, DocumentRecognized, f.lookupAs(f.device, oldCopy)[oldCopy])
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	require.NotNil(t, books[0].LinkedAt)
	require.NotNil(t, books[0].Linked)
	assert.Equal(t, file.ID, books[0].Linked.FileID)
}

func TestLink_AppliesHeldHistoryByHistorysRules(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	start := f.day1.AddDate(-1, 0, 0)
	f.backfill(UploadPayload{Pages: append(pages(oldCopy, 1, 10, start), pages(oldCopy, 11, 99, start.AddDate(0, 0, 5))...)})
	f.backfill(UploadPayload{
		Events:       []EventPayload{historyStatus("old-finished", oldCopy, KOReaderStatusFinished, day(start.AddDate(0, 0, 10)))},
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	})
	// Looking something up since sign-in, after history.
	f.upload(UploadPayload{Pages: pages(oldCopy, 1, 5, f.day1)})
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)

	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	rts := f.readThroughs(file)
	require.Len(t, rts, 1, "history's pages start nothing, and later ones leave its finish alone")
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	require.NotNil(t, rts[0].BeginDay)
	assert.Equal(t, day(start), *rts[0].BeginDay, "history begins on its first session's day")
	require.NotNil(t, rts[0].EndDay)
	assert.Equal(t, day(start.AddDate(0, 0, 10)), *rts[0].EndDay)
	assert.Len(t, f.sessionsOf(file), 3)
}

func TestDismiss_DeletesHeldDataAndDropsMore(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()

	require.NoError(t, f.dismissAs(f.user, f.device))

	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	assert.Empty(t, f.listUnrecognized())
	assert.Equal(t, DocumentDismissed, f.lookupAs(f.device, oldCopy)[oldCopy])

	// It stays gone after the next sync.
	f.upload(UploadPayload{
		Pages:        pages(oldCopy, 31, 40, f.day1.Add(time.Hour)),
		Events:       []EventPayload{{ID: "later", Type: PluginEventRating, Document: oldCopy, At: f.day1.Unix(), Rating: 2}},
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	})
	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	assert.Empty(t, f.listUnrecognized())
}

func TestClaimed_DropsWhatWasHeld(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()

	resp := f.upload(UploadPayload{Claimed: []string{oldCopy}})

	assert.NotNil(t, resp)
	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	assert.Empty(t, f.listUnrecognized())
	assert.Equal(t, DocumentUnknown, f.lookupAs(f.device, oldCopy)[oldCopy])
}

func TestClaimed_LeavesACopyLinkedHereAlone(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	f.upload(UploadPayload{Claimed: []string{oldCopy}})

	assert.Len(t, f.listUnrecognized(), 1)
	assert.Len(t, f.sessionsOf(file), 1)
}

func TestSuggestions_IdentifiersFirstThenTitleAndAuthor(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	byTitle := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	byIdentifier := f.addBook(f.lib, "Left Hand, Annotated", "Someone Else", models.FileTypeEPUB, "9780441478125")
	f.addBook(f.lib, "Unrelated", "Nobody", models.FileTypeEPUB)
	f.heldOldCopy()

	books := f.listUnrecognized()

	require.Len(t, books, 1)
	var ids []int
	for _, s := range books[0].Suggestions {
		ids = append(ids, s.FileID)
	}
	assert.Equal(t, []int{byIdentifier.ID, byTitle.ID}, ids)
	assert.Equal(t, LinkMatchIdentifier, books[0].Suggestions[0].Match)
	assert.Equal(t, []string{"Ursula K. Le Guin"}, books[0].Suggestions[1].Authors)
}

func TestSuggestions_LimitedToReachableLibrariesAndTheDevicesScope(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	reader := f.addUser("limited", true)
	mine := f.addLibrary("Mine")
	scoped := f.addLibrary("Scoped")
	other := f.addLibrary("Not mine")
	for _, lib := range []*models.Library{mine, scoped} {
		_, err := f.db.NewInsert().Model(&models.UserLibraryAccess{UserID: reader.ID, LibraryID: &lib.ID}).Exec(f.ctx)
		require.NoError(t, err)
	}
	device := f.addDevice(reader, devices.DeviceTypeKOReader)
	f.backdate(device, time.Now().AddDate(0, -1, 0))
	_, err := f.db.NewUpdate().Model(device).Set("scope_type = ?", devices.ScopeLibrary).Set("scope_library_id = ?", scoped.ID).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	inScope := f.addBook(scoped, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	outOfScope := f.addBook(mine, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB, "9780441478125")
	f.addBook(other, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB, "9780441478125")
	f.addBook(scoped, "The Left Hand of Darkness (audio)", "Ursula K. Le Guin", models.FileTypeM4B)
	rec := f.uploadAs(device, UploadPayload{Pages: pages(oldCopy, 1, 5, f.day1), Unrecognized: []UnrecognizedPayload{unrecognizedAbout()}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	books, err := f.listUnrecognizedAs(reader, device)
	require.NoError(t, err)

	require.Len(t, books, 1)
	require.Len(t, books[0].Suggestions, 1, "only the Device's library, and never an audiobook")
	assert.Equal(t, inScope.ID, books[0].Suggestions[0].FileID)
	assert.Equal(t, "Scoped", books[0].Suggestions[0].LibraryName)

	// Nor can a search or a link reach past it.
	rec, err = f.web(f.h.linkCandidates, http.MethodGet, "/?q=darkness", reader, "", "id", device.ID)
	require.NoError(t, err)
	var found []LinkCandidate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &found))
	require.Len(t, found, 1)
	assert.Equal(t, inScope.ID, found[0].FileID)
	err = f.linkAs(reader, device, outOfScope.ID)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
}

func TestSearch_FindsFilesToLinkTo(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	wanted := f.addBook(f.lib, "A Wizard of Earthsea", "Ursula K. Le Guin", models.FileTypeEPUB)
	f.addBook(f.lib, "Unrelated", "Nobody", models.FileTypeEPUB)

	candidates := f.searchCandidates("earthsea")

	require.Len(t, candidates, 1)
	assert.Equal(t, wanted.ID, candidates[0].FileID)
	assert.Equal(t, "A Wizard of Earthsea", candidates[0].Title)
	assert.Equal(t, models.FileTypeEPUB, candidates[0].FileType)
}

func TestLinkedCopy_SyncsPercentageOnlyPositions(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	at := f.day1.Add(2 * time.Hour)
	f.upload(UploadPayload{Events: []EventPayload{
		{ID: "after-link", Type: PluginEventPosition, Document: oldCopy, At: at.Unix(), Progress: xpointerA, Percentage: ptr(0.5)},
	}})

	stored, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.True(t, stored.IsZero(), "the copy's XPointer may not line up with the File")
	assert.InDelta(t, 0.5, stored.Percentage, 1e-9)

	// Another Device gets it by percentage.
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	require.NoError(t, f.hashes.RecordDocumentHash(f.ctx, file.ID, models.FileTypeEPUB, "abcdabcdabcdabcdabcdabcdabcdabcd"))
	pulled := f.pullAs(other, time.Now().Unix(), "abcdabcdabcdabcdabcdabcdabcdabcd")
	require.Len(t, pulled.Positions, 1)
	assert.Empty(t, pulled.Positions[0].Progress)
	assert.InDelta(t, 0.5, pulled.Positions[0].Percentage, 1e-9)
	assert.Equal(t, f.device.Name, pulled.Positions[0].Source)

	// And the linked copy gets its own back by percentage too.
	own := f.pullAs(f.device, time.Now().Unix(), oldCopy)
	require.Len(t, own.Positions, 1)
	assert.Empty(t, own.Positions[0].Progress)
	assert.True(t, own.Positions[0].Own)
}

func TestLink_ThePullAfterwardCarriesTheBooksState(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	cursor := f.pullStates(0).Cursor
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	state := f.state(cursor, oldCopy)

	require.NotNil(t, state, "the linked copy is reported since the last pull")
	assert.Equal(t, KOReaderStatusFinished, state.Status)
}

func TestUnrecognized_AnotherUsersAreNeverVisible(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	other := f.addUser("other", true)
	f.grantAll(other)
	otherDevice := f.addDevice(other, devices.DeviceTypeKOReader)
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)

	_, err := f.listUnrecognizedAs(other, f.device)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)

	books, err := f.listUnrecognizedAs(other, otherDevice)
	require.NoError(t, err)
	assert.Empty(t, books)

	for _, err := range []error{
		f.linkAs(other, f.device, file.ID),
		f.linkAs(other, otherDevice, file.ID),
		f.dismissAs(other, f.device),
		f.dismissAs(other, otherDevice),
	} {
		require.ErrorAs(t, err, &codeErr)
		assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
	}
	assert.Equal(t, DocumentUnknown, f.lookupAs(otherDevice, oldCopy)[oldCopy])
	assert.Equal(t, 30, f.heldRows())
}

func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, status, codeErr.HTTPCode)
}

func TestLink_IsTheUsersOwn(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	other := f.addUser("other", true)
	f.grantAll(other)
	otherDevice := f.addDevice(other, devices.DeviceTypeKOReader)
	f.backdate(otherDevice, time.Now().AddDate(0, -1, 0))

	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	// The same bytes on another user's Device stay theirs to link.
	assert.Equal(t, DocumentUnknown, f.lookupAs(otherDevice, oldCopy)[oldCopy])
	rec := f.uploadAs(otherDevice, UploadPayload{Pages: pages(oldCopy, 1, 5, f.day1)})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, f.sessions(other))
	books, err := f.listUnrecognizedAs(other, otherDevice)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Nil(t, books[0].LinkedAt)
}

func TestLink_AFileGoneSinceIsLinkedAgain(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	gone := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, gone.ID))
	_, err := f.db.NewDelete().Model((*models.File)(nil)).Where("id = ?", gone.ID).Exec(f.ctx)
	require.NoError(t, err)

	// Its reading since is held again, and it's listed to link.
	assert.Equal(t, DocumentHeld, f.lookupAs(f.device, oldCopy)[oldCopy])
	f.upload(UploadPayload{Pages: pages(oldCopy, 50, 52, f.day1.Add(5*time.Hour))})
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.Nil(t, books[0].LinkedAt)
	assert.Equal(t, 3, f.heldRows())

	again := f.addBook(f.lib, "The Left Hand of Darkness, again", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, again.ID))
	assert.Len(t, f.sessionsOf(again), 1)
	assert.Equal(t, DocumentRecognized, f.lookupAs(f.device, oldCopy)[oldCopy])
}

func TestLink_LeavesAnotherDeviceThatDoesntReachTheFileToLink(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	elsewhere := f.addLibrary("Elsewhere")
	scoped := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(scoped, time.Now().AddDate(0, -1, 0))
	_, err := f.db.NewUpdate().Model(scoped).Set("scope_type = ?", devices.ScopeLibrary).Set("scope_library_id = ?", elsewhere.ID).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	rec := f.uploadAs(scoped, UploadPayload{Pages: pages(oldCopy, 60, 62, f.day1.Add(6*time.Hour))})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	rec = f.uploadAs(other, UploadPayload{Pages: pages(oldCopy, 70, 72, f.day1.Add(7*time.Hour))})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)

	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	assert.Len(t, f.sessionsOf(file), 2, "the linking Device's reading and the other's that reaches the File")
	books, err := f.listUnrecognizedAs(f.user, scoped)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Nil(t, books[0].LinkedAt, "a Device scoped away from the File keeps its copy to link")
	assert.Equal(t, DocumentHeld, f.lookupAs(scoped, oldCopy)[oldCopy])
}

func TestLink_RefusesWhatCantBeLinked(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	audiobook := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeM4B)
	kobo := f.addDevice(f.user, devices.DeviceTypeKobo)

	requireStatus(t, f.linkAs(f.user, f.device, audiobook.ID), http.StatusUnprocessableEntity)
	requireStatus(t, f.linkAs(f.user, f.device, 999999), http.StatusNotFound)
	requireStatus(t, f.linkAs(f.user, kobo, audiobook.ID), http.StatusUnprocessableEntity)
	_, err := f.listUnrecognizedAs(f.user, kobo)
	requireStatus(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, 30, f.heldRows(), "nothing was applied")
}

func TestClaimed_KeepsADismissal(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))

	f.upload(UploadPayload{Claimed: []string{oldCopy}})

	assert.Equal(t, DocumentDismissed, f.lookupAs(f.device, oldCopy)[oldCopy],
		"dismissed here even if the other server stops recognizing it")
}

func TestUnrecognized_ALookupFailureIsAServerError(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	_, err := f.db.ExecContext(f.ctx, `DROP TABLE koreader_unrecognized_books`)
	require.NoError(t, err)

	for _, err := range []error{
		f.linkAs(f.user, f.device, file.ID),
		f.dismissAs(f.user, f.device),
	} {
		require.Error(t, err)
		var codeErr *errcodes.Error
		assert.NotErrorAs(t, err, &codeErr, "a failed lookup isn't a 4xx")
	}
}

func TestLink_AFileLookupFailureIsAServerError(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	_, err := f.db.ExecContext(f.ctx, `DROP TABLE files`)
	require.NoError(t, err)

	err = f.linkAs(f.user, f.device, 1)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "a failed lookup isn't a 4xx")
}
