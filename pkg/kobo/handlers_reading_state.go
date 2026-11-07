package kobo

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
)

// stateEntry returns the Device's entry entryID while it's in scope, or nil.
// Unlike requireEntry, an entry out of scope isn't an error: the Kobo keeps a
// book until the next sync removes it, and a Kobo treats a refused update
// from its library as a failed sync, which would stop that removal arriving.
func (h *handler) stateEntry(c echo.Context, entryID string) (*SyncEntry, error) {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return nil, err
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	entry, err := h.service.EntryInScope(c.Request().Context(), user, device, entryID)
	return entry, errors.WithStack(err)
}

// handleRetrieveReadingState handles GET /v1/library/:bookId/state: a Shisho
// book's reading state, none for one that has left the Device's scope, or,
// for a book from Kobo's store, the store's.
func (h *handler) handleRetrieveReadingState(c echo.Context) error {
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.stateEntry(c, entryID)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.WithStack(c.JSON(http.StatusOK, []*ReadingState{}))
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	state, err := h.states.retrieve(c.Request().Context(), user.ID, entry)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, []*ReadingState{state}))
}

// handleUpdateReadingState handles PUT /v1/library/:bookId/state, where a
// Kobo reports a book's status, reading time, and bookmark. A Shisho book's
// goes into Shisho's reading record; a store book's goes to Kobo's store.
// One that has left the Device's scope, or a state the body gives for
// another book, is accepted and changes nothing.
func (h *handler) handleUpdateReadingState(c echo.Context) error {
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.stateEntry(c, entryID)
	if err != nil {
		return err
	}
	// Kobo firmware adds fields of its own, which mustn't refuse its updates.
	c.Set("disallow_unknown_fields", false) //nolint:forbidigo // A binder option, not auth state.
	params := UpdateReadingStatesPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	response := UpdateReadingStatesResponse{RequestResult: updateSucceeded, UpdateResults: []*ReadingStateUpdateResult{}}
	if entry == nil {
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}
	var state *ReadingState
	for _, s := range params.ReadingStates {
		if s != nil && s.EntitlementID == entry.EntryID {
			state = s
		}
	}
	if state == nil {
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	if err := h.states.apply(c.Request().Context(), user, device, entry, state); err != nil {
		return err
	}
	response.UpdateResults = append(response.UpdateResults, updateResult(entry.EntryID, state))
	return errors.WithStack(c.JSON(http.StatusOK, response))
}
