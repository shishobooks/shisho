package models

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceSidecar | typeof DataSourceFileMetadata | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual        = "manual"
	DataSourceSidecar       = "sidecar"
	DataSourceFileMetadata  = "file_metadata"
	DataSourceExistingCover = "existing_cover"
	DataSourceEPUBMetadata  = "epub_metadata"
	DataSourceCBZMetadata   = "cbz_metadata"
	DataSourceM4BMetadata   = "m4b_metadata"
	DataSourceFilepath      = "filepath"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority       = 0
	DataSourceSidecarPriority      = 1 // Sidecar has higher priority than file metadata
	DataSourceFileMetadataPriority = 2 // All file-derived sources share this
	DataSourceFilepathPriority     = 3
)

var DataSourcePriority = map[string]int{
	DataSourceManual:        DataSourceManualPriority,
	DataSourceSidecar:       DataSourceSidecarPriority,
	DataSourceFileMetadata:  DataSourceFileMetadataPriority,
	DataSourceExistingCover: DataSourceFileMetadataPriority,
	DataSourceEPUBMetadata:  DataSourceFileMetadataPriority,
	DataSourceCBZMetadata:   DataSourceFileMetadataPriority,
	DataSourceM4BMetadata:   DataSourceFileMetadataPriority,
	DataSourceFilepath:      DataSourceFilepathPriority,
}
