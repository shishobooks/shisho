package models

import "strings"

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceSidecar | typeof DataSourcePlugin | typeof DataSourceFileMetadata | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath | `plugin:${string}`;
	DataSourceManual        = "manual"
	DataSourceSidecar       = "sidecar"
	DataSourcePlugin        = "plugin"
	DataSourceFileMetadata  = "file_metadata"
	DataSourceExistingCover = "existing_cover"
	DataSourceEPUBMetadata  = "epub_metadata"
	DataSourceCBZMetadata   = "cbz_metadata"
	DataSourceM4BMetadata   = "m4b_metadata"
	DataSourceFilepath      = "filepath"

	// DataSourcePluginPrefix is the prefix for plugin-specific data sources.
	// Actual values are "plugin:scope/id" (e.g., "plugin:shisho/goodreads-metadata").
	DataSourcePluginPrefix = "plugin:"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority       = 0
	DataSourceSidecarPriority      = 1 // Sidecar has higher priority than file metadata
	DataSourcePluginPriority       = 2 // Plugin enricher/parser results
	DataSourceFileMetadataPriority = 3 // All file-derived sources share this
	DataSourceFilepathPriority     = 4
)

var dataSourcePriority = map[string]int{
	DataSourceManual:        DataSourceManualPriority,
	DataSourceSidecar:       DataSourceSidecarPriority,
	DataSourcePlugin:        DataSourcePluginPriority,
	DataSourceFileMetadata:  DataSourceFileMetadataPriority,
	DataSourceExistingCover: DataSourceFileMetadataPriority,
	DataSourceEPUBMetadata:  DataSourceFileMetadataPriority,
	DataSourceCBZMetadata:   DataSourceFileMetadataPriority,
	DataSourceM4BMetadata:   DataSourceFileMetadataPriority,
	DataSourceFilepath:      DataSourceFilepathPriority,
}

// PluginDataSource returns a data source string for a specific plugin (e.g., "plugin:shisho/goodreads").
func PluginDataSource(scope, id string) string {
	return DataSourcePluginPrefix + scope + "/" + id
}

// GetDataSourcePriority returns the priority for a given data source string.
// Handles "plugin:scope/id" format by matching the prefix.
func GetDataSourcePriority(source string) int {
	if p, ok := dataSourcePriority[source]; ok {
		return p
	}
	if strings.HasPrefix(source, DataSourcePluginPrefix) {
		return DataSourcePluginPriority
	}
	return DataSourceFilepathPriority
}
