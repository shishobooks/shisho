package kobo

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Kobo sync delivers what the Kobo Device's Delivery Rules select, and keeps
// each book on the Kobo under one entry (ADR 0012).

// removedIDs returns the ids of the removals in items.
func removedIDs(items []syncItem) []string {
	ids := []string{}
	for _, item := range items {
		if item.ChangedEntitlement != nil && item.ChangedEntitlement.BookEntitlement["IsRemoved"] == true {
			ids = append(ids, item.ChangedEntitlement.BookEntitlement["Id"].(string))
		}
	}
	return ids
}

func (f *koboScopeFixture) addFile(t *testing.T, book *models.Book, name, fileType string) *models.File {
	t.Helper()
	return insertScopeFile(context.Background(), t, f.db, book, filepath.Dir(f.fileForBook(book).Filepath), name, fileType, models.FileRoleMain)
}

func (f *koboScopeFixture) fileForBook(book *models.Book) *models.File {
	if book.ID == f.bookA.ID {
		return f.fileA
	}
	return f.fileB
}

func (f *koboScopeFixture) deleteFile(t *testing.T, file *models.File) {
	t.Helper()
	_, err := f.db.NewDelete().Model((*models.File)(nil)).Where("id = ?", file.ID).Exec(context.Background())
	require.NoError(t, err)
}

func (f *koboScopeFixture) setRules(t *testing.T, rules ...delivery.Rule) {
	t.Helper()
	var err error
	f.device, err = devices.NewService(f.db).SetRules(context.Background(), f.user.ID, f.device.ID, devices.RulesModeCustom, rules)
	require.NoError(t, err)
}

// When the File an entry holds is deleted and another File of the book is
// delivered as the same type, the entry is handed to it: the Kobo gets a
// ChangedEntitlement under the same ids and updates the book in place.
func TestHandleSync_SwapToAnotherFileKeepsTheEntry(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, token := f.sync(t, "")
	entryID := f.entryID(t, f.fileA.ID)
	rev := entitlementRevisionID(t, f.itemFor(t, items, f.fileA.ID).NewEntitlement)

	other := f.addFile(t, f.bookA, "Alpha Second Edition", models.FileTypeEPUB)
	f.nameFile(t, other, "Alpha Second Edition")
	f.deleteFile(t, f.fileA)

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], entryID, rev)
	assert.Equal(t, entryID, f.entryID(t, other.ID))
	assert.Equal(t, "Alpha Second Edition", items[0].ChangedEntitlement.BookMetadata["Title"])
}

// A newly added File that a higher rule selects takes over the entry of the
// File it replaces: a CBZ-only book on the Kobo moves to its new EPUB in
// place, under the same ids.
func TestHandleSync_NewlyPreferredFileTakesOverTheEntry(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	comic, cbz := insertScopeBookWithFile(context.Background(), t, f.db, f.libA.ID, "Comic", models.FileTypeCBZ)

	items, token := f.sync(t, "")
	entryID := f.entryID(t, cbz.ID)
	rev := entitlementRevisionID(t, f.itemFor(t, items, cbz.ID).NewEntitlement)

	epub := insertScopeFile(context.Background(), t, f.db, comic, filepath.Dir(cbz.Filepath), "Comic Novel", models.FileTypeEPUB, models.FileRoleMain)

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], entryID, rev)
	assert.Equal(t, entryID, f.entryID(t, epub.ID))
}

// Under the Kobo preset, a book on the Kobo as a KePub converted from its
// EPUB that gains a KePub File moves to the KePub in place: same book,
// category, and delivered File Type, so the entry is handed over.
func TestHandleSync_AddedKePubTakesOverTheEPUBsEntry(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	require.Equal(t, devices.RulesModePreset, f.device.DeliveryRulesMode)

	items, token := f.sync(t, "")
	entryID := f.entryID(t, f.fileA.ID)
	rev := entitlementRevisionID(t, f.itemFor(t, items, f.fileA.ID).NewEntitlement)

	kepub := f.addFile(t, f.bookA, "Alpha", models.FileTypeKePub)

	items, _ = f.sync(t, token)
	require.Len(t, items, 1)
	assertChangedUnder(t, items[0], entryID, rev)
	assert.Equal(t, entryID, f.entryID(t, kepub.ID))
}

// A File that stops being delivered with no File to take its entry is
// removed under its entry id.
func TestHandleSync_FileWithNoReplacementIsRemoved(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	_, token := f.sync(t, "")
	entryID := f.entryID(t, f.fileB.ID)
	f.deleteFile(t, f.fileB)

	items, _ := f.sync(t, token)
	assert.Equal(t, []string{entryID}, removedIDs(items))
	require.Len(t, items, 1)
}

