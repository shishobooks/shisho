package koreaderplugin

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/koreader"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

// Copies no server the plugin signed in to recognizes (ADR 0017). The
// plugin reports one only once every server has said it doesn't recognize
// it. The server holds its page rows and events against its hash, treats
// them as taken in, and lists the copy on the Device's page, where the
// user links it to a File, which applies what was held. Dismissing it only
// sets it aside, still held, to restore or link later. Deleting its reading
// deletes what was held and drops the copy's data from then on, until the
// user restores it.

// unrecognizedBook is the record of document on device, or nil for none.
func (h *handler) unrecognizedBook(ctx context.Context, deviceID, document string) (*models.KOReaderUnrecognizedBook, error) {
	book := &models.KOReaderUnrecognizedBook{}
	err := h.db.NewSelect().
		Model(book).
		Where("kub.device_id = ? AND kub.document = ?", deviceID, document).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return book, errors.WithStack(err)
}

// dropped reports whether the user deleted the reading of document on
// device, so its data is dropped.
func (h *handler) dropped(ctx context.Context, deviceID, document string) (bool, error) {
	exists, err := h.db.NewSelect().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Where("kub.device_id = ? AND kub.document = ? AND kub.reading_deleted_at IS NOT NULL", deviceID, document).
		Exists(ctx)
	return exists, errors.WithStack(err)
}

