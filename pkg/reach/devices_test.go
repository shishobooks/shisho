package reach

import (
	"context"
	"fmt"
	"testing"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kobo adds a Kobo Device on its preset, which delivers EPUB and CBZ as
// KePub.
func (f *fixture) kobo() *devices.Device {
	f.t.Helper()
	d, err := f.devices.Create(context.Background(), f.user.ID, "Kobo", devices.DeviceTypeKobo)
	require.NoError(f.t, err)
	return d
}

// addFile adds a main File of fileType to b.
func (f *fixture) addFile(b *models.Book, fileType string) *models.File {
	f.t.Helper()
	file := &models.File{LibraryID: b.LibraryID, BookID: b.ID, FileType: fileType, FileRole: models.FileRoleMain, Filepath: b.Filepath + "/extra." + fileType, FilesizeBytes: 1}
	_, err := f.db.NewInsert().Model(file).Exec(context.Background())
	require.NoError(f.t, err)
	return file
}

// onKobo records that kobo holds fileID as deliveredType, as a sync would.
func (f *fixture) onKobo(kobo *devices.Device, b *models.Book, fileID int, deliveredType string) {
	f.t.Helper()
	_, err := f.db.ExecContext(context.Background(),
		`INSERT INTO kobo_sync_entries (device_id, entry_id, book_id, file_id, delivered_file_type, revision_id) VALUES (?, ?, ?, ?, ?, 'rev')`,
		kobo.ID, fmt.Sprintf("shisho-%d", fileID), b.ID, fileID, deliveredType)
	require.NoError(f.t, err)
}

func (f *fixture) loadBook(b *models.Book) *models.Book {
	f.t.Helper()
	book, err := f.svc.bookService.RetrieveBook(context.Background(), books.RetrieveBookOptions{ID: &b.ID})
	require.NoError(f.t, err)
	return book
}

func lineFor(t *testing.T, lines []BookDevice, deviceID string) BookDevice {
	t.Helper()
	for _, line := range lines {
		if line.DeviceID == deviceID {
			return line
		}
	}
	t.Fatalf("no line for Device %s", deviceID)
	return BookDevice{}
}

// Each Device gets a line saying what it gets of the book and as what, or
// why it gets nothing.
func TestBookDevices_LinePerDevice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Novels")
	other := f.library("Comics")
	b := f.book(lib, "Novel", models.FileTypeEPUB, "", "")
	epub := f.loadBook(b).Files[0]

	kobo := f.kobo()
	panels := f.device(cbzOnly, nil)
	elsewhere := f.device([]delivery.Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}}, &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &other.ID})

	lines, err := f.svc.BookDevices(ctx, f.user, f.loadBook(b))
	require.NoError(t, err)
	require.Len(t, lines, 3)

	assert.Equal(t, []BookDelivery{{FileID: epub.ID, SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub}}, lineFor(t, lines, kobo.ID).Deliveries)
	assert.Empty(t, lineFor(t, lines, kobo.ID).NotDeliveredReason)

	assert.Empty(t, lineFor(t, lines, panels.ID).Deliveries)
	assert.Equal(t, NotDeliveredNoRule, lineFor(t, lines, panels.ID).NotDeliveredReason)

	assert.Equal(t, NotDeliveredOutsideScope, lineFor(t, lines, elsewhere.ID).NotDeliveredReason)
}

// Files tied on the winning rule each get their own delivery.
func TestBookDevices_TiedFilesEachDeliver(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	lib := f.library("Novels")
	b := f.book(lib, "Novel", models.FileTypeEPUB, "", "")
	second := f.addFile(b, models.FileTypeEPUB)
	kobo := f.kobo()

	lines, err := f.svc.BookDevices(context.Background(), f.user, f.loadBook(b))
	require.NoError(t, err)
	deliveries := lineFor(t, lines, kobo.ID).Deliveries
	require.Len(t, deliveries, 2)
	assert.Equal(t, second.ID, deliveries[1].FileID)
}

