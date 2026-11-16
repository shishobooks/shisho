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

func (f *syncFixture) restoreAs(user *models.User, device *devices.Device) error {
	f.t.Helper()
	_, err := f.web(f.h.restoreUnrecognized, http.MethodPost, "/", user, "", "id", device.ID, "document", oldCopy)
	return err
}

func (f *syncFixture) deleteReadingAs(user *models.User, device *devices.Device) error {
	f.t.Helper()
	_, err := f.web(f.h.deleteUnrecognizedReading, http.MethodPost, "/", user, "", "id", device.ID, "document", oldCopy)
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

func TestDismiss_SetsTheCopyAsideAndKeepsHoldingItsReading(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()

	require.NoError(t, f.dismissAs(f.user, f.device))

	assert.Equal(t, 30, f.heldRows())
	assert.Equal(t, 3, f.heldEvents())
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.NotNil(t, books[0].DismissedAt)
	assert.False(t, books[0].ReadingDeleted)
	assert.Empty(t, books[0].Suggestions, "a dismissed copy isn't matched")
	assert.Equal(t, DocumentHeld, f.lookupAs(f.device, oldCopy)[oldCopy])

	// Its later reading is held too.
	f.upload(UploadPayload{
		Pages:        pages(oldCopy, 31, 40, f.day1.Add(time.Hour)),
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	})
	assert.Equal(t, 40, f.heldRows())
	requireStatus(t, f.dismissAs(f.user, f.device), http.StatusNotFound)
}

func TestDismiss_ADismissedCopyLinksWithEverythingHeld(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.dismissAs(f.user, f.device))

	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	assert.Zero(t, f.heldRows())
	assert.Len(t, f.sessionsOf(file), 1)
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.NotNil(t, books[0].LinkedAt)
	assert.Nil(t, books[0].DismissedAt, "linking takes it out of the dismissed ones")
}

func TestRestore_PutsADismissedCopyBackToLink(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	requireStatus(t, f.restoreAs(f.user, f.device), http.StatusNotFound)
	require.NoError(t, f.dismissAs(f.user, f.device))

	require.NoError(t, f.restoreAs(f.user, f.device))

	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.Nil(t, books[0].DismissedAt)
	assert.NotEmpty(t, books[0].Suggestions)
	assert.Equal(t, 30, f.heldRows())
}

func TestDeleteReading_DeletesHeldDataAndDropsMore(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))

	require.NoError(t, f.deleteReadingAs(f.user, f.device))

	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	books := f.listUnrecognized()
	require.Len(t, books, 1, "it stays listed with the dismissed ones")
	assert.NotNil(t, books[0].DismissedAt)
	assert.True(t, books[0].ReadingDeleted)
	assert.Equal(t, DocumentDismissed, f.lookupAs(f.device, oldCopy)[oldCopy])

	// It stays gone after the next sync.
	f.upload(UploadPayload{
		Pages:        pages(oldCopy, 31, 40, f.day1.Add(time.Hour)),
		Events:       []EventPayload{{ID: "later", Type: PluginEventRating, Document: oldCopy, At: f.day1.Unix(), Rating: 2}},
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	})
	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	requireStatus(t, f.linkAs(f.user, f.device, file.ID), http.StatusNotFound)
	requireStatus(t, f.deleteReadingAs(f.user, f.device), http.StatusNotFound)
}

func TestDeleteReading_OnlyADismissedCopy(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()

	requireStatus(t, f.deleteReadingAs(f.user, f.device), http.StatusNotFound)

	assert.Equal(t, 30, f.heldRows(), "a copy still to link keeps its reading history")
}

func TestRestore_ADeletedCopyHoldsItsReadingWhenSentAgain(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))
	require.NoError(t, f.deleteReadingAs(f.user, f.device))

	require.NoError(t, f.restoreAs(f.user, f.device))
	assert.Equal(t, DocumentHeld, f.lookupAs(f.device, oldCopy)[oldCopy])
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.Nil(t, books[0].DismissedAt)
	assert.False(t, books[0].ReadingDeleted)

	// The plugin sends its reading history again, which holds the rows
	// that were deleted, and its status, as history.
	again := UploadPayload{
		Backfill: true,
		Pages:    pages(oldCopy, 1, 30, f.day1),
		Events: []EventPayload{
			{ID: "again-status", Type: PluginEventStatus, Document: oldCopy, Day: day(f.day1), Status: KOReaderStatusFinished},
		},
		Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
	}
	f.upload(again)
	assert.Equal(t, 30, f.heldRows())
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))
	assert.Len(t, f.sessionsOf(file), 1)
	assert.Equal(t, models.ReadingStatusRead, f.status(file))

	// Sending it yet again adds nothing, and the link stays.
	again.Events[0].ID = "again-status-2"
	f.upload(again)
	assert.Zero(t, f.heldRows())
	assert.Len(t, f.sessionsOf(file), 1)
	assert.Equal(t, DocumentRecognized, f.lookupAs(f.device, oldCopy)[oldCopy])
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

