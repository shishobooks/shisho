package shishoapp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
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
	"github.com/uptrace/bun"
)

const (
	// clockTolerance is how far off a device's clock can be before its times
	// are corrected, as for the KOReader plugin (ADR 0016): less is ordinary
	// drift, and correcting it would only add the network's delay.
	clockTolerance = 5 * time.Minute
	// maxSessionLength bounds a Reading Session, as for the web reader: one
	// ends after a few idle minutes, so one longer than a day is wrong.
	maxSessionLength = 24 * time.Hour
	// maxEventID is the longest event id the server keeps.
	maxEventID = 64
)

// journalEvent records that an installation's event was taken in, so a
// repeat is ignored (ADR 0024).
type journalEvent struct {
	bun.BaseModel `bun:"table:app_journal_events,alias:aje"`

	UserID         int       `bun:"user_id,pk"`
	InstallationID string    `bun:"installation_id,pk"`
	EventID        string    `bun:"event_id,pk"`
	EventType      string    `bun:"event_type,notnull"`
	CreatedAt      time.Time `bun:"created_at,notnull"`
}

// deviceClock converts the times of one batch from the device's clock to
// the server's.
type deviceClock struct {
	// offset is the server's clock minus the device's, and shift what's added
	// to the device's times: the offset when it's past clockTolerance.
	offset, shift time.Duration
	arrival       time.Time
	// floor is the earliest instant accepted: when the user's account was
	// made, since an installation's journal may predate its current Device
	// (ADR 0024).
	floor time.Time
}

func newDeviceClock(device, now, floor time.Time) deviceClock {
	c := deviceClock{offset: now.Sub(device), arrival: now, floor: floor}
	if c.offset > clockTolerance || c.offset < -clockTolerance {
		c.shift = c.offset
	}
	return c
}

// instant is when deviceTime happened on the server's clock, or false for a
// time before the floor. One after the batch arrived is taken as its arrival.
func (c deviceClock) instant(deviceTime time.Time) (time.Time, bool) {
	t := deviceTime.Add(c.shift).UTC()
	// The account's creation is stored to the second.
	if t.Before(c.floor.Truncate(time.Second)) {
		return time.Time{}, false
	}
	if t.After(c.arrival) {
		t = c.arrival.UTC()
	}
	return t, true
}

// eventOutcome is what applying an understood event did: nothing refused it,
// or it was refused for reason.
type eventOutcome struct {
	refused string
}

// journal takes in a batch of the app's journal (ADR 0024). Each event is
// judged on its own, so an unknown type or field is "not understood" for
// that event alone and the app keeps it queued. An event this installation
// sent before, under any of its sign-ins, is a duplicate. One understood and
// invalid, such as one for a deleted File, is refused with a reason. Times
// are corrected for the device's clock and each event is placed by its own
// time through the reading update service.
func (h *handler) journal(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	// Events are decoded one by one below, strictly; only the batch's own
	// fields are bound here.
	c.Set("disallow_unknown_fields", false) //nolint:forbidigo // A binder option, not auth state.
	var payload JournalPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	// Events are kept by installation, so a journal resent after signing in
	// again as a new Device is applied once.
	installation := device.ID
	if device.InstallationID != nil && *device.InstallationID != "" {
		installation = *device.InstallationID
	}
	clock := newDeviceClock(payload.Clock, time.Now(), user.CreatedAt)
	// An event started is finished even if the app gives up on the batch,
	// such as when the network goes; the rest of the batch isn't started.
	requestCtx := ctx
	ctx = context.WithoutCancel(ctx)

	resp := JournalResponse{Results: make([]JournalResult, 0, len(payload.Events)), ClockOffsetSeconds: clock.offset.Seconds()}
	counts := map[string]int{}
	for _, raw := range payload.Events {
		if err := requestCtx.Err(); err != nil {
			return errors.WithStack(err)
		}
		result, err := h.takeEvent(ctx, c, user, device, installation, clock, raw)
		if err != nil {
			return err
		}
		counts[result.Result]++
		resp.Results = append(resp.Results, result)
	}
	if counts[JournalRefused]+counts[JournalNotUnderstood] > 0 {
		logger.FromContext(ctx).Warn("shisho app journal events not taken in", logger.Data{
			"device_id": device.ID, "refused": counts[JournalRefused], "not_understood": counts[JournalNotUnderstood],
			"clock_offset_seconds": int(clock.offset.Seconds()),
		})
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

var unknownField = regexp.MustCompile(`^json: unknown field "(.*)"$`)

// decodeStrict decodes data into v, refusing fields v doesn't have. It
// returns the unknown field's name when that's why it failed.
func decodeStrict(data []byte, v any) (unknown string, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
			return m[1], err
		}
		return "", err
	}
	return "", nil
}

