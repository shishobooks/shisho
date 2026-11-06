package koreader

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/reading"
)

const (
	// webReaderDeviceName and webReaderDeviceID label a position the web
	// reader wrote. KOReader shows the name when it offers the jump.
	webReaderDeviceName = "Shisho web reader"
	webReaderDeviceID   = models.ReadingSourceWebReader
	// removedDeviceName and removedDeviceID label a position from a KOReader
	// Device that's since been deleted.
	removedDeviceName = "KOReader"
	removedDeviceID   = models.ReadingSourceKOReader

	// finishedAt is the percentage that counts as the end. KOReader's
	// continuous view never reports 100%, and the last pages are back matter,
	// so reaching them finishes the book, as a streaming service counts an
	// episode watched once the credits start.
	finishedAt = 0.99

	registerRefused = "Shisho doesn't create accounts here. Add a KOReader Device under Devices in Shisho, then log in with your Shisho username and the Device's password."
	loginRefused    = "Wrong username or password. Use your Shisho username and the password on your KOReader Device's page in Shisho."
)

type handler struct {
	deviceService      *devices.Service
	fingerprintService *fingerprints.Service
	readingService     *reading.Service
	reachService       *reach.Service
}

// authenticate signs a request in from KOReader's x-auth-user and x-auth-key
// headers: the Shisho username and the MD5 of a KOReader Device's credential.
// Every refusal is a 401 in the protocol's shape, since KOReader doesn't
// expect any other status from these routes and keeps retrying a push that
// gets one. Like Middleware.DeviceAuth, it stores the Device and its owner.
func (h *handler) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		username := c.Request().Header.Get("x-auth-user")
		key := c.Request().Header.Get("x-auth-key")
		device, err := h.deviceService.GetByOwnerCredentialMD5(ctx, username, key, devices.ConnectionKOReaderSync)
		if err != nil {
			return err
		}
		if device == nil {
			return refuseLogin(c)
		}
		user, err := h.deviceService.AuthenticateOwner(ctx, device)
		var codeErr *errcodes.Error
		if errors.As(err, &codeErr) {
			return refuseLogin(c)
		}
		if err != nil {
			return err
		}

		go func() {
			_ = h.deviceService.TouchLastAccessed(context.Background(), device.ID)
		}()

		devices.SetDevice(c, device)
		auth.SetUser(c, user)
		return next(c)
	}
}

func refuseLogin(c echo.Context) error {
	return errors.WithStack(c.JSON(http.StatusUnauthorized, ProtocolErrorResponse{Message: loginRefused}))
}

// register always refuses: a Device is created in Shisho, and its credential
// is the password. KOReader shows the message for a 402.
func (h *handler) register(c echo.Context) error {
	return errors.WithStack(c.JSON(http.StatusPaymentRequired, ProtocolErrorResponse{Message: registerRefused}))
}

func (h *handler) authorize(c echo.Context) error {
	return errors.WithStack(c.JSON(http.StatusOK, AuthorizeResponse{Authorized: "OK"}))
}

// updateProgress stores a pushed position. KOReader sends no time, and its
// clock can be well off, so the update is dated when it arrives. A document
// Shisho doesn't recognize is logged and answered as a success: KOReader
// keeps retrying a refused push for weeks, and the book may not be one
// Shisho has at all.
func (h *handler) updateProgress(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	// A later KOReader may add fields, which mustn't refuse its pushes.
	c.Set("disallow_unknown_fields", false) //nolint:forbidigo // A binder option, not auth state.
	params := UpdateProgressPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	locator, ok := parseProgress(params.Progress)
	if !ok {
		return errcodes.ValidationError("Progress must be an XPointer or a page number.")
	}

	file, err := h.resolve(ctx, user, device, params.Document)
	if err != nil {
		return err
	}
	response := UpdateProgressResponse{Document: params.Document, Timestamp: time.Now().Unix()}
	if file == nil {
		logUnknownDocument(ctx, device, params.Document, string(params.Metadata))
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	update := reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     file.FileID,
		Source:     models.ReadingSourceKOReader,
		DeviceID:   &device.ID,
		Locator:    locator,
		Percentage: *params.Percentage,
		AtEnd:      *params.Percentage >= finishedAt,
	}
	if params.Device != "" {
		update.ReportedDevice = &params.Device
	}
	if params.DeviceID != "" {
		update.ReportedDeviceID = &params.DeviceID
	}
	_, err = h.readingService.UpdatePosition(ctx, update)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, response))
}