// Files tied on a rule each hold their own entry, so a second EPUB edition
// arrives as a new book, titled with its File name, and the first edition's
// entry is left alone.
func TestHandleSync_TiedFilesHoldSeparateEntries(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	_, token := f.sync(t, "")
	firstEntry := f.entryID(t, f.fileA.ID)
	second := f.addFile(t, f.bookA, "Alpha Annotated", models.FileTypeEPUB)
	f.nameFile(t, second, "Alpha Annotated")

	items, _ := f.sync(t, token)
	require.Len(t, items, 1)
	added := f.itemFor(t, items, second.ID)
	require.NotNil(t, added.NewEntitlement)
	assert.NotEqual(t, firstEntry, added.NewEntitlement.BookEntitlement["Id"])
	assert.Equal(t, "Alpha Annotated", added.NewEntitlement.BookMetadata["Title"])
	assert.Equal(t, firstEntry, f.entryID(t, f.fileA.ID), "a still-delivered File keeps its entry")
}

// A still-delivered File's entry is never handed to another File, even when
// a File of the same book and type loses its own.
func TestHandleSync_StillDeliveredEntryIsNeverHandedOver(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	second := f.addFile(t, f.bookA, "Alpha Second", models.FileTypeEPUB)

	_, token := f.sync(t, "")
	firstEntry := f.entryID(t, f.fileA.ID)
	secondEntry := f.entryID(t, second.ID)
	f.deleteFile(t, second)

	items, _ := f.sync(t, token)
	assert.Equal(t, []string{secondEntry}, removedIDs(items))
	require.Len(t, items, 1)
	assert.Equal(t, firstEntry, f.entryID(t, f.fileA.ID))
}

// New entries get ids that can't be read as a File's Shisho id.
func TestHandleSync_NewEntryIDsCannotMatchFileIDs(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, _ := f.sync(t, "")
	require.Len(t, items, 2)
	for _, item := range items {
		require.NotNil(t, item.NewEntitlement)
		assert.NotRegexp(t, `^shisho-\d+$`, item.NewEntitlement.BookEntitlement["Id"])
	}
}

// Changing what a Kobo receives from KePub to EPUB goes out as one entry
// removed and another added, never as a change in place, and the new entry
// is reported as EPUB3.
func TestHandleSync_DeliveredTypeChangeIsRemovePlusAdd(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	_, token := f.sync(t, "")
	kepubEntry := f.entryID(t, f.fileA.ID)
	f.setScope(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libA.ID})
	_, token = f.sync(t, token)

	f.setRules(t, delivery.Rule{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB})
	items, _ := f.sync(t, token)

	require.Len(t, items, 2)
	assert.Equal(t, []string{kepubEntry}, removedIDs(items))
	added := f.itemFor(t, items, f.fileA.ID)
	require.NotNil(t, added.NewEntitlement)
	assert.NotEqual(t, kepubEntry, added.NewEntitlement.BookEntitlement["Id"])
	urls := added.NewEntitlement.BookMetadata["DownloadUrls"].([]any)
	require.Len(t, urls, 1)
	assert.Equal(t, "EPUB3", urls[0].(map[string]any)["Format"])
}

// A CBZ delivered as EPUB goes out as a fixed-layout EPUB3FL, and downloads
// as an EPUB.
func TestHandleSync_CBZAsEPUBIsFixedLayout(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	_, cbz := insertScopeBookWithFile(context.Background(), t, f.db, f.libA.ID, "Comic", models.FileTypeCBZ)
	f.setRules(t, delivery.Rule{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB})

	items, _ := f.sync(t, "")
	require.Len(t, items, 1)
	item := f.itemFor(t, items, cbz.ID)
	urls := item.NewEntitlement.BookMetadata["DownloadUrls"].([]any)
	require.Len(t, urls, 1)
	assert.Equal(t, "EPUB3FL", urls[0].(map[string]any)["Format"])

	rec := f.serve(http.MethodGet, downloadPaths(f.prefix(), f.entryID(t, cbz.ID))[0])
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Header().Get("Content-Disposition"), ".epub")
	assert.NotContains(t, rec.Header().Get("Content-Disposition"), ".kepub")
}

