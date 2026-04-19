package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/version"
)

// AllowedFetchHosts lists the allowed host prefixes for repository manifest URLs.
// Tests can override this to allow test servers.
var AllowedFetchHosts = []string{"https://raw.githubusercontent.com/"}

// validateReleaseDate returns nil for an empty string, RFC3339, or date-only
// (YYYY-MM-DD) values. Any other non-empty value returns an error.
func validateReleaseDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return nil
	}
	return errors.Errorf("invalid releaseDate %q: expected RFC3339 or YYYY-MM-DD", s)
}

// filterInvalidReleaseDates drops versions whose ReleaseDate fails validation,
// logging a warning for each dropped entry. It mutates manifest in place.
func filterInvalidReleaseDates(manifest *RepositoryManifest) {
	log := logger.New()
	for i := range manifest.Plugins {
		p := &manifest.Plugins[i]
		filtered := p.Versions[:0]
		for _, v := range p.Versions {
			if err := validateReleaseDate(v.ReleaseDate); err != nil {
				log.Warn("skipping plugin version with invalid releaseDate", logger.Data{
					"scope":        manifest.Scope,
					"plugin":       p.ID,
					"version":      v.Version,
					"release_date": v.ReleaseDate,
					"error":        err.Error(),
				})
				continue
			}
			filtered = append(filtered, v)
		}
		p.Versions = filtered
	}
}

// RepositoryManifest is the parsed JSON from a repository URL.
type RepositoryManifest struct {
	RepositoryVersion int               `json:"repositoryVersion"`
	Scope             string            `json:"scope"`
	Name              string            `json:"name"`
	Plugins           []AvailablePlugin `json:"plugins"`
}

// AvailablePlugin describes a plugin available for installation from a repository.
type AvailablePlugin struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Overview    string          `json:"overview"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Homepage    string          `json:"homepage"`
	ImageURL    string          `json:"imageUrl"`
	Versions    []PluginVersion `json:"versions"`
}

// PluginVersion describes a specific version of an available plugin.
type PluginVersion struct {
	Version          string        `json:"version"`
	MinShishoVersion string        `json:"minShishoVersion"`
	ManifestVersion  int           `json:"manifestVersion"`
	ReleaseDate      string        `json:"releaseDate"`
	Changelog        string        `json:"changelog"`
	DownloadURL      string        `json:"downloadUrl"`
	SHA256           string        `json:"sha256"`
	Capabilities     *Capabilities `json:"capabilities,omitempty"`
}

// FetchRepository downloads and parses a repository manifest from the given URL.
// Only HTTPS URLs to allowed hosts (raw.githubusercontent.com by default) are permitted.
func FetchRepository(rawURL string) (*RepositoryManifest, error) {
	if !isAllowedFetchURL(rawURL) {
		return nil, errors.Errorf("invalid repository URL: only URLs starting with %v are allowed", AllowedFetchHosts)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request for repository manifest")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch repository manifest")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to fetch repository manifest: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read repository manifest response")
	}

	var manifest RepositoryManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, errors.Wrap(err, "failed to parse repository manifest JSON")
	}

	if manifest.RepositoryVersion != 1 {
		return nil, errors.Errorf("unsupported repositoryVersion %d (only version 1 is supported)", manifest.RepositoryVersion)
	}

	filterInvalidReleaseDates(&manifest)

	return &manifest, nil
}

// isAllowedFetchURL checks whether the URL matches any allowed host prefix.
func isAllowedFetchURL(url string) bool {
	for _, prefix := range AllowedFetchHosts {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}

// FilterCompatibleVersions returns only versions with a manifestVersion
// supported by this Shisho release.
func FilterCompatibleVersions(versions []PluginVersion) []PluginVersion {
	var compatible []PluginVersion
	for _, v := range versions {
		for _, supported := range SupportedManifestVersions {
			if v.ManifestVersion == supported {
				compatible = append(compatible, v)
				break
			}
		}
	}
	return compatible
}

// AnnotatedPluginVersion extends PluginVersion with a compatibility flag.
type AnnotatedPluginVersion struct {
	PluginVersion
	Compatible bool `json:"compatible"`
}

// AnnotateVersionCompatibility annotates each version with whether it is
// compatible with the running Shisho version based on minShishoVersion.
func AnnotateVersionCompatibility(versions []PluginVersion) []AnnotatedPluginVersion {
	result := make([]AnnotatedPluginVersion, len(versions))
	for i, v := range versions {
		result[i] = AnnotatedPluginVersion{
			PluginVersion: v,
			Compatible:    version.IsCompatible(v.MinShishoVersion),
		}
	}
	return result
}

// FilterVersionCompatibleVersions returns only versions whose minShishoVersion
// is satisfied by the running Shisho version.
func FilterVersionCompatibleVersions(versions []PluginVersion) []PluginVersion {
	var compatible []PluginVersion
	for _, v := range versions {
		if version.IsCompatible(v.MinShishoVersion) {
			compatible = append(compatible, v)
		}
	}
	return compatible
}