// noteUnrecognized records that device holds data for document, which no
// server recognizes, with what KOReader knows of it when the plugin sent
// that (about). A copy whose reading was deleted is left alone.
func (h *handler) noteUnrecognized(ctx context.Context, user *models.User, device *devices.Device, clock deviceClock, document string, about *UnrecognizedPayload) error {
	now := time.Now()
	_, err := h.db.NewInsert().
		Model(&models.KOReaderUnrecognizedBook{
			DeviceID: device.ID, Document: document, UserID: user.ID,
			CreatedAt: now, UpdatedAt: now, Authors: []string{}, Identifiers: []string{},
		}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if about == nil {
		return nil
	}
	return h.describeUnrecognized(ctx, device, clock, about)
}

// describeUnrecognized records what KOReader knows of a copy device holds
// data for, as about says. A copy the server doesn't hold, or whose reading
// the user deleted, is left alone.
func (h *handler) describeUnrecognized(ctx context.Context, device *devices.Device, clock deviceClock, about *UnrecognizedPayload) error {
	book := &models.KOReaderUnrecognizedBook{
		UpdatedAt:      time.Now(),
		Title:          nonEmpty(about.Title),
		Authors:        trimmed(about.Authors),
		Identifiers:    trimmed(about.Identifiers),
		Language:       nonEmpty(about.Language),
		FileType:       nonEmpty(strings.ToLower(about.FileType)),
		ReadingSeconds: about.ReadingSeconds,
		FirstReadAt:    clock.reported(about.FirstRead),
		LastReadAt:     clock.reported(about.LastRead),
	}
	_, err := h.db.NewUpdate().
		Model(book).
		Column("updated_at", "title", "authors", "identifiers", "language", "file_type", "reading_seconds", "first_read_at", "last_read_at").
		Where("device_id = ? AND document = ? AND reading_deleted_at IS NULL", device.ID, about.Document).
		Exec(ctx)
	return errors.WithStack(err)
}

// reported is a time KOReader recorded about a copy, after correcting the
// device's clock, or nil for none or one before the year 2000. It may
// predate the Device, as reading before the plugin signed in does.
func (c deviceClock) reported(deviceTime int64) *time.Time {
	if deviceTime <= 0 {
		return nil
	}
	t := time.Unix(deviceTime+c.shift, 0).UTC()
	if t.Before(earliestInstant) {
		return nil
	}
	if t.After(c.arrival) {
		t = c.arrival.UTC()
	}
	return &t
}

func nonEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func trimmed(values []string) []string {
	out := []string{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// holdEvent holds event, about a copy no server recognizes, until the copy
// is linked, with its time corrected now, by this request's clock. It
// returns false for one refused for its time. A status or Rating from
// history is dropped when the copy already holds one: history fills in only
// what's missing, and history sent again (ADR 0017) would otherwise be
// applied ahead of the copy's own reading when it's linked.
func (h *handler) holdEvent(ctx context.Context, user *models.User, device *devices.Device, clock deviceClock, event EventPayload, history bool) (bool, error) {
	if history && (event.Type == PluginEventStatus || event.Type == PluginEventRating) {
		holds, err := h.db.NewSelect().
			Model((*models.KOReaderHeldEvent)(nil)).
			Where("khe.device_id = ? AND khe.document = ? AND khe.type = ?", device.ID, event.Document, event.Type).
			Exists(ctx)
		if err != nil {
			return false, errors.WithStack(err)
		}
		if holds {
			return true, nil
		}
	}
	held := &models.KOReaderHeldEvent{
		CreatedAt:      time.Now(),
		DeviceID:       device.ID,
		Document:       event.Document,
		EventID:        event.ID,
		Type:           event.Type,
		History:        history,
		Rating:         event.Rating,
		PreviousRating: event.PreviousRating,
		Status:         nonEmpty(event.Status),
		PreviousStatus: nonEmpty(event.PreviousStatus),
	}
	if event.Type == PluginEventPosition {
		at, ok := clock.instant(event.At)
		if !ok {
			return false, nil
		}
		if event.Percentage == nil {
			return true, nil
		}
		held.OccurredAt = &at
		held.Percentage = event.Percentage
	} else {
		loc, err := h.reading.Location(ctx, user.ID)
		if err != nil {
			return false, err
		}
		day, ok := clock.changeDay(event, loc)
		if !ok {
			return false, nil
		}
		held.ChangedDay = &day
	}
	_, err := h.db.NewInsert().Model(held).Exec(ctx)
	return true, errors.WithStack(err)
}

// dropHeld deletes what device held for each of documents: its ungrouped
// page rows and its events. Deleting a copy's reading, and a claim, drop
// held data through it.
func dropHeld(ctx context.Context, db bun.IDB, deviceID string, documents []string) error {
	_, err := db.NewDelete().
		Model((*models.KOReaderPageRow)(nil)).
		Where("device_id = ? AND document IN (?) AND file_id IS NULL AND NOT grouped", deviceID, bun.List(documents)).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = db.NewDelete().
		Model((*models.KOReaderHeldEvent)(nil)).
		Where("device_id = ? AND document IN (?)", deviceID, bun.List(documents)).
		Exec(ctx)
	return errors.WithStack(err)
}

// dropClaimed forgets each of documents that another server the plugin
// signed in to now recognizes: what device held for it and its record,
// unless the user linked it here. A deletion stays, so the copy's data stays
// dropped here if the other server stops recognizing it.
func (h *handler) dropClaimed(ctx context.Context, deviceID string, documents []string) error {
	for _, document := range documents {
		book, err := h.unrecognizedBook(ctx, deviceID, document)
		if err != nil {
			return err
		}
		if book != nil && book.LinkedAt != nil {
			continue
		}
		if err := dropHeld(ctx, h.db, deviceID, []string{document}); err != nil {
			return err
		}
		if book != nil && book.ReadingDeletedAt != nil {
			continue
		}
		_, err = h.db.NewDelete().
			Model((*models.KOReaderUnrecognizedBook)(nil)).
			Where("device_id = ? AND document = ?", deviceID, document).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// lookUp says what the server knows of document for this request.
func (h *handler) lookUp(ctx context.Context, user *models.User, device *devices.Device, document string) (string, error) {
	file, err := h.documents.Resolve(ctx, user, device, document)
	if err != nil {
		return "", err
	}
	if file != nil {
		return DocumentRecognized, nil
	}
	book, err := h.currentBook(ctx, user, device, document)
	switch {
	case err != nil:
		return "", err
	case book == nil:
		return DocumentUnknown, nil
	case book.ReadingDeletedAt != nil:
		return DocumentDismissed, nil
	default:
		return DocumentHeld, nil
	}
}

// maxUndescribed bounds the copies one lookup asks the plugin to describe.
const maxUndescribed = 500

// undescribed is the copies device holds as unrecognized, dismissed or
// not, with nothing KOReader knows of them, no title or file type, which the
// plugin is asked to describe. A copy that resolves to a File now, as one Shisho has since
// produced, isn't unrecognized any more, so it isn't asked about.
func (h *handler) undescribed(ctx context.Context, user *models.User, device *devices.Device) ([]string, error) {
	var candidates []string
	err := h.db.NewSelect().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Column("kub.document").
		Where("kub.device_id = ? AND kub.reading_deleted_at IS NULL AND kub.linked_at IS NULL", device.ID).
		Where("kub.title IS NULL AND kub.file_type IS NULL").
		OrderExpr("kub.document").
		Limit(maxUndescribed).
		Scan(ctx, &candidates)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	documents := []string{}
	for _, document := range candidates {
		file, err := h.documents.Resolve(ctx, user, device, document)
		if err != nil {
			return nil, err
		}
		if file == nil {
			documents = append(documents, document)
		}
	}
	return documents, nil
}

// currentBook is the record of document on device, as unrecognized
// (unrecognizedBook). A copy linked to a File it no longer resolves to,
// such as one since deleted or now out of reach, is one to link again, so
// its record goes back to waiting for a link.
func (h *handler) currentBook(ctx context.Context, user *models.User, device *devices.Device, document string) (*models.KOReaderUnrecognizedBook, error) {
	book, err := h.unrecognizedBook(ctx, device.ID, document)
	if err != nil || book == nil || book.LinkedAt == nil {
		return book, err
	}
	file, err := h.documents.Resolve(ctx, user, device, document)
	if err != nil || file != nil {
		return book, err
	}
	book.LinkedAt, book.LinkedFileID, book.UpdatedAt = nil, nil, time.Now()
	_, err = h.db.NewUpdate().Model(book).Column("linked_at", "linked_file_id", "updated_at").WherePK().Exec(ctx)
	return book, errors.WithStack(err)
}

// deliveredAs is the File Type a linked copy was delivered as: its file's
// extension when that's one KOReader reads, and otherwise the File's own.
func deliveredAs(book *models.KOReaderUnrecognizedBook, file *models.File) string {
	if book.FileType != nil {
		switch *book.FileType {
		case models.FileTypeEPUB, models.FileTypeKePub, models.FileTypeCBZ, models.FileTypePDF:
			return *book.FileType
		}
	}
	return file.FileType
}

// linkable reports whether a File can be linked to a KOReader copy: a main
// File of a kind KOReader reads.
func linkable(file *models.File) bool {
	return file.FileRole == models.FileRoleMain && !models.IsAudiobookFileType(file.FileType)
}

// reachableFile returns the File fileID when user reaches it through
// device, errFileNotFound when it doesn't exist or isn't in reach, and
// errFileNotLinkable for one a copy can't be linked to.
func (h *handler) reachableFile(ctx context.Context, user *models.User, device *devices.Device, fileID int) (*models.File, error) {
	file := &models.File{}
	err := h.db.NewSelect().Model(file).Where("f.id = ?", fileID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errFileNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !user.HasLibraryAccess(file.LibraryID) {
		return nil, errFileNotFound
	}
	r, err := h.reach.Scope(ctx, user, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	holds, err := h.reach.HoldsBook(ctx, r, file.LibraryID, file.BookID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !holds {
		return nil, errFileNotFound
	}
	if !linkable(file) {
		return nil, errFileNotLinkable
	}
	return file, nil
}

// A link or dismissal the user can't make: the copy isn't one waiting to be
// linked, the File isn't one the Device reaches, or it's one a KOReader copy
// can't be, such as an audiobook.
var (
	errCopyNotFound    = errors.New("unrecognized copy not found")
	errFileNotFound    = errors.New("file not reachable")
	errFileNotLinkable = errors.New("file not linkable")
)

// link links document on device to the File fileID (ADR 0017), applying
// what was held for the copy at once, on every one of the user's Devices
// that holds it and reaches the File, so its history appears without a
// sync. What was held is applied before the copy resolves to the File, so a
// sync during it holds what it sends rather than applying it ahead of older
// held data, and a link that fails partway is finished by trying again.
// Whatever a sync held while it ran is applied once more after. A dismissed
// copy links like any other, and is no longer dismissed.
func (h *handler) link(ctx context.Context, user *models.User, device *devices.Device, document string, fileID int) error {
	book, err := h.currentBook(ctx, user, device, document)
	if err != nil {
		return err
	}
	if book == nil || book.UserID != user.ID || book.ReadingDeletedAt != nil || book.LinkedAt != nil {
		return errCopyNotFound
	}
	file, err := h.reachableFile(ctx, user, device, fileID)
	if err != nil {
		return err
	}
	delivered := deliveredAs(book, file)
	linked := &models.DocumentHash{Hash: document, FileID: file.ID, DeliveredFileType: delivered, UserMade: true, File: file}

	var records []*models.KOReaderUnrecognizedBook
	err = h.db.NewSelect().
		Model(&records).
		Where("kub.user_id = ? AND kub.document = ?", user.ID, document).
		Where("kub.reading_deleted_at IS NULL AND kub.linked_at IS NULL").
		OrderExpr("kub.device_id = ? DESC, kub.device_id", device.ID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	var holders []*devices.Device
	for _, record := range records {
		holder := device
		if record.DeviceID != device.ID {
			holder, err = h.devices.Retrieve(ctx, user.ID, record.DeviceID)
			if err != nil {
				return errors.WithStack(err)
			}
			// A Device that doesn't reach the File keeps its copy to link.
			if _, err := h.reachableFile(ctx, user, holder, file.ID); errors.Is(err, errFileNotFound) {
				continue
			} else if err != nil {
				return err
			}
		}
		if err := h.applyHeld(ctx, user, holder, linked); err != nil {
			return err
		}
		holders = append(holders, holder)
	}

	if err := h.hashes.LinkDocument(ctx, user.ID, file.ID, delivered, document); err != nil {
		return errors.WithStack(err)
	}
	for _, holder := range holders {
		now := time.Now()
		_, err = h.db.NewUpdate().
			Model((*models.KOReaderUnrecognizedBook)(nil)).
			Set("linked_at = ?", now).
			Set("linked_file_id = ?", file.ID).
			Set("dismissed_at = NULL").
			Set("updated_at = ?", now).
			Where("device_id = ? AND document = ?", holder.ID, document).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if err := h.applyHeld(ctx, user, holder, linked); err != nil {
			return err
		}
	}
	return nil
}

// applyHeld applies what device held for the copy now linked as file, in
// the order a sync sends it: history's page rows, then its statuses and
// Ratings, then the rest of the rows, then the rest of the events. Each
// event is deleted once applied, so a link that fails partway is finished
// by trying again.
func (h *handler) applyHeld(ctx context.Context, user *models.User, device *devices.Device, file *models.DocumentHash) error {
	for _, history := range []bool{true, false} {
		if err := h.reading.GroupHeldRows(ctx, user.ID, device.ID, file.Hash, file.FileID, history); err != nil {
			return err
		}
		var events []*models.KOReaderHeldEvent
		err := h.db.NewSelect().
			Model(&events).
			Where("khe.device_id = ? AND khe.document = ? AND khe.history = ?", device.ID, file.Hash, history).
			OrderExpr("khe.id").
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, event := range events {
			if err := h.applyHeldEvent(ctx, user, device, file, event); err != nil {
				return err
			}
			if _, err := h.db.NewDelete().Model(event).WherePK().Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

// applyHeldEvent applies one held event to file, as it would have been had
// the copy been recognized when it arrived.
func (h *handler) applyHeldEvent(ctx context.Context, user *models.User, device *devices.Device, file *models.DocumentHash, held *models.KOReaderHeldEvent) error {
	if held.Type == PluginEventPosition {
		if held.OccurredAt == nil || held.Percentage == nil {
			return nil
		}
		event := EventPayload{ID: held.EventID, Type: held.Type, Document: held.Document, Percentage: held.Percentage}
		return h.applyPositionTo(ctx, user, device, file, event, *held.OccurredAt)
	}
	if held.ChangedDay == nil || file.File == nil || file.File.FileRole == models.FileRoleSupplement {
		return nil
	}
	loc, err := h.reading.Location(ctx, user.ID)
	if err != nil {
		return err
	}
	event := EventPayload{
		ID: held.EventID, Type: held.Type, Document: held.Document,
		Rating: held.Rating, PreviousRating: held.PreviousRating,
	}
	if held.Status != nil {
		event.Status = *held.Status
	}
	if held.PreviousStatus != nil {
		event.PreviousStatus = *held.PreviousStatus
	}
	c := &change{file: file, day: *held.ChangedDay, loc: loc}
	if held.History {
		return h.applyHistoryTo(ctx, user, c, event)
	}
	return h.applyChangeTo(ctx, user, c, event, nil)
}

// unlink undoes the user's link of document (ADR 0017): the copy stops
// resolving to the File on every one of the user's Devices, goes back to
// waiting for a link, and its reading from then on is held. What the link
// already applied stays, for the user to correct by hand.
func (h *handler) unlink(ctx context.Context, user *models.User, device *devices.Device, document string) error {
	book, err := h.currentBook(ctx, user, device, document)
	if err != nil {
		return err
	}
	if book == nil || book.UserID != user.ID || book.LinkedAt == nil {
		return errCopyNotFound
	}
	return h.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().
			Model((*models.KOReaderDocumentLink)(nil)).
			Where("user_id = ? AND hash = ?", user.ID, document).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewUpdate().
			Model((*models.KOReaderUnrecognizedBook)(nil)).
			Set("linked_at = NULL").
			Set("linked_file_id = NULL").
			Set("updated_at = ?", time.Now()).
			Where("user_id = ? AND document = ? AND linked_at IS NOT NULL", user.ID, document).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// pendingBook is the user's record of document on device, when it's one
// still to link or dismissed, not linked and with its reading not deleted.
func (h *handler) pendingBook(ctx context.Context, user *models.User, device *devices.Device, document string) (*models.KOReaderUnrecognizedBook, error) {
	book, err := h.currentBook(ctx, user, device, document)
	if err != nil {
		return nil, err
	}
	if book == nil || book.UserID != user.ID || book.ReadingDeletedAt != nil || book.LinkedAt != nil {
		return nil, errCopyNotFound
	}
	return book, nil
}

// dismiss sets document on device aside: it's no longer listed to link,
// and what was held for it, and the copy's data from then on, stay held to
// link later.
func (h *handler) dismiss(ctx context.Context, user *models.User, device *devices.Device, document string) error {
	book, err := h.pendingBook(ctx, user, device, document)
	if err != nil {
		return err
	}
	if book.DismissedAt != nil {
		return errCopyNotFound
	}
	now := time.Now()
	_, err = h.db.NewUpdate().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Set("dismissed_at = ?", now).
		Set("updated_at = ?", now).
		Where("device_id = ? AND document = ?", device.ID, document).
		Exec(ctx)
	return errors.WithStack(err)
}

// dismissAll dismisses each of documents on device still to link, as
// dismiss does one. Linked and dismissed copies, and copies the page didn't
// list, stay as they are. It runs in batches, under SQLite's limit on bound
// values.
func (h *handler) dismissAll(ctx context.Context, user *models.User, device *devices.Device, documents []string) error {
	const batch = 500
	for start := 0; start < len(documents); start += batch {
		chunk := documents[start:min(start+batch, len(documents))]
		now := time.Now()
		_, err := h.db.NewUpdate().
			Model((*models.KOReaderUnrecognizedBook)(nil)).
			Set("dismissed_at = ?", now).
			Set("updated_at = ?", now).
			Where("device_id = ? AND user_id = ? AND document IN (?)", device.ID, user.ID, bun.List(chunk)).
			Where("dismissed_at IS NULL AND reading_deleted_at IS NULL AND linked_at IS NULL").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// restore puts document on device, dismissed or with its reading deleted,
// back to link. A copy whose reading was deleted holds its data again from
// then on, including what the plugin sends again (ADR 0017).
func (h *handler) restore(ctx context.Context, user *models.User, device *devices.Device, document string) error {
	book, err := h.currentBook(ctx, user, device, document)
	if err != nil {
		return err
	}
	if book == nil || book.UserID != user.ID || book.LinkedAt != nil || book.DismissedAt == nil && book.ReadingDeletedAt == nil {
		return errCopyNotFound
	}
	_, err = h.db.NewUpdate().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Set("dismissed_at = NULL").
		Set("reading_deleted_at = NULL").
		Set("updated_at = ?", time.Now()).
		Where("device_id = ? AND document = ?", device.ID, document).
		Exec(ctx)
	return errors.WithStack(err)
}

// deleteReading deletes the reading history held for document on device, a
// dismissed copy, and drops the copy's data from then on, as for a copy from
// another server's library that this one should never hold. The copy stays
// listed as dismissed, so it can be restored.
func (h *handler) deleteReading(ctx context.Context, user *models.User, device *devices.Device, document string) error {
	book, err := h.pendingBook(ctx, user, device, document)
	if err != nil {
		return err
	}
	if book.DismissedAt == nil {
		return errCopyNotFound
	}
	return h.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if err := dropHeld(ctx, tx, device.ID, []string{document}); err != nil {
			return err
		}
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*models.KOReaderUnrecognizedBook)(nil)).
			Set("reading_deleted_at = ?", now).
			Set("updated_at = ?", now).
			Where("device_id = ? AND document = ?", device.ID, document).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// unlinkedCount is how many copies device holds as unrecognized that are
// still to link, as the Device's page counts them (devices.UnlinkedBooks).
func (h *handler) unlinkedCount(ctx context.Context, deviceID string) (int, error) {
	counts, err := h.devices.UnlinkedBooks(ctx, []string{deviceID})
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return counts[deviceID], nil
}

// listUnrecognized lists device's unrecognized copies: the ones to link,
// with suggestions, most recently read first, then the linked ones, most
// recently linked first, then the dismissed ones, most recently dismissed
// first. A linked copy whose File is gone or out of reach is one to link
// again.
func (h *handler) listUnrecognized(ctx context.Context, user *models.User, device *devices.Device) ([]UnrecognizedBook, error) {
	var records []*models.KOReaderUnrecognizedBook
	err := h.db.NewSelect().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Column("kub.document").
		Where("kub.device_id = ? AND kub.user_id = ? AND kub.linked_at IS NOT NULL", device.ID, user.ID).
		Scan(ctx, &records)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, record := range records {
		if _, err := h.currentBook(ctx, user, device, record.Document); err != nil {
			return nil, err
		}
	}

	records = nil
	err = h.db.NewSelect().
		Model(&records).
		Relation("LinkedFile").
		Where("kub.device_id = ? AND kub.user_id = ?", device.ID, user.ID).
		OrderExpr("kub.dismissed_at IS NOT NULL, kub.linked_at IS NOT NULL, kub.linked_at DESC, kub.dismissed_at DESC, COALESCE(kub.last_read_at, kub.created_at) DESC, kub.document").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	scope, err := h.linkScope(ctx, user, device)
	if err != nil {
		return nil, err
	}
	books := []UnrecognizedBook{}
	for _, record := range records {
		book := UnrecognizedBook{
			Document: record.Document, Title: record.Title, Authors: record.Authors, Identifiers: record.Identifiers,
			Language: record.Language, FileType: record.FileType, ReadingSeconds: record.ReadingSeconds,
			FirstReadAt: record.FirstReadAt, LastReadAt: record.LastReadAt, LinkedAt: record.LinkedAt,
			DismissedAt: record.DismissedAt, ReadingDeleted: record.ReadingDeletedAt != nil,
			Suggestions: []LinkCandidate{},
		}
		if book.Authors == nil {
			book.Authors = []string{}
		}
		if book.Identifiers == nil {
			book.Identifiers = []string{}
		}
		switch {
		case record.LinkedAt == nil && record.DismissedAt == nil:
			book.Suggestions, err = h.suggest(ctx, scope, record)
			if err != nil {
				return nil, err
			}
		case record.LinkedFile != nil:
			// Resolved for this Device just now, so it's in reach.
			candidates, err := h.candidates(ctx, []*models.File{record.LinkedFile}, "")
			if err != nil {
				return nil, err
			}
			if len(candidates) > 0 {
				book.Linked = &candidates[0]
			}
		}
		books = append(books, book)
	}
	return books, nil
}

// pullLinked returns the documents device linked since since, whose
// statuses and Ratings the device should now hold.
func (h *handler) pullLinked(ctx context.Context, deviceID string, since time.Time) ([]string, error) {
	documents := []string{}
	err := h.db.NewSelect().
		Model((*models.KOReaderUnrecognizedBook)(nil)).
		Column("kub.document").
		Where("kub.device_id = ? AND kub.linked_at > ?", deviceID, since).
		OrderExpr("kub.document").
		Scan(ctx, &documents)
	return documents, errors.WithStack(err)
}

// atEnd reports whether percentage is the end of the book, as KOReader
// counts it.
func atEnd(percentage float64) bool {
	return percentage >= koreader.FinishedAt
}

// positionUpdate is the position update for percentage in file, reached
// at at, from device.
func positionUpdate(user *models.User, device *devices.Device, file *models.DocumentHash, percentage float64, at time.Time) reading.PositionUpdate {
	return reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     file.FileID,
		Source:     models.ReadingSourceKOReader,
		DeviceID:   &device.ID,
		Percentage: percentage,
		AtEnd:      atEnd(percentage),
		OccurredAt: &at,
	}
}
