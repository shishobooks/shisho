package delivery

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	service *Service
}

func buildDefaultRulesResponse(rules *UserRules) DefaultRulesResponse {
	builtIn := BuiltInRules()
	options := make([]RuleOption, len(builtIn))
	for i, rule := range builtIn {
		options[i] = RuleOption{
			SourceFileType:     rule.SourceFileType,
			DeliveredFileTypes: models.ReachableFileTypes(rule.SourceFileType),
		}
	}
	return DefaultRulesResponse{
		FollowsBuiltIn: rules.FollowsBuiltIn,
		Rules:          rules.Rules,
		Options:        options,
	}
}

// retrieve returns the current user's default Delivery Rules.
func (h *handler) retrieve(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rules, err := h.service.UserDefault(c.Request().Context(), user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, buildDefaultRulesResponse(rules))
}

// update replaces the current user's default Delivery Rules with a
// customized list.
func (h *handler) update(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	var payload UpdateDeliveryRulesPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	rules, err := h.service.SaveUserDefault(c.Request().Context(), user.ID, payload.Rules)
	var rulesErr *RulesError
	if errors.As(err, &rulesErr) {
		return errcodes.ValidationError(rulesErr.Reason)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, buildDefaultRulesResponse(rules))
}

// reset makes the current user's default follow the built-in rules again.
func (h *handler) reset(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rules, err := h.service.ResetUserDefault(c.Request().Context(), user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, buildDefaultRulesResponse(rules))
}
