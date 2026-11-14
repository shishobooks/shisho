package koreaderplugin

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/koreader"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
)

const (
	// clockTolerance is how far off a device's clock can be before the
	// server corrects the times it sends. Less than this is ordinary drift,
	// and correcting it would only add the network's delay.
	clockTolerance = 5 * time.Minute
)

// earliestInstant is the floor on any time a device reports: anything
// earlier is a clock reset, as after a battery runs flat.
var earliestInstant = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// deviceClock converts between a device's clock and the server's for one
// request (ADR 0016).
type deviceClock struct {
	// offset is the server's clock minus the device's, and shift what's added
	// to the device's times: the offset when it's past clockTolerance.
	offset  int64
	shift   int64
	arrival time.Time
	// floor is the earliest instant accepted.
	floor time.Time
}

func newDeviceClock(device *devices.Device, clock int64, now time.Time) deviceClock {
	c := deviceClock{offset: now.Unix() - clock, arrival: now, floor: earliestInstant}
	if time.Duration(abs(c.offset))*time.Second > clockTolerance {
		c.shift = c.offset
	}
	if device.CreatedAt.After(c.floor) {
		c.floor = device.CreatedAt
	}
	return c
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// instant is when deviceTime happened, after correcting the clock, or false
// for one that can't be right: before the year 2000 or before the Device
// existed. One after the request arrived is taken as its arrival.
func (c deviceClock) instant(deviceTime int64) (time.Time, bool) {
	t := time.Unix(deviceTime+c.shift, 0).UTC()
	if t.Before(c.floor) {
		return time.Time{}, false
	}
	if t.After(c.arrival) {
		t = c.arrival.UTC()
	}
	return t, true
}

// onDevice is instant on the device's clock, the inverse of instant.
func (c deviceClock) onDevice(instant time.Time) int64 {
	return instant.Unix() - c.shift
}

// upload takes in a batch of page rows and journal events from the plugin
// (ADR 0016). Everything is acknowledged, including what's ignored: a
// repeat, a time that can't be right, or a document Shisho doesn't
// recognize. That document's rows are stored ungrouped and count toward
// nothing, so linking the copy to a File can group them later (ADR 0017).
func (h *handler) upload(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	var payload UploadPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	now := time.Now()
	clock := newDeviceClock(device, payload.Clock, now)
	files := map[string]*models.DocumentHash{}
	resolve := func(document string) (*models.DocumentHash, error) {
		if file, ok := files[document]; ok {
			return file, nil
		}
		file, err := h.documents.Resolve(ctx, user, device, document)
		if err != nil {
			return nil, err
		}
		files[document] = file
		return file, nil
	}

	// Whether Shisho changed a status or Rating is judged by what it held
	// before this upload, since the device's own pages and positions in it
	// can resume or finish a read before its status change is applied.
	before, err := h.statesBefore(ctx, user, payload.Events, resolve)
	if err != nil {
		return err
	}

	refused := 0
	var order []string
	byDocument := map[string][]reading.PageRow{}
	for _, row := range payload.Pages {
		at, ok := clock.instant(row.StartTime)
		if !ok {
			refused++
			continue
		}
		if _, seen := byDocument[row.Document]; !seen {
			order = append(order, row.Document)
		}
		// A visit can't end after it arrived, any more than it can start.
		duration := row.Duration
		if latest := int(clock.arrival.Sub(at) / time.Second); duration > latest {
			duration = max(0, latest)
		}
		byDocument[row.Document] = append(byDocument[row.Document], reading.PageRow{
			Document: row.Document, Page: row.Page, TotalPages: row.TotalPages,
			StartTime: row.StartTime, StartedAt: at, Duration: duration,
		})
	}
	for _, document := range order {
		file, err := resolve(document)
		if err != nil {
			return err
		}
		var fileID *int
		if file != nil {
			fileID = &file.FileID
		}
		if err := h.reading.RecordPageRows(ctx, user.ID, device.ID, fileID, byDocument[document]); err != nil {
			return err
		}
	}

	acknowledged := make([]string, 0, len(payload.Events))
	for _, event := range payload.Events {
		ok, err := h.applyEvent(ctx, user, device, clock, event, resolve, before)
		if err != nil {
			return err
		}
		if !ok {
			refused++
		}
		acknowledged = append(acknowledged, event.ID)
	}
	if refused > 0 {
		logger.FromContext(ctx).Warn("koreader plugin times refused", logger.Data{
			"device_id": device.ID, "refused": refused, "clock_offset": clock.offset,
		})
	}

	if err := h.devices.RecordPluginSync(ctx, device.ID, now); err != nil {
		return errors.WithStack(err)
	}
	if err := h.devices.RecordPluginConnection(ctx, device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, UploadResponse{
		Server:      h.serverInfo(),
		ClockOffset: clock.offset,
		Pages:       len(payload.Pages),
		Events:      acknowledged,
	}))
}