// parseProgress reads KOReader's progress: an XPointer, which is a path, or
// the 1-based page as a string.
func parseProgress(progress string) (models.ReadingLocator, bool) {
	if strings.HasPrefix(progress, "/") {
		return models.ReadingLocator{XPointer: &progress}, true
	}
	page, err := strconv.Atoi(progress)
	if err != nil || page < 1 {
		return models.ReadingLocator{}, false
	}
	page--
	return models.ReadingLocator{Page: &page}, true
}

// retrieveProgress answers a pull with the position KOReader can use, or its
// "no progress" response.
func (h *handler) retrieveProgress(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	document := c.Param("document")
	file, err := h.resolve(ctx, user, device, document)
	if err != nil {
		return err
	}
	if file == nil {
		logUnknownDocument(ctx, device, document, "")
		return errors.WithStack(c.JSON(http.StatusOK, ProgressResponse{}))
	}
	position, err := h.readingService.RetrievePosition(ctx, user.ID, file.FileID)
	if err != nil {
		return err
	}
	response, err := h.buildProgress(ctx, user, device, document, file, position)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, response))
}

// buildProgress is what device gets for position in the copy file names.
//
// A position another source wrote is labelled with that source's identity in
// Shisho, so KOReader takes it for another device's and offers the jump.
func (h *handler) buildProgress(ctx context.Context, user *models.User, device *devices.Device, document string, file *models.DocumentHash, position *models.ReadingPosition) (ProgressResponse, error) {
	if position == nil {
		return ProgressResponse{}, nil
	}
	response := ProgressResponse{Document: document, Percentage: &position.Percentage}

	// One Device can hold a File both as itself and converted, such as a CBZ
	// and its EPUB. A position in the other's form means nothing to this copy,
	// and KOReader can't go to it.
	pageBased := models.IsPageBasedFileType(file.DeliveredFileType)
	if pageBased && position.Page == nil || !pageBased && position.XPointer == nil {
		return ProgressResponse{}, nil
	}
	if pageBased {
		response.Progress = strconv.Itoa(*position.Page + 1)
	} else {
		response.Progress = *position.XPointer
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

	// Pages mean the same to every reader. A reflowable position written
	// elsewhere has no form KOReader can read yet.
	if !pageBased {
		return ProgressResponse{}, nil
	}
	response.Timestamp = &timestamp
	switch {
	case position.SourceDeviceID != nil:
		source, err := h.deviceService.Retrieve(ctx, user.ID, *position.SourceDeviceID)
		if err != nil {
			return ProgressResponse{}, err
		}
		response.Device, response.DeviceID = source.Name, source.ID
	case position.Source == models.ReadingSourceKOReader:
		response.Device, response.DeviceID = removedDeviceName, removedDeviceID
	default:
		response.Device, response.DeviceID = webReaderDeviceName, webReaderDeviceID
	}
	return response, nil
}

// resolve returns the File a document hash names for this request, or nil
// when it names none the request reaches. Identical bytes can map to several
// Files, so it's the ones the user's library access and the Device's scope
// reach, preferring one the user already has a position in, so reading doesn't
// split across them, and then the oldest.
func (h *handler) resolve(ctx context.Context, user *models.User, device *devices.Device, document string) (*models.DocumentHash, error) {
	candidates, err := h.fingerprintService.FindDocumentHash(ctx, document)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	r, err := h.reachService.Scope(ctx, user, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var first *models.DocumentHash
	for _, candidate := range candidates {
		if candidate.File == nil || !user.HasLibraryAccess(candidate.File.LibraryID) {
			continue
		}
		holds, err := h.reachService.HoldsBook(ctx, r, candidate.File.LibraryID, candidate.File.BookID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !holds {
			continue
		}
		position, err := h.readingService.RetrievePosition(ctx, user.ID, candidate.FileID)
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
	return first, nil
}

// logUnknownDocument names a document Shisho can't place, for an operator
// working out why a book won't sync. KOReader sends a title only with its
// "send metadata" option on, and only with a push.
func logUnknownDocument(ctx context.Context, device *devices.Device, document, metadata string) {
	logger.FromContext(ctx).Info("koreader document not recognized", logger.Data{
		"document":  document,
		"device_id": device.ID,
		"metadata":  metadata,
	})
}
