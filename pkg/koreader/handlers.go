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
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
)

const (
	// webReaderDeviceName and webReaderDeviceID label a position the web
	// reader wrote. KOReader shows the name when it offers the jump.
	webReaderDeviceName = "Shisho web reader"
	webReaderDeviceID   = models.ReadingSourceWebReader
	// removedDeviceName and removedDeviceID label a position from a KOReader
	// Device that's since been deleted, and removedKoboName and removedKoboID
	// one from a deleted Kobo.
	removedDeviceName = "KOReader"
	removedDeviceID   = models.ReadingSourceKOReader
	removedKoboName   = "Kobo"
	removedKoboID     = models.ReadingSourceKobo

	// FinishedAt is the percentage that counts as the end. KOReader's
	// continuous view never reports 100%, and the last pages are back matter,
	// so reaching them finishes the book, as a streaming service counts an
	// episode watched once the credits start.
	FinishedAt = 0.99

	// sessionWindow is how long after a pull a push of the same document
	// from the same Device still makes a Reading Session with it. KOReader
	// pulls on opening or waking and pushes on closing or sleeping, so the
	// pair brackets a sitting; a longer gap is a book left open.
	sessionWindow = 6 * time.Hour
	// replayWindow is how close together pushes from one Device arrive when
	// KOReader sends the ones it queued offline, all at once on reconnecting.
	// KOReader spaces the pushes it makes as you read at least 25 seconds
	// apart, so only a manual push sent twice in a row looks the same.
	replayWindow = 5 * time.Second

	registerRefused = "Shisho doesn't create accounts here. Add a KOReader Device under Devices in Shisho, then log in with your Shisho username and the Device's password."
	loginRefused    = "Wrong username or password. Use your Shisho username and the password on your KOReader Device's page in Shisho."
)

type handler struct {
	deviceService  *devices.Service
	readingService *reading.Service
	docs           *Documents
	pulls          *pullStore
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
// clock can be well off, so the update is dated when it arrives, unless it
// carries a timestamp. A document Shisho doesn't recognize is logged and
// answered as a success: KOReader keeps retrying a refused push for weeks,
// and the book may not be one Shisho has at all.
//
// A push of the document this Device last pulled ends a sitting so far,
// recorded as an estimated Reading Session. Pushes arriving together are
// KOReader replaying what it queued offline: they apply in order, but make
// no sitting, and the first of them, which arrived before the rest showed it
// was a replay, loses the session it started.
func (h *handler) updateProgress(c echo.Context) error {
	ctx := c.Request().Context()
	now := time.Now()
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
	locator, ok := ParseProgress(params.Progress)
	if !ok {
		return errcodes.ValidationError("Progress must be an XPointer or a page number.")
	}

	response := UpdateProgressResponse{Document: params.Document, Timestamp: now.Unix()}
	// A Device syncing through the KOReader plugin reports the same reading
	// there, with its real time, so a push left on beside it would count it
	// twice. It's acknowledged, so KOReader stops retrying it.
	if device.PluginLastSyncedAt != nil {
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	file, err := h.docs.Resolve(ctx, user, device, params.Document)
	if err != nil {
		return err
	}
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
		AtEnd:      *params.Percentage >= FinishedAt,
	}
	if file.File != nil && file.DeliveredFileType != file.File.FileType {
		update.LocatorFileType = &file.DeliveredFileType
	}
	if params.Device != "" {
		update.ReportedDevice = &params.Device
	}
	if params.DeviceID != "" {
		update.ReportedDeviceID = &params.DeviceID
	}
	at := now
	if params.Timestamp != nil {
		// A clock running fast can't date the push, or end its session,
		// after it arrived.
		if stamped := time.Unix(*params.Timestamp, 0); stamped.Before(now) {
			at = stamped
		}
		update.OccurredAt = &at
	}

	// Asked before the update, which would find itself.
	replay, err := h.readingService.DeviceUpdatedSince(ctx, device.ID, now.Add(-replayWindow))
	if err != nil {
		return err
	}
	if _, err := h.readingService.UpdatePosition(ctx, update); err != nil {
		return err
	}
	// The position is saved, which is what KOReader would retry for, so
	// failing to record the sitting only logs.
	if replay {
		err = h.pulls.replay(ctx, user.ID, device.ID, params.Document, now.Add(-replayWindow), now)
	} else {
		err = h.pulls.sitting(ctx, user.ID, device.ID, params.Document, file.FileID, at, *params.Percentage, now)
	}
	if err != nil {
		logger.FromContext(ctx).Warn("koreader sitting not recorded", logger.Data{"device_id": device.ID, "error": err.Error()})
	}
	return errors.WithStack(c.JSON(http.StatusOK, response))
}

// ParseProgress reads KOReader's progress: an XPointer, which is a path, or
// the 1-based page as a string.
func ParseProgress(progress string) (models.ReadingLocator, bool) {
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
	file, err := h.docs.Resolve(ctx, user, device, document)
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
	var percentage *float64
	if position != nil {
		percentage = &position.Percentage
	}
	// The pull only starts a session, which matters less than KOReader
	// getting the position, so failing to note it doesn't fail the pull.
	if err := h.pulls.record(ctx, device.ID, document, file.FileID, percentage, time.Now()); err != nil {
		logger.FromContext(ctx).Warn("koreader pull not recorded", logger.Data{"device_id": device.ID, "error": err.Error()})
	}
	response, err := h.docs.Progress(ctx, user, device, document, file, position)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, response))
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
