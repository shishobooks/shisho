package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// AllowedFetchHosts lists the allowed host prefixes for repository manifest URLs.
// Tests can override this to allow test servers.
var AllowedFetchHosts = []string{"https://raw.githubusercontent.com/"}

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
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Homepage    string          `json:"homepage"`
	Versions    []PluginVersion `json:"versions"`
}

// PluginVersion describes a specific version of an available plugin.
type PluginVersion struct {
	Version          string `json:"version"`
	MinShishoVersion string `json:"minShishoVersion"`
	ManifestVersion  int    `json:"manifestVersion"`
	ReleaseDate      string `json:"releaseDate"`
	Changelog        string `json:"changelog"`
	DownloadURL      string `json:"downloadUrl"`
	SHA256           string `json:"sha256"`
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
