package plugins

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

const validRepoURLPrefix = "https://raw.githubusercontent.com/"

type addRepoPayload struct {
	URL   string `json:"url" validate:"required,url"`
	Scope string `json:"scope" validate:"required"`
}

// syncRepositoryResponse is the JSON body returned from the sync endpoint.
// Embeds the repository so its fields stay at the top level for
// backwards-compat with clients reading them, and adds an optional
// update_refresh_error populated when the post-sync update refresh failed.
type syncRepositoryResponse struct {
	*models.PluginRepository
	UpdateRefreshError *string `json:"update_refresh_error,omitempty"`
}

func isValidRepoURL(url string) bool {
	return strings.HasPrefix(url, validRepoURLPrefix)
}

func (h *handler) listRepositories(c echo.Context) error {
	ctx := c.Request().Context()

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, repos))
}

func (h *handler) addRepository(c echo.Context) error {
	ctx := c.Request().Context()

	var payload addRepoPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.URL == "" || payload.Scope == "" {
		return errcodes.ValidationError("URL and scope are required.")
	}

	if !isValidRepoURL(payload.URL) {
		return &errcodes.Error{
			HTTPCode: http.StatusBadRequest,
			Message:  "Invalid repository URL. Only GitHub raw content URLs are allowed (https://raw.githubusercontent.com/...).",
			Code:     "invalid_repo_url",
		}
	}

	repo := &models.PluginRepository{
		URL:        payload.URL,
		Scope:      payload.Scope,
		IsOfficial: false,
		Enabled:    true,
	}

	if err := h.service.AddRepository(ctx, repo); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, repo))
}

func (h *handler) removeRepository(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")

	if err := h.service.RemoveRepository(ctx, scope); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) syncRepository(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")

	repo, err := h.service.GetRepository(ctx, scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Repository")
		}
		return errors.WithStack(err)
	}

	manifest, fetchErr := FetchRepository(repo.URL)

	now := time.Now()
	repo.LastFetchedAt = &now

	if fetchErr != nil {
		errMsg := fetchErr.Error()
		repo.FetchError = &errMsg
		if err := h.service.UpdateRepository(ctx, repo); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(c.JSON(http.StatusOK, syncRepositoryResponse{PluginRepository: repo}))
	}

	// Update repository metadata from manifest
	repo.Name = &manifest.Name
	repo.FetchError = nil

	if err := h.service.UpdateRepository(ctx, repo); err != nil {
		return errors.WithStack(err)
	}

	// Re-evaluate update_available_version for installed plugins from this
	// repo using the manifest we already fetched above — avoids a second
	// round of network fetches against every other enabled repository and
	// keeps the refresh scoped to what actually changed.
	var updateRefreshError *string
	if h.manager != nil {
		if err := h.manager.CheckForUpdatesForRepo(ctx, scope, manifest); err != nil {
			logger.FromContext(ctx).Warn("failed to refresh plugin updates after repo sync", logger.Data{
				"scope": scope,
				"error": err.Error(),
			})
			msg := err.Error()
			updateRefreshError = &msg
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, syncRepositoryResponse{
		PluginRepository:   repo,
		UpdateRefreshError: updateRefreshError,
	}))
}
