package kobo

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
)

// handleRetrieveReadingState handles GET /v1/library/:bookId/state: a Shisho
// book's reading state, or, for a book from Kobo's store, the store's.
func (h *handler) handleRetrieveReadingState(c echo.Context) error {
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.requireEntry(c, entryID)
	if err != nil {
		return err
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
func (h *handler) handleUpdateReadingState(c echo.Context) error {
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.requireEntry(c, entryID)
	if err != nil {
		return err
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
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
	var state *ReadingState
	for _, s := range params.ReadingStates {
		if s != nil && (state == nil || s.EntitlementID == entry.EntryID) {
			state = s
		}
	}
	if state == nil {
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}
	if err := h.states.apply(c.Request().Context(), user, device, entry, state); err != nil {
		return err
	}
	response.UpdateResults = append(response.UpdateResults, updateResult(entry.EntryID, state))
	return errors.WithStack(c.JSON(http.StatusOK, response))
}