// takeEvent judges and applies one event, returning its result. Only a
// server fault is an error, which fails the batch so the app keeps it all.
func (h *handler) takeEvent(ctx context.Context, c echo.Context, user *models.User, device *devices.Device,
	installation string, clock deviceClock, raw json.RawMessage) (JournalResult, error) {
	var head struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &head); err != nil || strings.TrimSpace(head.ID) == "" {
		return JournalResult{Result: JournalRefused, Reason: "The event has no id."}, nil
	}
	result := JournalResult{ID: head.ID}
	if len(head.ID) > maxEventID {
		result.Result, result.Reason = JournalRefused, fmt.Sprintf("The event's id is longer than %d characters.", maxEventID)
		return result, nil
	}
	refuse := func(reason string) JournalResult {
		result.Result, result.Reason = JournalRefused, reason
		return result
	}
	notUnderstood := func(reason string) JournalResult {
		result.Result, result.Reason = JournalNotUnderstood, reason
		return result
	}

	unreadable := func(err error) JournalResult {
		logger.FromContext(ctx).Warn("shisho app journal event not readable", logger.Data{
			"device_id": device.ID, "event_id": result.ID, "error": err.Error(),
		})
		return refuse("The event isn't readable.")
	}

	var ev JournalEvent
	if field, err := decodeStrict(raw, &ev); err != nil {
		if field != "" {
			return notUnderstood(fmt.Sprintf("This server doesn't know the event field %q.", field)), nil
		}
		return unreadable(err), nil
	}
	reject := func(problem *payloadProblem) JournalResult {
		switch {
		case problem.unknownField != "":
			return notUnderstood(fmt.Sprintf("This server doesn't know the %s field %q.", ev.Type, problem.unknownField))
		case problem.invalid != "":
			return refuse(problem.invalid)
		default:
			return unreadable(problem.err)
		}
	}
	var apply func(claim claimFunc) (eventOutcome, error)
	switch ev.Type {
	case JournalEventReadingPosition:
		p, problem := decodePayload[JournalPositionPayload](c, ev)
		if problem != nil {
			return reject(problem), nil
		}
		apply = func(claim claimFunc) (eventOutcome, error) {
			return h.applyPosition(ctx, user, device, clock, ev.At, p, claim)
		}
	case JournalEventReadingSession:
		p, problem := decodePayload[JournalSessionPayload](c, ev)
		if problem != nil {
			return reject(problem), nil
		}
		apply = func(claim claimFunc) (eventOutcome, error) {
			return h.applySession(ctx, user, device, installation, clock, p, claim)
		}
	case JournalEventBookmarkCreate:
		p, problem := decodePayload[JournalBookmarkCreatePayload](c, ev)
		if problem != nil {
			return reject(problem), nil
		}
		apply = func(claim claimFunc) (eventOutcome, error) {
			return h.applyBookmarkCreate(ctx, user, clock, ev.At, p, claim)
		}
	case JournalEventBookmarkNote:
		p, problem := decodePayload[JournalBookmarkNotePayload](c, ev)
		if problem != nil {
			return reject(problem), nil
		}
		apply = func(claim claimFunc) (eventOutcome, error) {
			return h.applyBookmarkNote(ctx, user, clock, ev.At, p, claim)
		}
	case JournalEventBookmarkDelete:
		p, problem := decodePayload[JournalBookmarkRef](c, ev)
		if problem != nil {
			return reject(problem), nil
		}
		apply = func(claim claimFunc) (eventOutcome, error) {
			return h.applyBookmarkDelete(ctx, user, clock, ev.At, p, claim)
		}
	default:
		return notUnderstood(fmt.Sprintf("This server doesn't know the event type %q.", ev.Type)), nil
	}
	if ev.At.IsZero() {
		return refuse("The event has no time."), nil
	}

	// An event taken in before is a duplicate, even if what it names is
	// gone since.
	seen, err := h.db.NewSelect().Model((*journalEvent)(nil)).
		Where("user_id = ? AND installation_id = ? AND event_id = ?", user.ID, installation, ev.ID).
		Exists(ctx)
	if err != nil {
		return result, errors.WithStack(err)
	}
	if seen {
		result.Result = JournalDuplicate
		return result, nil
	}
	// Claimed in the same transaction that applies it, so it's recorded
	// exactly when it's applied: a refusal or a fault leaves it unclaimed,
	// and a batch sent twice at once applies it once.
	claim := func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewInsert().
			Model(&journalEvent{UserID: user.ID, InstallationID: installation, EventID: ev.ID, EventType: ev.Type, CreatedAt: time.Now()}).
			On("CONFLICT DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return errors.WithStack(err)
		} else if n == 0 {
			return errDuplicate
		}
		return nil
	}
	outcome, err := apply(claim)
	switch {
	case errors.Is(err, errDuplicate):
		result.Result = JournalDuplicate
		return result, nil
	case err != nil:
		return result, err
	case outcome.refused != "":
		return refuse(outcome.refused), nil
	}
	result.Result = JournalAccepted
	return result, nil
}

