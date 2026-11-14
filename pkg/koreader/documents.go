package koreader

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/reflowable"
	"github.com/uptrace/bun"
)

// Documents finds the File a KOReader document hash names, and gives
// KOReader a Reading Position in that copy, labelled with where it came
// from. Progress sync and the KOReader plugin both use it.
type Documents struct {
	deviceService      *devices.Service
	fingerprintService *fingerprints.Service
	readingService     *reading.Service
	reachService       *reach.Service
}

// NewDocuments returns Documents for db, reaching Books through bookService.
func NewDocuments(db *bun.DB, bookService *books.Service) *Documents {
	return &Documents{
		deviceService:      devices.NewService(db),
		fingerprintService: fingerprints.NewService(db),
		readingService:     reading.NewService(db),
		reachService:       reach.NewService(db, bookService),
	}
}

// Progress is what device gets for position in the copy file names.
//
// A position another source wrote is labelled with that source's identity in
// Shisho, so KOReader takes it for another device's and offers the jump. A
// reflowable one is translated into this copy's XPointer: the web reader's
// CFI, another KOReader's XPointer in a copy with or without a KePub's span
// markup, or this Device's own from its other copy.
//
// A copy a user linked (ADR 0017), or a position kept as a percentage only,
// has no place KOReader's progress sync can go to, so it gets none.
func (d *Documents) Progress(ctx context.Context, user *models.User, device *devices.Device, document string, file *models.DocumentHash, position *models.ReadingPosition) (ProgressResponse, error) {
	if position == nil || file.UserMade || position.IsZero() {
		return ProgressResponse{}, nil
	}
	response := ProgressResponse{Document: document, Percentage: &position.Percentage}

	// One Device can hold a File both as itself and converted, such as a CBZ
	// and its EPUB. A position in the other's form means nothing to this copy,
	// and KOReader can't go to it.
	pageBased := models.IsPageBasedFileType(file.DeliveredFileType)
	switch {
	case pageBased && position.Page != nil:
		response.Progress = strconv.Itoa(*position.Page + 1)
	case pageBased:
		return ProgressResponse{}, nil
	case position.XPointer != nil && sameCopy(position, file):
		response.Progress = *position.XPointer
	default:
		progress, ok := d.translate(ctx, position, file)
		if !ok {
			return ProgressResponse{}, nil
		}
		response.Progress = progress
	}

	timestamp := position.OccurredAt.Unix()

	// A Device gets back exactly what it pushed, under the name and id it
	// sent, as KOReader's own server does. KOReader ignores a position
	// carrying its own, so syncing never moves the device that pushed it.
	if position.SourceDeviceID != nil && *position.SourceDeviceID == device.ID {
		if position.ReportedDevice != nil && position.ReportedDeviceID != nil {
			response.Device, response.DeviceID = *position.ReportedDevice, *position.ReportedDeviceID
			response.Timestamp = &timestamp
			return response, nil
		}
		// Without its own name and id, KOReader would take the position for
		// another device's. With no time it judges by percentage, so a clock
		// that has drifted can't make it offer to move back.
		response.Device, response.DeviceID = device.Name, device.ID
		return response, nil
	}

	response.Timestamp = &timestamp
	name, id, err := d.Source(ctx, user, position)
	if err != nil {
		return ProgressResponse{}, err
	}
	response.Device, response.DeviceID = name, id
	return response, nil
}

// Source names the reader that wrote position, and gives it an id, as
// KOReader shows another device's: the Device's own, or a stand-in for the
// web reader or a Device since removed.
func (d *Documents) Source(ctx context.Context, user *models.User, position *models.ReadingPosition) (string, string, error) {
	switch {
	case position.SourceDeviceID != nil:
		source, err := d.deviceService.Retrieve(ctx, user.ID, *position.SourceDeviceID)
		if err != nil {
			return "", "", err
		}
		return source.Name, source.ID, nil
	case position.Source == models.ReadingSourceKOReader:
		return removedDeviceName, removedDeviceID, nil
	case position.Source == models.ReadingSourceKobo:
		return removedKoboName, removedKoboID, nil
	default:
		return webReaderDeviceName, webReaderDeviceID, nil
	}
}

// sameCopy reports whether position's XPointer was written in the copy file
// names, so it needs no translating.
func sameCopy(position *models.ReadingPosition, file *models.DocumentHash) bool {
	fileType := ""
	if file.File != nil {
		fileType = file.File.FileType
	}
	return reflowable.WrittenIn(position, fileType) == file.DeliveredFileType
}

// translate returns position as an XPointer in the copy file names, or false
// when it can't be: a File or copy that isn't an EPUB or KePub, or a File
// that can't be read.
func (d *Documents) translate(ctx context.Context, position *models.ReadingPosition, file *models.DocumentHash) (string, bool) {
	if file.File == nil {
		return "", false
	}
	locator, err := reflowable.Translate(ctx, file.File.Filepath, file.File.FileType, position, file.DeliveredFileType, reflowable.FormXPointer)
	if errors.Is(err, reflowable.ErrNotTranslatable) {
		return "", false
	}
	if err != nil || locator.XPointer == nil || *locator.XPointer == "" {
		logger.FromContext(ctx).Warn("reading position not translated for koreader", logger.Data{
			"file_id": file.FileID, "delivered_file_type": file.DeliveredFileType, "error": fmt.Sprint(err),
		})
		return "", false
	}
	return *locator.XPointer, true
}

// Resolve returns the File a document hash names for this request, or nil
// when it names none the request reaches. Identical bytes can map to several
// Files, so it's the ones the user's library access and the Device's scope
// reach, preferring one the user already has a position in, so reading doesn't
// split across them, and then the oldest. Only when Shisho produced none of
// them is it the File the user linked the copy to (ADR 0017), marked UserMade.
func (d *Documents) Resolve(ctx context.Context, user *models.User, device *devices.Device, document string) (*models.DocumentHash, error) {
	candidates, err := d.fingerprintService.FindDocumentHash(ctx, document)
	if err != nil {
		return nil, err
	}
	link, err := d.fingerprintService.FindDocumentLink(ctx, user.ID, document)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 && link == nil {
		return nil, nil
	}
	r, err := d.reachService.Scope(ctx, user, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var first *models.DocumentHash
	for _, candidate := range candidates {
		if candidate.File == nil || !user.HasLibraryAccess(candidate.File.LibraryID) {
			continue
		}
		holds, err := d.reachService.HoldsBook(ctx, r, candidate.File.LibraryID, candidate.File.BookID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !holds {
			continue
		}
		position, err := d.readingService.RetrievePosition(ctx, user.ID, candidate.FileID)
		if err != nil {
			return nil, err
		}
		if position != nil {
			return candidate, nil
		}
		if first == nil {
			first = candidate
		}
	}
	if first != nil || link == nil || link.File == nil || !user.HasLibraryAccess(link.File.LibraryID) {
		return first, nil
	}
	holds, err := d.reachService.HoldsBook(ctx, r, link.File.LibraryID, link.File.BookID)
	if err != nil || !holds {
		return nil, errors.WithStack(err)
	}
	return link, nil
}
