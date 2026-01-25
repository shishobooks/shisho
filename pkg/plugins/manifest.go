package plugins

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// SupportedManifestVersions lists manifest versions this Shisho release supports.
var SupportedManifestVersions = []int{1}

type Manifest struct {
	ManifestVersion  int          `json:"manifestVersion"`
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	Description      string       `json:"description"`
	Author           string       `json:"author"`
	Homepage         string       `json:"homepage"`
	License          string       `json:"license"`
	MinShishoVersion string       `json:"minShishoVersion"`
	Capabilities     Capabilities `json:"capabilities"`
	ConfigSchema     ConfigSchema `json:"configSchema"`
}

type Capabilities struct {
	InputConverter   *InputConverterCap   `json:"inputConverter"`
	FileParser       *FileParserCap       `json:"fileParser"`
	OutputGenerator  *OutputGeneratorCap  `json:"outputGenerator"`
	MetadataEnricher *MetadataEnricherCap `json:"metadataEnricher"`
	IdentifierTypes  []IdentifierTypeCap  `json:"identifierTypes"`
	HTTPAccess       *HTTPAccessCap       `json:"httpAccess"`
	FileAccess       *FileAccessCap       `json:"fileAccess"`
	FFmpegAccess     *FFmpegAccessCap     `json:"ffmpegAccess"`
}

type InputConverterCap struct {
	Description string   `json:"description"`
	SourceTypes []string `json:"sourceTypes"`
	MIMETypes   []string `json:"mimeTypes"`
	TargetType  string   `json:"targetType"`
}

type FileParserCap struct {
	Description string   `json:"description"`
	Types       []string `json:"types"`
	MIMETypes   []string `json:"mimeTypes"`
}

type OutputGeneratorCap struct {
	Description string   `json:"description"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SourceTypes []string `json:"sourceTypes"`
}

type MetadataEnricherCap struct {
	Description string   `json:"description"`
	FileTypes   []string `json:"fileTypes"`
}

type IdentifierTypeCap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URLTemplate string `json:"urlTemplate"`
	Pattern     string `json:"pattern"`
}

type HTTPAccessCap struct {
	Description string   `json:"description"`
	Domains     []string `json:"domains"`
}

type FileAccessCap struct {
	Level       string `json:"level"` // "read" or "readwrite"
	Description string `json:"description"`
}

type FFmpegAccessCap struct {
	Description string `json:"description"`
}

type ConfigSchema map[string]ConfigField

type ConfigField struct {
	Type        string         `json:"type"` // string, boolean, number, select, textarea
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Required    bool           `json:"required"`
	Secret      bool           `json:"secret"`
	Default     interface{}    `json:"default"`
	Min         *float64       `json:"min"`
	Max         *float64       `json:"max"`
	Options     []SelectOption `json:"options"`
}

type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ParseManifest parses and validates a manifest.json byte slice.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, "failed to parse manifest JSON")
	}

	if m.ManifestVersion == 0 {
		return nil, errors.New("manifest: manifestVersion is required")
	}

	supported := false
	for _, v := range SupportedManifestVersions {
		if m.ManifestVersion == v {
			supported = true
			break
		}
	}
	if !supported {
		return nil, errors.Errorf("manifest: unsupported manifestVersion %d (supported: %v)", m.ManifestVersion, SupportedManifestVersions)
	}

	if m.ID == "" {
		return nil, errors.New("manifest: id is required")
	}

	if m.Name == "" {
		return nil, errors.New("manifest: name is required")
	}

	if m.Version == "" {
		return nil, errors.New("manifest: version is required")
	}

	return &m, nil
}