func TestLinkedCopy_SyncsSessionsStatusAndRatingLikeAnyBook(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// Only pages are held, so the status and Rating can come only from later.
	f.upload(UploadPayload{Pages: pages(oldCopy, 1, 30, f.day1), Unrecognized: []UnrecognizedPayload{unrecognizedAbout()}})
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))
	require.Len(t, f.sessionsOf(file), 1)

	later := f.day1.Add(2 * time.Hour)
	f.upload(UploadPayload{
		Pages: pages(oldCopy, 31, 40, later),
		Events: []EventPayload{
			{ID: "later-status", Type: PluginEventStatus, Document: oldCopy, At: later.Add(time.Hour).Unix(), Status: KOReaderStatusFinished},
			{ID: "later-rating", Type: PluginEventRating, Document: oldCopy, At: later.Add(time.Hour).Unix(), Rating: 3},
		},
	})

	rts := f.readThroughs(file)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Equal(t, models.ReadingStatusRead, f.status(file))
	assert.Equal(t, ptr(12), f.ratingUnits(file))
	sessions := f.sessionsOf(file)
	require.Len(t, sessions, 2)
	measured := sessions[1]
	assert.True(t, measured.Measured)
	assert.True(t, measured.StartedAt.Equal(later))
	assert.Equal(t, 10*60, measured.Duration)
	require.NotNil(t, measured.ReadThroughID)
	assert.Equal(t, rts[0].ID, *measured.ReadThroughID)
	assert.Zero(t, f.heldRows())
	assert.Zero(t, f.heldEvents())
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
		f.restoreAs(other, f.device),
		f.deleteReadingAs(other, f.device),
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

func TestClaimed_KeepsADeletion(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))
	require.NoError(t, f.deleteReadingAs(f.user, f.device))

	f.upload(UploadPayload{Claimed: []string{oldCopy}})

	assert.Equal(t, DocumentDismissed, f.lookupAs(f.device, oldCopy)[oldCopy],
		"dropped here even if the other server stops recognizing it")
}

func TestClaimed_ForgetsADismissedCopy(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))

	f.upload(UploadPayload{Claimed: []string{oldCopy}})

	assert.Zero(t, f.heldRows())
	assert.Empty(t, f.listUnrecognized())
	assert.Equal(t, DocumentUnknown, f.lookupAs(f.device, oldCopy)[oldCopy])
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
		f.unlinkAs(f.user, f.device),
		f.dismissAs(f.user, f.device),
		f.restoreAs(f.user, f.device),
		f.deleteReadingAs(f.user, f.device),
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

func (f *syncFixture) unlinkAs(user *models.User, device *devices.Device) error {
	f.t.Helper()
	_, err := f.web(f.h.unlinkUnrecognized, http.MethodPost, "/", user, "", "id", device.ID, "document", oldCopy)
	return err
}

func TestUnlink_StopsSyncingAndHoldsWhatComesNext(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	wrong := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, wrong.ID))

	require.NoError(t, f.unlinkAs(f.user, f.device))

	// What the link applied stays, to clean up by hand.
	assert.Len(t, f.sessionsOf(wrong), 1)
	assert.Equal(t, DocumentHeld, f.lookupAs(f.device, oldCopy)[oldCopy])
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	assert.Nil(t, books[0].LinkedAt)
	assert.NotEmpty(t, books[0].Suggestions)

	// Reading from then on is held, not applied to the File.
	f.upload(UploadPayload{Pages: pages(oldCopy, 80, 82, f.day1.Add(8*time.Hour))})
	assert.Len(t, f.sessionsOf(wrong), 1)
	assert.Equal(t, 3, f.heldRows())

	// And it can be linked again.
	right := f.addBook(f.lib, "The Left Hand of Darkness, annotated", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, right.ID))
	assert.Len(t, f.sessionsOf(right), 1)
}

