package plugins

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// availablePluginResponse is the response format for available plugins.
type availablePluginResponse struct {
	Scope       string                   `json:"scope"`
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Overview    string                   `json:"overview"`
	Description string                   `json:"description"`
	Homepage    string                   `json:"homepage"`
	ImageURL    string                   `json:"imageUrl"`
	IsOfficial  bool                     `json:"is_official"`
	Versions    []AnnotatedPluginVersion `json:"versions"`
	Compatible  bool                     `json:"compatible"`
}

func (h *handler) listIdentifierTypes(c echo.Context) error {
	ctx := c.Request().Context()

	types, err := h.service.ListIdentifierTypes(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, types))
}

func (h *handler) listInstalled(c echo.Context) error {
	ctx := c.Request().Context()

	plugins, err := h.service.ListPlugins(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugins))
}

// listAvailable aggregates plugins from all enabled repositories.
func (h *handler) listAvailable(c echo.Context) error {
	ctx := c.Request().Context()

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	var result []availablePluginResponse

	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}

		manifest, fetchErr := FetchRepository(repo.URL)
		if fetchErr != nil {
			continue
		}

		for _, p := range manifest.Plugins {
			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			annotated := AnnotateVersionCompatibility(compatible)
			hasCompatible := false
			for _, v := range annotated {
				if v.Compatible {
					hasCompatible = true
					break
				}
			}

			result = append(result, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Overview:    p.Overview,
				Description: p.Description,
				Homepage:    p.Homepage,
				ImageURL:    p.ImageURL,
				IsOfficial:  repo.IsOfficial,
				Versions:    annotated,
				Compatible:  hasCompatible,
			})
		}
	}

	if result == nil {
		result = []availablePluginResponse{}
	}

	return errors.WithStack(c.JSON(http.StatusOK, result))
}

// retrieveAvailable returns details for a specific available plugin.
func (h *handler) retrieveAvailable(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, repo := range repos {
		if !repo.Enabled || repo.Scope != scope {
			continue
		}

		manifest, fetchErr := FetchRepository(repo.URL)
		if fetchErr != nil {
			continue
		}

		for _, p := range manifest.Plugins {
			if p.ID != id {
				continue
			}

			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			annotated := AnnotateVersionCompatibility(compatible)
			hasCompatible := false
			for _, v := range annotated {
				if v.Compatible {
					hasCompatible = true
					break
				}
			}

			return errors.WithStack(c.JSON(http.StatusOK, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Overview:    p.Overview,
				Description: p.Description,
				Homepage:    p.Homepage,
				ImageURL:    p.ImageURL,
				IsOfficial:  repo.IsOfficial,
				Versions:    annotated,
				Compatible:  hasCompatible,
			}))
		}
	}

	return errcodes.NotFound("Plugin")
}