// On a Kobo, a book whose entry the next sync hands to a newly added File
// gets an edition move, so the book page can warn that it may reopen at
// the start.
func TestBookDevices_NewFileMovesAKoboToAnotherEdition(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	lib := f.library("Comics")
	b := f.book(lib, "Comic", models.FileTypeCBZ, "", "")
	cbz := f.loadBook(b).Files[0]
	kobo := f.kobo()
	f.onKobo(kobo, b, cbz.ID, models.FileTypeKePub)

	lines, err := f.svc.BookDevices(context.Background(), f.user, f.loadBook(b))
	require.NoError(t, err)
	assert.Empty(t, lineFor(t, lines, kobo.ID).EditionMoves, "nothing moves until a File is added")

	epub := f.addFile(b, models.FileTypeEPUB)
	lines, err = f.svc.BookDevices(context.Background(), f.user, f.loadBook(b))
	require.NoError(t, err)
	assert.Equal(t, []EditionMove{{FromFileID: &cbz.ID, ToFileID: epub.ID}}, lineFor(t, lines, kobo.ID).EditionMoves)
}

// The preview counts the books a Device gets something of, and the books it
// gets nothing of by the File Types of their main Files.
func TestPreview_Counts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Lib")
	f.book(lib, "Novel", models.FileTypeEPUB, "", "")
	f.book(lib, "Comic", models.FileTypeCBZ, "", "")
	f.book(lib, "Scan", models.FileTypePDF, "", "")
	audio := f.book(lib, "Audio", models.FileTypeM4B, "", "")
	f.addFile(audio, models.FileTypePDF)
	kobo := f.kobo()

	preview, err := f.svc.Preview(ctx, f.user, kobo, devices.PresetRules(devices.DeviceTypeKobo))
	require.NoError(t, err)
	assert.Equal(t, 2, preview.ReceivedBooks)
	assert.Equal(t, []NotReceivedCount{
		{SourceFileType: models.FileTypePDF, Books: 2},
		{SourceFileType: models.FileTypeM4B, Books: 1},
	}, preview.NotReceived)

	preview, err = f.svc.Preview(ctx, f.user, kobo, []delivery.Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeKePub}})
	require.NoError(t, err)
	assert.Equal(t, 1, preview.ReceivedBooks)
	assert.Equal(t, []NotReceivedCount{
		{SourceFileType: models.FileTypeEPUB, Books: 1},
		{SourceFileType: models.FileTypePDF, Books: 2},
		{SourceFileType: models.FileTypeM4B, Books: 1},
	}, preview.NotReceived)
}

// On a Kobo, the preview counts the books on it that a change would switch
// between KePub and EPUB, or move to another edition.
func TestPreview_KoboWarnings(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Lib")
	novel := f.book(lib, "Novel", models.FileTypeEPUB, "", "")
	comic := f.book(lib, "Comic", models.FileTypeCBZ, "", "")
	f.addFile(comic, models.FileTypeEPUB)
	kobo := f.kobo()
	f.onKobo(kobo, novel, f.loadBook(novel).Files[0].ID, models.FileTypeKePub)
	f.onKobo(kobo, comic, f.loadBook(comic).Files[0].ID, models.FileTypeKePub)

	// The comic's EPUB outranks its CBZ under the preset, so the Kobo's
	// comic moves to it.
	preview, err := f.svc.Preview(ctx, f.user, kobo, devices.PresetRules(devices.DeviceTypeKobo))
	require.NoError(t, err)
	assert.Equal(t, 1, preview.MovedBooks)
	assert.Equal(t, 0, preview.RetypedBooks)

	preview, err = f.svc.Preview(ctx, f.user, kobo, []delivery.Rule{
		{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
		{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeKePub},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, preview.RetypedBooks, "both books on the Kobo switch from KePub to EPUB")
	assert.Equal(t, 0, preview.MovedBooks)

	// Another Device type never warns about a Kobo's entries.
	opds := f.device(cbzOnly, nil)
	preview, err = f.svc.Preview(ctx, f.user, opds, []delivery.Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}})
	require.NoError(t, err)
	assert.Zero(t, preview.MovedBooks+preview.RetypedBooks)
}
