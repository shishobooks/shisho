package kobo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncItem is one decoded entry of a sync response. Exactly one of the two
// wrappers is set.
type syncItem struct {
	NewEntitlement     *syncItemBody `json:"NewEntitlement"`
	ChangedEntitlement *syncItemBody `json:"ChangedEntitlement"`
}

type syncItemBody struct {
	BookEntitlement map[string]any `json:"BookEntitlement"`
	BookMetadata    map[string]any `json:"BookMetadata"`
}

// sync runs one /v1/library/sync request under the "all" scope with the
// given token and returns the decoded items and the next token.
func (f *koboScopeFixture) sync(t *testing.T, token string) ([]syncItem, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, f.prefix("all")+"/v1/library/sync", nil)
	if token != "" {
		req.Header.Set("X-Kobo-SyncToken", token)
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Empty(t, rec.Header().Get("X-Kobo-Sync"), "fixture syncs fit in one page")
	var items []syncItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	return items, rec.Header().Get("X-Kobo-SyncToken")
}

// itemFor returns the single item whose entitlement id is the given file's.
func itemFor(t *testing.T, items []syncItem, fileID int) syncItem {
	t.Helper()
	var found []syncItem
	for _, item := range items {
		body := item.NewEntitlement
		if body == nil {
			body = item.ChangedEntitlement
		}
		require.NotNil(t, body)
		if body.BookEntitlement["Id"] == ShishoID(fileID) {
			found = append(found, item)
		}
	}
	require.Len(t, found, 1, "expected one item for file %d", fileID)
	return found[0]
}

func entitlementRevisionID(t *testing.T, body *syncItemBody) string {
	t.Helper()
	require.NotNil(t, body)
	rev, ok := body.BookEntitlement["RevisionId"].(string)
	require.True(t, ok, "RevisionId missing from %v", body.BookEntitlement)
	return rev
}

// assertChangedUnder checks that item is a full ChangedEntitlement for the
// file under the given RevisionId, carrying metadata for an in-place update.
func assertChangedUnder(t *testing.T, item syncItem, fileID int, rev string) {
	t.Helper()
	require.Nil(t, item.NewEntitlement, "a change must not go out as a NewEntitlement")
	require.NotNil(t, item.ChangedEntitlement)
	ent := item.ChangedEntitlement.BookEntitlement
	assert.Equal(t, ShishoID(fileID), ent["Id"])
	assert.Equal(t, rev, ent["RevisionId"])
	assert.Equal(t, false, ent["IsRemoved"])
	assert.Equal(t, "Active", ent["Status"])
	require.NotNil(t, item.ChangedEntitlement.BookMetadata)
	assert.NotEmpty(t, item.ChangedEntitlement.BookMetadata["DownloadUrls"])
}

func (f *koboScopeFixture) moveFile(t *testing.T, file *models.File, newPath string, newSize int64) {
	t.Helper()
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).
		Set("filepath = ?", newPath).
		Set("filesize_bytes = ?", newSize).
		Where("id = ?", file.ID).
		Exec(context.Background())
	require.NoError(t, err)
}

func (f *koboScopeFixture) retitleBook(t *testing.T, book *models.Book, title string) {
	t.Helper()
	_, err := f.db.NewUpdate().Model((*models.Book)(nil)).
		Set("title = ?", title).
		Where("id = ?", book.ID).
		Exec(context.Background())
	require.NoError(t, err)
}

func (f *koboScopeFixture) nameFile(t *testing.T, file *models.File, name string) {
	t.Helper()
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).
		Set("name = ?", name).
		Where("id = ?", file.ID).
		Exec(context.Background())
	require.NoError(t, err)
}

// A moved, renamed, or rewritten file comes back as a ChangedEntitlement
// under the ids it was first sent with, so the Kobo updates it in place
// instead of adding a second copy.
func TestHandleSync_PathOrSizeChangeIsChangedEntitlementUnderSameIDs(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, token := f.sync(t, "")
	first := itemFor(t, items, f.fileA.ID)
	require.NotNil(t, first.NewEntitlement)
	rev := entitlementRevisionID(t, first.NewEntitlement)

	f.moveFile(t, f.fileA, strings.Replace(f.fileA.Filepath, "Alpha.epub", "Alpha Renamed.epub", 1), 99)

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], f.fileA.ID, rev)
}