// A book that synced as both an EPUB and a CBZ before the upgrade keeps only
// the entry its rules select: the EPUB's, under its old id and RevisionId,
// unchanged, while the CBZ's is removed.
func TestHandleSync_PreUpgradeBookWithEPUBAndCBZKeepsOneEntry(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()
	cbz := f.addFile(t, f.bookA, "Alpha Comic", models.FileTypeCBZ)

	svc := NewService(f.db)
	var snapshot []ScopedFile
	for _, file := range []*models.File{f.fileA, cbz, f.fileB} {
		f.addEntry(t, file.ID, models.FileTypeKePub)
		withBook := file
		book := f.bookA
		if file.ID == f.fileB.ID {
			book = f.bookB
		}
		snapshot = append(snapshot, ScopedFile{
			FileID:            file.ID,
			FileHash:          computeFileHash(withBook),
			FileSize:          file.FilesizeBytes,
			MetadataHash:      computeMetadataHash(book, file),
			EntryID:           ShishoID(file.ID),
			DeliveredFileType: models.FileTypeKePub,
			RevisionID:        "rev-" + ShishoID(file.ID),
		})
	}
	sp, err := svc.CreateSyncPoint(ctx, f.device.ID, snapshot)
	require.NoError(t, err)
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, f.device.ID, sp.ID))
	_, err = f.db.ExecContext(ctx, `UPDATE kobo_sync_point_books SET cover_modified_at = NULL`)
	require.NoError(t, err)

	items, _ := f.sync(t, encodeTestSyncToken(t, SyncToken{LastSyncPointID: sp.ID}))
	assert.Equal(t, []string{ShishoID(cbz.ID)}, removedIDs(items))
	require.Len(t, items, 1, "the books that stay are unchanged")
	assert.Equal(t, ShishoID(f.fileA.ID), f.entryID(t, f.fileA.ID))
	assert.Equal(t, ShishoID(f.fileB.ID), f.entryID(t, f.fileB.ID))

	// Their RevisionIds are the ones they went out with.
	f.retitleBook(t, f.bookA, "Alpha Revised")
	items, _ = f.sync(t, "")
	item := f.itemFor(t, items, f.fileA.ID)
	assert.Equal(t, "rev-"+ShishoID(f.fileA.ID), entitlementRevisionID(t, item.NewEntitlement))
}

// Clearing a Device's sync history forces a fresh sync, but its entries stay,
// so books already on the Kobo come back under their ids and RevisionIds.
func TestHandleSync_ClearedHistoryKeepsEntries(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	items, _ := f.sync(t, "")
	entryID := f.entryID(t, f.fileA.ID)
	rev := entitlementRevisionID(t, f.itemFor(t, items, f.fileA.ID).NewEntitlement)
	f.resetSync(t)

	items, _ = f.sync(t, "")
	item := f.itemFor(t, items, f.fileA.ID)
	assert.Equal(t, entryID, item.NewEntitlement.BookEntitlement["Id"])
	assert.Equal(t, rev, entitlementRevisionID(t, item.NewEntitlement))
}

// A damaged EPUB (here one that is not a zip) delivered to a Kobo as KePub
// is a 500 with no attachment: a failed Conversion never sends the Original
// File in its place. Delivered as EPUB, it still gets the Original File,
// since a failed generation of its own File Type only loses Shisho's
// metadata. A failure partway through a Conversion is covered with the other
// channels in pkg/server's streaming tests.
func TestHandleDownload_DamagedEPUB(t *testing.T) {
	t.Parallel()
	damaged := func(t *testing.T, f *koboScopeFixture) *models.File {
		t.Helper()
		f.deleteFile(t, f.fileA)
		path := filepath.Join(filepath.Dir(f.fileA.Filepath), "broken.bin")
		require.NoError(t, os.WriteFile(path, []byte("not a zip"), 0o644))
		file := &models.File{LibraryID: f.libA.ID, BookID: f.bookA.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: path, FilesizeBytes: 9}
		_, err := f.db.NewInsert().Model(file).Exec(context.Background())
		require.NoError(t, err)
		return file
	}

	t.Run("as KePub", func(t *testing.T) {
		t.Parallel()
		f := newKoboScopeFixture(t)
		file := damaged(t, f)
		f.sync(t, "")
		for _, path := range downloadPaths(f.prefix(), f.entryID(t, file.ID)) {
			rec := f.serve(http.MethodGet, path)
			assert.Equal(t, http.StatusInternalServerError, rec.Code, path)
			assert.Empty(t, rec.Header().Get("Content-Disposition"), path)
			assert.NotContains(t, rec.Body.String(), "not a zip", path)
		}
	})

	t.Run("as EPUB", func(t *testing.T) {
		t.Parallel()
		f := newKoboScopeFixture(t)
		file := damaged(t, f)
		f.setRules(t, delivery.Rule{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB})
		f.sync(t, "")
		rec := f.serve(http.MethodGet, downloadPaths(f.prefix(), f.entryID(t, file.ID))[0])
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "not a zip", rec.Body.String())
		assert.Equal(t, `attachment; filename="broken.bin"`, rec.Header().Get("Content-Disposition"))
	})
}

// An entry can be handed a File whose own entry, of another delivered type,
// is removed in the same sync.
func TestHandleSync_HandoverToAFileWhoseEntryIsRemoved(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()
	second := f.addFile(t, f.bookA, "Alpha Second", models.FileTypeEPUB)

	_, token := f.sync(t, "")
	firstEntry := f.entryID(t, f.fileA.ID)
	// The second edition went out as EPUB under rules since changed back.
	_, err := f.db.ExecContext(ctx, `UPDATE kobo_sync_entries SET delivered_file_type = 'epub' WHERE file_id = ?`, second.ID)
	require.NoError(t, err)
	staleEntry := f.entryID(t, second.ID)
	f.deleteFile(t, f.fileA)

	items, _ := f.sync(t, token)
	assert.Equal(t, firstEntry, f.entryID(t, second.ID), "the KePub entry moves to the second edition")
	assert.Equal(t, []string{staleEntry}, removedIDs(items))
}