// payloadProblem is why an event's payload can't be taken in: a field this
// server doesn't know, an invalid value, or a payload that isn't readable.
type payloadProblem struct {
	unknownField string
	invalid      string
	err          error
}

// decodePayload decodes ev's payload strictly and validates it as the binder
// would.
func decodePayload[T any](c echo.Context, ev JournalEvent) (T, *payloadProblem) {
	var p T
	if field, err := decodeStrict(ev.Payload, &p); err != nil {
		if field != "" {
			return p, &payloadProblem{unknownField: field}
		}
		return p, &payloadProblem{err: err}
	}
	if err := validate(c, &p); err != nil {
		return p, &payloadProblem{invalid: err.Error()}
	}
	return p, nil
}

// claimFunc records an event as taken in, inside the transaction that
// applies it.
type claimFunc = func(ctx context.Context, tx bun.Tx) error

// errDuplicate is a claim on an event already taken in.
var errDuplicate = errors.New("journal event already taken in")

// validate checks v against its validate tags as the binder would.
func validate(c echo.Context, v any) error {
	type validator interface{ Validate(i interface{}) error }
	if b, ok := c.Echo().Binder.(validator); ok {
		if err := b.Validate(v); err != nil {
			var coded *errcodes.Error
			if errors.As(err, &coded) {
				return errors.New(coded.Message)
			}
			return err
		}
	}
	return nil
}

const (
	refusedFile          = "The File doesn't exist, or you can no longer reach it."
	refusedBeforeAccount = "The event is from before your account was made, so the device's clock was wrong."
)

// readableFile reports whether user can reach fileID.
func (h *handler) readableFile(ctx context.Context, user *models.User, fileID int) (bool, error) {
	libraryID, err := h.reading.FileLibraryID(ctx, fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, errors.WithStack(err)
	}
	return user.HasLibraryAccess(libraryID), nil
}