// applyEvent takes in one journal event, once. It returns false for one
// refused for its time.
func (h *handler) applyEvent(ctx context.Context, user *models.User, device *devices.Device, clock deviceClock, event EventPayload,
	resolve func(string) (*models.DocumentHash, error), before map[int]*reading.StatusSummary) (bool, error) {
	exists, err := h.db.NewSelect().
		Model((*models.KOReaderPluginEvent)(nil)).
		Where("kpe.device_id = ? AND kpe.event_id = ?", device.ID, event.ID).
		Exists(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	if exists {
		return true, nil
	}
	var ok bool
	switch event.Type {
	case PluginEventStatus, PluginEventRating:
		ok, err = h.applyChange(ctx, user, clock, event, resolve, before)
		if err != nil {
			return false, err
		}
	default:
		var at time.Time
		at, ok = clock.instant(event.At)
		if ok {
			if err := h.applyPosition(ctx, user, device, event, at, resolve); err != nil {
				return false, err
			}
		}
	}
	// Recorded once applied, so a failure partway is retried. Applying a
	// position, a status, or a Rating twice changes nothing.
	_, err = h.db.NewInsert().
		Model(&models.KOReaderPluginEvent{DeviceID: device.ID, EventID: event.ID, CreatedAt: time.Now()}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return ok, errors.WithStack(err)
}

// applyPosition sends a position event through the update service, at the
// time it happened.
func (h *handler) applyPosition(ctx context.Context, user *models.User, device *devices.Device, event EventPayload, at time.Time,
	resolve func(string) (*models.DocumentHash, error)) error {
	file, err := resolve(event.Document)
	if err != nil || file == nil {
		return err
	}
	locator, ok := koreader.ParseProgress(event.Progress)
	if !ok || event.Percentage == nil {
		// Refusing it would leave it queued on the device for good.
		logger.FromContext(ctx).Warn("koreader plugin position not readable", logger.Data{
			"device_id": device.ID, "event_id": event.ID,
		})
		return nil
	}
	update := reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     file.FileID,
		Source:     models.ReadingSourceKOReader,
		DeviceID:   &device.ID,
		Locator:    locator,
		Percentage: *event.Percentage,
		AtEnd:      *event.Percentage >= koreader.FinishedAt,
		OccurredAt: &at,
	}
	if file.File != nil && file.DeliveredFileType != file.File.FileType {
		update.LocatorFileType = &file.DeliveredFileType
	}
	_, err = h.reading.UpdatePosition(ctx, update)
	return err
}

// pull answers with the position of each named book that has one the
// device can go to, translated for its copy and timed on its clock, and
// with the statuses and Ratings the device should hold (pullStates). The
// cursor is taken before the states are read, so nothing changed while
// they are is missed.
func (h *handler) pull(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	var payload PullPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	clock := newDeviceClock(device, payload.Clock, time.Now())
	positions := []PulledPosition{}
	for _, document := range payload.Documents {
		file, err := h.documents.Resolve(ctx, user, device, document)
		if err != nil {
			return err
		}
		if file == nil {
			continue
		}
		stored, err := h.reading.RetrievePosition(ctx, user.ID, file.FileID)
		if err != nil {
			return err
		}
		if stored == nil {
			continue
		}
		progress, err := h.documents.Progress(ctx, user, device, document, file, stored)
		if err != nil {
			return err
		}
		if progress.Progress == "" || progress.Percentage == nil {
			continue
		}
		own := stored.SourceDeviceID != nil && *stored.SourceDeviceID == device.ID
		source := progress.Device
		if own {
			source = device.Name
		}
		positions = append(positions, PulledPosition{
			Document:   document,
			Progress:   progress.Progress,
			Percentage: *progress.Percentage,
			Timestamp:  clock.onDevice(stored.OccurredAt),
			Source:     source,
			Own:        own,
		})
	}
	cursor := time.Now().Unix()
	states, err := h.pullStates(ctx, user, device, payload)
	if err != nil {
		return err
	}
	if err := h.devices.RecordPluginConnection(ctx, device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, PullResponse{
		Server:      h.serverInfo(),
		ClockOffset: clock.offset,
		Positions:   positions,
		States:      states,
		Cursor:      cursor,
	}))
}
