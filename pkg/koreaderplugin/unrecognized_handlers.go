package koreaderplugin

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// lookup answers what the server knows of each copy the plugin asks about,
// so it reports a copy as unrecognized only when no server it signed in to
// recognizes it (ADR 0017).
func (h *handler) lookup(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	var payload LookupPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	documents := make([]DocumentLookedUp, 0, len(payload.Documents))
	for _, document := range payload.Documents {
		state, err := h.lookUp(ctx, user, device, document)
		if err != nil {
			return err
		}
		documents = append(documents, DocumentLookedUp{Document: document, State: state})
	}
	undescribed, err := h.undescribed(ctx, user, device)
	if err != nil {
		return err
	}
	if err := h.devices.RecordPluginConnection(ctx, device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, LookupResponse{Server: h.serverInfo(), Documents: documents, Undescribed: undescribed}))
}

// ownDevice is the signed-in user and their Device the request's :id
// names, which must be a KOReader Device. Another user's Device is not
// found.
func (h *handler) ownDevice(c echo.Context) (*models.User, *devices.Device, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, nil, err
	}
	device, err := h.devices.Retrieve(c.Request().Context(), user.ID, c.Param("id"))
	if errors.Is(err, devices.ErrNotFound) {
		return nil, nil, errcodes.NotFound("Device")
	}
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if !device.HasConnection(devices.ConnectionKOReaderPlugin) {
		return nil, nil, errcodes.InvalidState("Only a KOReader Device can use the KOReader plugin.")
	}
	return user, device, nil
}

// unrecognized lists a KOReader Device's unrecognized copies, with Files
// to link each to.
func (h *handler) unrecognized(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	books, err := h.listUnrecognized(c.Request().Context(), user, device)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, books))
}

// linkCandidates searches the Files the Device reaches for one to link a
// copy to.
func (h *handler) linkCandidates(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	var query LinkCandidatesQuery
	if err := c.Bind(&query); err != nil {
		return errors.WithStack(err)
	}
	ctx := c.Request().Context()
	scope, err := h.linkScope(ctx, user, device)
	if err != nil {
		return err
	}
	files, err := h.searchFiles(ctx, scope, query.Q, maxCandidates)
	if err != nil {
		return err
	}
	candidates, err := h.candidates(ctx, files, LinkMatchSearch)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, candidates))
}

// linkUnrecognized links an unrecognized copy to a File, applying what was
// held for it at once.
func (h *handler) linkUnrecognized(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	var payload LinkPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	err = h.link(c.Request().Context(), user, device, c.Param("document"), payload.FileID)
	switch {
	case errors.Is(err, errCopyNotFound):
		return errcodes.NotFound("Unrecognized book")
	case errors.Is(err, errFileNotFound):
		return errcodes.NotFound("File")
	case errors.Is(err, errFileNotLinkable):
		return errcodes.InvalidState("A KOReader copy can only be linked to a book's main ebook or comic file.")
	case err != nil:
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// unlinkUnrecognized undoes a link, so the copy is one to link again.
func (h *handler) unlinkUnrecognized(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	err = h.unlink(c.Request().Context(), user, device, c.Param("document"))
	if errors.Is(err, errCopyNotFound) {
		return errcodes.NotFound("Linked book")
	}
	if err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// dismissAllUnrecognized dismisses the copies still to link that the
// Device's page listed.
func (h *handler) dismissAllUnrecognized(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	var payload DismissAllPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	if err := h.dismissAll(c.Request().Context(), user, device, payload.Documents); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// dismissUnrecognized dismisses an unrecognized copy, deleting what was
// held for it.
func (h *handler) dismissUnrecognized(c echo.Context) error {
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
	}
	err = h.dismiss(c.Request().Context(), user, device, c.Param("document"))
	if errors.Is(err, errCopyNotFound) {
		return errcodes.NotFound("Unrecognized book")
	}
	if err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}