func TestHandleSync_MetadataChangeIsChangedEntitlementUnderSameIDs(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, token := f.sync(t, "")
	rev := entitlementRevisionID(t, itemFor(t, items, f.fileA.ID).NewEntitlement)

	f.retitleBook(t, f.bookA, "Alpha, Revised")

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], f.fileA.ID, rev)
	assert.Equal(t, "Alpha, Revised", items[0].ChangedEntitlement.BookMetadata["Title"])
}

// The Kobo shows a file's name as its title, so changing only the name has
// to reach a Kobo that already synced the file.
func TestHandleSync_FileNameChangeIsChangedEntitlement(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	f.nameFile(t, f.fileA, "")
	items, token := f.sync(t, "")
	first := itemFor(t, items, f.fileA.ID)
	rev := entitlementRevisionID(t, first.NewEntitlement)
	assert.Equal(t, f.bookA.Title, first.NewEntitlement.BookMetadata["Title"], "an empty name falls back to the book's title")

	f.nameFile(t, f.fileA, "Alpha: Special Edition")

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], f.fileA.ID, rev)
	assert.Equal(t, "Alpha: Special Edition", items[0].ChangedEntitlement.BookMetadata["Title"])
}

// A file first synced after the upgrade gets a RevisionId that does not
// come from its path or size, and keeps it through every later change.
func TestHandleSync_NewEntryKeepsOneRevisionIDAcrossChanges(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, token := f.sync(t, "")
	rev := entitlementRevisionID(t, itemFor(t, items, f.fileA.ID).NewEntitlement)
	assert.NotContains(t, rev, computeFileHash(f.fileA)[:8], "a new RevisionId must not depend on the file")

	f.moveFile(t, f.fileA, f.fileA.Filepath+".moved", f.fileA.FilesizeBytes)
	items, token = f.sync(t, token)
	assertChangedUnder(t, itemFor(t, items, f.fileA.ID), f.fileA.ID, rev)

	f.moveFile(t, f.fileA, f.fileA.Filepath+".moved", 1234)
	items, token = f.sync(t, token)
	assertChangedUnder(t, itemFor(t, items, f.fileA.ID), f.fileA.ID, rev)

	f.retitleBook(t, f.bookA, "Alpha Again")
	items, _ = f.sync(t, token)
	assertChangedUnder(t, itemFor(t, items, f.fileA.ID), f.fileA.ID, rev)
}

// Sync state from before the upgrade holds the hash-form RevisionId each
// book went out with, as the migration records it. A later change goes out
// under that same RevisionId.
func TestHandleSync_PreUpgradeEntryKeepsHashFormRevisionID(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()

	hashForm := ShishoID(f.fileA.ID) + "-" + computeFileHash(f.fileA)[:8]
	sp, err := NewService(f.db).CreateSyncPoint(ctx, f.key.ID, []ScopedFile{{
		FileID:       f.fileA.ID,
		FileHash:     computeFileHash(f.fileA),
		FileSize:     f.fileA.FilesizeBytes,
		MetadataHash: "pre-upgrade",
	}})
	require.NoError(t, err)
	require.NoError(t, NewService(f.db).MarkSyncPointCompleted(ctx, f.key.ID, sp.ID))
	_, err = f.db.ExecContext(ctx,
		`INSERT INTO kobo_sync_entries (api_key_id, file_id, revision_id) VALUES (?, ?, ?)`,
		f.key.ID, f.fileA.ID, hashForm)
	require.NoError(t, err)
	token := encodeTestSyncToken(t, SyncToken{LastSyncPointID: sp.ID})

	f.moveFile(t, f.fileA, f.fileA.Filepath+".moved", 4321)

	items, _ := f.sync(t, token)
	assertChangedUnder(t, itemFor(t, items, f.fileA.ID), f.fileA.ID, hashForm)
}