func TestUnlink_UnlinksTheCopyOnEveryOneOfTheUsersDevices(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	rec := f.uploadAs(other, UploadPayload{Pages: pages(oldCopy, 70, 72, f.day1.Add(7*time.Hour))})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))

	require.NoError(t, f.unlinkAs(f.user, f.device))

	// The link is the user's, so no Device of theirs resolves the copy now.
	assert.Equal(t, DocumentHeld, f.lookupAs(other, oldCopy)[oldCopy])
	books, err := f.listUnrecognizedAs(f.user, other)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Nil(t, books[0].LinkedAt)
}

func TestUnlink_RefusesACopyThatIsntLinked(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	other := f.addUser("other", true)
	f.grantAll(other)
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)

	requireStatus(t, f.unlinkAs(f.user, f.device), http.StatusNotFound)
	require.NoError(t, f.linkAs(f.user, f.device, file.ID))
	requireStatus(t, f.unlinkAs(other, f.device), http.StatusNotFound)
	assert.Equal(t, DocumentRecognized, f.lookupAs(f.device, oldCopy)[oldCopy])
}

// undescribedNow is what a lookup about nothing says the Device's plugin
// should describe, sent as the plugin sends it, with no list.
func (f *syncFixture) undescribedNow() []string {
	f.t.Helper()
	rec := f.do(request{path: "/koreader/plugin/lookup", body: `{}`, username: f.user.Username, credential: f.device.Credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp LookupResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Undescribed
}

func TestLookup_AsksForTheDetailsOfACopyHeldWithoutThem(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// A rating alone, from a plugin that sent nothing of the book.
	f.upload(UploadPayload{Events: []EventPayload{
		{ID: "bare", Type: PluginEventRating, Document: oldCopy, Day: day(f.day1), Rating: 2},
	}})
	require.Len(t, f.listUnrecognized(), 1)

	// Asked about or not.
	assert.Equal(t, []string{oldCopy}, f.undescribedNow())

	// The details alone, with nothing else of the book, fill it in.
	f.upload(UploadPayload{Unrecognized: []UnrecognizedPayload{unrecognizedAbout()}})
	books := f.listUnrecognized()
	require.Len(t, books, 1)
	require.NotNil(t, books[0].Title)
	assert.Equal(t, "The Left Hand of Darkness", *books[0].Title)
	assert.Equal(t, 7200, books[0].ReadingSeconds)
	assert.Empty(t, f.undescribedNow())
}

func TestUpload_DetailsAloneListNothingNew(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	require.NoError(t, f.dismissAs(f.user, f.device))
	require.NoError(t, f.deleteReadingAs(f.user, f.device))
	other := unrecognizedAbout()
	other.Document = "ffffffffffffffffffffffffffffffff"

	f.upload(UploadPayload{Unrecognized: []UnrecognizedPayload{other, unrecognizedAbout()}})

	books := f.listUnrecognized()
	require.Len(t, books, 1, "a copy with no data isn't listed")
	assert.True(t, books[0].ReadingDeleted)
	assert.Equal(t, DocumentUnknown, f.lookupAs(f.device, other.Document)[other.Document])
}

// dismissAllAs dismisses documents, the books still to link the Device's
// page listed.
func (f *syncFixture) dismissAllAs(user *models.User, device *devices.Device, documents ...string) error {
	f.t.Helper()
	body, err := json.Marshal(DismissAllPayload{Documents: documents})
	require.NoError(f.t, err)
	_, err = f.web(f.h.dismissAllUnrecognized, http.MethodPost, "/", user, string(body), "id", device.ID)
	return err
}

func TestDismissAll_DismissesEveryBookStillToLink(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	another := unrecognizedAbout()
	another.Document = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	f.upload(UploadPayload{Pages: pages(another.Document, 1, 3, f.day1), Unrecognized: []UnrecognizedPayload{another}})
	linked := unrecognizedAbout()
	linked.Document = "dddddddddddddddddddddddddddddddd"
	f.upload(UploadPayload{Pages: pages(linked.Document, 1, 3, f.day1.Add(time.Hour)), Unrecognized: []UnrecognizedPayload{linked}})
	file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
	body, err := json.Marshal(LinkPayload{FileID: file.ID})
	require.NoError(t, err)
	_, err = f.web(f.h.linkUnrecognized, http.MethodPost, "/", f.user, string(body), "id", f.device.ID, "document", linked.Document)
	require.NoError(t, err)
	// Another Device's books are its own.
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	rec := f.uploadAs(other, UploadPayload{Pages: pages(oldCopy, 50, 52, f.day1), Unrecognized: []UnrecognizedPayload{unrecognizedAbout()}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// A book that arrived after the page listed them isn't one it showed.
	later := unrecognizedAbout()
	later.Document = "cccccccccccccccccccccccccccccccc"
	f.upload(UploadPayload{Pages: pages(later.Document, 1, 3, f.day1.Add(2*time.Hour)), Unrecognized: []UnrecognizedPayload{later}})

	require.NoError(t, f.dismissAllAs(f.user, f.device, oldCopy, another.Document, linked.Document))

	books := f.listUnrecognized()
	require.Len(t, books, 4)
	assert.Equal(t, later.Document, books[0].Document, "the one that arrived since is still to link")
	assert.Nil(t, books[0].DismissedAt)
	assert.Equal(t, linked.Document, books[1].Document, "the linked book stays linked")
	assert.Nil(t, books[1].DismissedAt)
	for _, book := range books[2:] {
		assert.NotNil(t, book.DismissedAt, book.Document)
		assert.False(t, book.ReadingDeleted, book.Document)
	}
	states := f.lookupAs(f.device, oldCopy, another.Document)
	assert.Equal(t, DocumentHeld, states[oldCopy])
	assert.Equal(t, DocumentHeld, states[another.Document])
	held, err := f.db.NewSelect().Model((*models.KOReaderPageRow)(nil)).
		Where("device_id = ? AND file_id IS NULL AND document != ?", f.device.ID, later.Document).Count(f.ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 33, held, "what was held for them stays held")
	assert.Equal(t, 3, f.heldEventsOn(f.device))
	otherBooks, err := f.listUnrecognizedAs(f.user, other)
	require.NoError(t, err)
	assert.Len(t, otherBooks, 1)
}

func TestDismissAll_RefusesAnotherUsersDevice(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.heldOldCopy()
	other := f.addUser("other", true)

	requireStatus(t, f.dismissAllAs(other, f.device, oldCopy), http.StatusNotFound)
	assert.Len(t, f.listUnrecognized(), 1)
}

func (f *syncFixture) heldEventsOn(device *devices.Device) int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*models.KOReaderHeldEvent)(nil)).Where("device_id = ?", device.ID).Count(f.ctx)
	require.NoError(f.t, err)
	return int(n)
}

func TestLookup_AsksOnlyForTheDetailsOfThisDevicesCopiesStillToLink(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	bare := func(document string) {
		f.upload(UploadPayload{Events: []EventPayload{
			{ID: "bare-" + document, Type: PluginEventRating, Document: document, Day: day(f.day1), Rating: 2},
		}})
	}
	bare(oldCopy)
	deleted := "dddddddddddddddddddddddddddddddd"
	bare(deleted)
	_, err := f.web(f.h.dismissUnrecognized, http.MethodPost, "/", f.user, "", "id", f.device.ID, "document", deleted)
	require.NoError(t, err)
	_, err = f.web(f.h.deleteUnrecognizedReading, http.MethodPost, "/", f.user, "", "id", f.device.ID, "document", deleted)
	require.NoError(t, err)
	// A dismissed copy is still held, and listed.
	dismissed := "cccccccccccccccccccccccccccccccc"
	bare(dismissed)
	_, err = f.web(f.h.dismissUnrecognized, http.MethodPost, "/", f.user, "", "id", f.device.ID, "document", dismissed)
	require.NoError(t, err)
	// A bare copy that resolves now, as one Shisho has produced since.
	produced := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	bare(produced)
	require.NoError(t, f.hashes.RecordDocumentHash(f.ctx, f.file.ID, models.FileTypeEPUB, produced))
	// Another Device's bare copy is its own.
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	rec := f.uploadAs(other, UploadPayload{Events: []EventPayload{
		{ID: "other-bare", Type: PluginEventRating, Document: "ffffffffffffffffffffffffffffffff", Day: day(f.day1), Rating: 2},
	}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	assert.Equal(t, []string{oldCopy, dismissed}, f.undescribedNow())

	// Details for a copy that resolves change nothing of it.
	about := unrecognizedAbout()
	about.Document = produced
	f.upload(UploadPayload{Unrecognized: []UnrecognizedPayload{about}})
	record, err := f.h.unrecognizedBook(f.ctx, f.device.ID, produced)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Nil(t, record.Title)
}

func TestPull_CountsTheCopiesStillToLink(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	assert.Zero(t, f.pullStates(0).Unlinked)
	f.heldOldCopy()
	for _, document := range []string{"cccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddd"} {
		about := unrecognizedAbout()
		about.Document = document
		f.upload(UploadPayload{Pages: pages(document, 1, 3, f.day1), Unrecognized: []UnrecognizedPayload{about}})
	}
	// Another Device's copies are its own.
	other := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(other, time.Now().AddDate(0, -1, 0))
	rec := f.uploadAs(other, UploadPayload{Pages: pages("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", 1, 3, f.day1)})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 3, f.pullStates(0).Unlinked)

	require.NoError(t, f.dismissAs(f.user, f.device))
	for _, handler := range []echo.HandlerFunc{f.h.dismissUnrecognized, f.h.deleteUnrecognizedReading} {
		_, err := f.web(handler, http.MethodPost, "/", f.user, "", "id", f.device.ID, "document", "cccccccccccccccccccccccccccccccc")
		require.NoError(t, err)
	}
	file := f.addBook(f.lib, "Linked", "Someone", models.FileTypeEPUB)
	body, err := json.Marshal(LinkPayload{FileID: file.ID})
	require.NoError(t, err)
	_, err = f.web(f.h.linkUnrecognized, http.MethodPost, "/", f.user, string(body), "id", f.device.ID, "document", "dddddddddddddddddddddddddddddddd")
	require.NoError(t, err)

	assert.Zero(t, f.pullStates(0).Unlinked, "dismissed, deleted, and linked copies don't count")
}

// History sent again carries the status of every copy the server holds, but
// a copy that already holds a status keeps it: a status as history would be
// applied ahead of the reading it already holds when linked, and start the
// read on the wrong day.
func TestUnrecognized_HistorySentAgainKeepsAHeldCopysOwnStatusAndRating(t *testing.T) {
	t.Parallel()
	outcome := func(sendAgain bool) (string, int, []*int) {
		f := newSyncFixture(t)
		day2 := f.day1.Add(24 * time.Hour)
		live := UploadPayload{
			Pages: append(pages(oldCopy, 1, 30, f.day1), pages(oldCopy, 31, 60, day2)...),
			Events: []EventPayload{
				{ID: "s1", Type: PluginEventStatus, Document: oldCopy, At: f.day1.Unix(), Status: KOReaderStatusReading},
				{ID: "s2", Type: PluginEventStatus, Document: oldCopy, At: day2.Add(time.Hour).Unix(), Status: KOReaderStatusFinished, PreviousStatus: KOReaderStatusReading},
				{ID: "r1", Type: PluginEventRating, Document: oldCopy, At: day2.Add(time.Hour).Unix(), Rating: 4},
			},
			Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
		}
		f.upload(live)
		if sendAgain {
			f.upload(UploadPayload{
				Backfill: true,
				Pages:    live.Pages,
				Events: []EventPayload{
					{ID: "again-status", Type: PluginEventStatus, Document: oldCopy, Day: day(day2), Status: KOReaderStatusFinished},
					{ID: "again-rating", Type: PluginEventRating, Document: oldCopy, Day: day(day2), Rating: 2},
				},
				Unrecognized: []UnrecognizedPayload{unrecognizedAbout()},
			})
		}
		file := f.addBook(f.lib, "The Left Hand of Darkness", "Ursula K. Le Guin", models.FileTypeEPUB)
		require.NoError(t, f.linkAs(f.user, f.device, file.ID))
		rts := f.readThroughs(file)
		require.Len(t, rts, 1)
		require.NotNil(t, rts[0].BeginDay)
		var sessionRTs []*int
		for _, s := range f.sessionsOf(file) {
			sessionRTs = append(sessionRTs, s.ReadThroughID)
		}
		rating := 0
		if units := f.ratingUnits(file); units != nil {
			rating = *units
		}
		return *rts[0].BeginDay, rating, sessionRTs
	}

	begin, rating, sessions := outcome(false)
	againBegin, againRating, againSessions := outcome(true)

	assert.Equal(t, begin, againBegin, "the read begins on the day reading began")
	assert.Equal(t, rating, againRating, "the copy's own stars stay")
	require.Len(t, againSessions, len(sessions))
	for _, rt := range againSessions {
		assert.NotNil(t, rt, "every session is on the read")
	}
}