// applyPosition sends a Reading Position update through the update
// service at the time it happened, which starts, resumes, and finishes
// Read-throughs by the same rules as the web player.
func (h *handler) applyPosition(ctx context.Context, user *models.User, device *devices.Device, clock deviceClock,
	at time.Time, p JournalPositionPayload, claim claimFunc) (eventOutcome, error) {
	forms := 0
	for _, given := range []bool{p.CFI != nil, p.Page != nil, p.Seconds != nil} {
		if given {
			forms++
		}
	}
	if forms != 1 {
		return eventOutcome{refused: "A position has exactly one of cfi, page, or seconds."}, nil
	}
	if ok, err := h.readableFile(ctx, user, p.FileID); err != nil || !ok {
		return eventOutcome{refused: refusedFile}, err
	}
	occurred, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	deviceID := device.ID
	update := reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     p.FileID,
		Source:     models.ReadingSourceShishoApp,
		DeviceID:   &deviceID,
		Locator:    models.ReadingLocator{CFI: p.CFI, Page: p.Page, Seconds: p.Seconds},
		Percentage: p.Percentage,
		AtEnd:      p.AtEnd,
		OccurredAt: &occurred,
		Claim:      claim,
	}
	if p.ListenedSeconds != nil {
		update.ListenedSeconds = *p.ListenedSeconds
	}
	// It backdates a Read-through, so a sitting that can't be right, from
	// before the account or more than a day before the update, is left out.
	if p.ListeningSince != nil {
		if since, ok := clock.instant(*p.ListeningSince); ok && !since.After(occurred) && occurred.Sub(since) <= maxSessionLength {
			update.ListeningSince = &since
		}
	}
	_, err := h.reading.UpdatePosition(ctx, update)
	if errors.Is(err, reading.ErrFileNotFound) {
		return eventOutcome{refused: refusedFile}, nil
	}
	return eventOutcome{}, err
}

// journalSession links the snapshots sent under the device's id for a
// session to the Reading Session they update; ReadingSessionID is nil once
// the user deleted it.
type journalSession struct {
	bun.BaseModel `bun:"table:app_journal_sessions,alias:ajs"`

	UserID           int       `bun:"user_id,pk"`
	InstallationID   string    `bun:"installation_id,pk"`
	SessionID        string    `bun:"session_id,pk"`
	ReadingSessionID *int      `bun:"reading_session_id"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
}

// applySession records a Reading Session at the time it happened,
// on the Read-through underway then (ADR 0016).
//
// A snapshot named by SessionID updates the session its first snapshot
// made, when it ends later, and a session the user deleted stays deleted
// (ADR 0024).
func (h *handler) applySession(ctx context.Context, user *models.User, device *devices.Device, installation string,
	clock deviceClock, p JournalSessionPayload, claim claimFunc) (eventOutcome, error) {
	if p.EndedAt.Before(p.StartedAt) {
		return eventOutcome{refused: "A Reading Session can't end before it starts."}, nil
	}
	if p.EndedAt.Sub(p.StartedAt) > maxSessionLength {
		return eventOutcome{refused: "A Reading Session lasts at most a day."}, nil
	}
	if ok, err := h.readableFile(ctx, user, p.FileID); err != nil || !ok {
		return eventOutcome{refused: refusedFile}, err
	}
	started, ok := clock.instant(p.StartedAt)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	ended, _ := clock.instant(p.EndedAt)
	deviceID := device.ID
	input := reading.SessionInput{
		StartedAt:       started,
		EndedAt:         ended,
		StartPercentage: p.StartPercentage,
		EndPercentage:   p.EndPercentage,
		Measured:        true,
		DeviceID:        &deviceID,
		Duration:        p.Duration,
		Claim:           claim,
	}
	if p.SessionID != "" {
		link := &journalSession{}
		err := h.db.NewSelect().Model(link).
			Where("user_id = ? AND installation_id = ? AND session_id = ?", user.ID, installation, p.SessionID).
			Scan(ctx)
		switch {
		case err == nil && link.ReadingSessionID == nil:
			// Deleted: the snapshot is taken in and changes nothing.
			return eventOutcome{}, errors.WithStack(h.db.RunInTx(ctx, nil, claim))
		case err == nil:
			return eventOutcome{}, h.reading.ExtendSession(ctx, user.ID, *link.ReadingSessionID, input)
		case !errors.Is(err, sql.ErrNoRows):
			return eventOutcome{}, errors.WithStack(err)
		}
		input.Recorded = func(ctx context.Context, tx bun.Tx, sessionID int) error {
			_, err := tx.NewInsert().Model(&journalSession{
				UserID: user.ID, InstallationID: installation, SessionID: p.SessionID,
				ReadingSessionID: &sessionID, CreatedAt: time.Now(),
			}).Exec(ctx)
			return errors.WithStack(err)
		}
	}
	_, err := h.reading.RecordSession(ctx, user.ID, p.FileID, models.ReadingSourceShishoApp, input)
	if errors.Is(err, reading.ErrFileNotFound) {
		return eventOutcome{refused: refusedFile}, nil
	}
	return eventOutcome{}, err
}