// A fresh sync (no token, as after clearing sync history) still sends every
// book as a NewEntitlement, under the RevisionId recorded for it.
func TestHandleSync_FreshSyncResendsRecordedRevisionID(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, _ := f.sync(t, "")
	require.Len(t, items, 2)
	rev := entitlementRevisionID(t, itemFor(t, items, f.fileA.ID).NewEntitlement)

	f.moveFile(t, f.fileA, f.fileA.Filepath+".moved", 777)

	items, _ = f.sync(t, "")
	require.Len(t, items, 2)
	item := itemFor(t, items, f.fileA.ID)
	require.NotNil(t, item.NewEntitlement, "a fresh sync sends every book as new")
	assert.Equal(t, rev, entitlementRevisionID(t, item.NewEntitlement))
}

// A book that leaves the scope still goes out as a bare removal.
func TestHandleSync_RemovalUnchanged(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	_, token := f.sync(t, "")
	f.restrictToLibrary(t, f.libA.ID)

	items, _ := f.sync(t, token)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].ChangedEntitlement)
	assert.Equal(t, map[string]any{"Id": ShishoID(f.fileB.ID), "IsRemoved": true}, items[0].ChangedEntitlement.BookEntitlement)
	assert.Equal(t, ShishoID(f.fileB.ID), items[0].ChangedEntitlement.BookMetadata["EntitlementId"])
}

func encodeTestSyncToken(t *testing.T, token SyncToken) string {
	t.Helper()
	b, err := json.Marshal(token)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

// A file deleted after the snapshot was taken is dropped from the page it
// falls on, and the rest of the page still goes out.
func TestHandleSync_FileDeletedAfterSnapshotIsDropped(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()

	const deletedFileID = 9999
	sp, err := NewService(f.db).CreateSyncPoint(ctx, f.key.ID, []ScopedFile{
		{FileID: f.fileA.ID, FileHash: computeFileHash(f.fileA), FileSize: f.fileA.FilesizeBytes, MetadataHash: "m"},
		{FileID: deletedFileID, FileHash: "0123456789abcdef", FileSize: 1, MetadataHash: "m"},
	})
	require.NoError(t, err)
	token := encodeTestSyncToken(t, SyncToken{OngoingSyncPointID: sp.ID})

	items, _ := f.sync(t, token)
	require.Len(t, items, 1)
	require.NotNil(t, itemFor(t, items, f.fileA.ID).NewEntitlement)
}

// replaceCover overwrites the file's cover in place, under the same
// filename, the way re-uploading a cover of the same type does.
func (f *koboScopeFixture) replaceCover(t *testing.T, file *models.File) {
	t.Helper()
	path := covers.FileCoverPath(file)
	require.NoError(t, os.WriteFile(path, []byte("a different cover"), 0o600))
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, later, later))
}

// A cover replaced under the same filename goes out as a change, with a new
// CoverImageId so the Kobo refreshes its thumbnail.
func TestHandleSync_CoverReplacedUnderSameFilenameIsChanged(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, token := f.sync(t, "")
	first := itemFor(t, items, f.fileA.ID)
	rev := entitlementRevisionID(t, first.NewEntitlement)
	coverID := first.NewEntitlement.BookMetadata["CoverImageId"]

	f.replaceCover(t, f.fileA)

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], f.fileA.ID, rev)
	assert.NotEqual(t, coverID, items[0].ChangedEntitlement.BookMetadata["CoverImageId"])
}

// Sync points stored before covers were tracked hold no cover time. The
// first sync after the upgrade must not report every book as changed.
func TestHandleSync_SnapshotWithoutCoverTimeIsNotChanged(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()

	_, token := f.sync(t, "")
	_, err := f.db.ExecContext(ctx, `UPDATE kobo_sync_point_books SET cover_modified_at = NULL`)
	require.NoError(t, err)

	items, _ := f.sync(t, token)
	assert.Empty(t, items)
}

// The metadata route builds the same CoverImageId as the sync entry, so the
// device asks for the thumbnail it already cached.
func TestHandleMetadata_CoverImageIDMatchesSync(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.replaceCover(t, f.fileA)

	items, _ := f.sync(t, "")
	synced := itemFor(t, items, f.fileA.ID).NewEntitlement.BookMetadata["CoverImageId"]

	rec := f.serve(http.MethodGet, f.prefix("all")+"/v1/library/"+ShishoID(f.fileA.ID)+"/metadata")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var metadata []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &metadata))
	require.Len(t, metadata, 1)
	assert.Equal(t, synced, metadata[0]["CoverImageId"])
}
