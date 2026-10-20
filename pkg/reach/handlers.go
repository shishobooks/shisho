package reach

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
)

type handler struct {
	service       *Service
	bookService   *books.Service
	deviceService *devices.Service
}

// bookDevices returns what each of the current user's Devices gets of a
// book.
func (h *handler) bookDevices(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}
	lines, err := h.service.BookDevices(ctx, user, book)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, lines)
}

// deliveryPreview returns what one of the current user's Devices would
// receive under the rules it follows now, or under the ones in the query.
func (h *handler) deliveryPreview(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var query DeliveryPreviewQuery
	if err := c.Bind(&query); err != nil {
		return errors.WithStack(err)
	}
	device, err := h.deviceService.Retrieve(ctx, user.ID, c.Param("id"))
	if errors.Is(err, devices.ErrNotFound) {
		return errcodes.NotFound("Device")
	}
	if err != nil {
		return errors.WithStack(err)
	}

	var rules []delivery.Rule
	if query.Mode == "" {
		rules, err = h.deviceService.Rules(ctx, device)
	} else {
		var custom []delivery.Rule
		custom, err = parseRules(query.Rules)
		if err == nil {
			rules, err = h.deviceService.RulesInMode(ctx, device, query.Mode, custom)
		}
	}
	var rulesErr *delivery.RulesError
	if errors.As(err, &rulesErr) {
		return errcodes.ValidationError(rulesErr.Reason)
	}
	if err != nil {
		return errors.WithStack(err)
	}

	preview, err := h.service.Preview(ctx, user, device, rules)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, preview)
}

// parseRules reads a list of "source:delivered" pairs separated by commas.
func parseRules(raw string) ([]delivery.Rule, error) {
	rules := []delivery.Rule{}
	if raw == "" {
		return rules, nil
	}
	for _, pair := range strings.Split(raw, ",") {
		source, delivered, ok := strings.Cut(pair, ":")
		if !ok || source == "" || delivered == "" {
			return nil, &delivery.RulesError{Reason: "Each rule needs a source and a delivered File Type."}
		}
		rules = append(rules, delivery.Rule{SourceFileType: source, DeliveredFileType: delivered})
	}
	return rules, nil
}
