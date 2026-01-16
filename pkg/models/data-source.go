package models

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceFileMetadata | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual        = "manual"
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
	DataSourceFileMetadataPriority = 1 // All file-derived sources share this
	DataSourceFilepathPriority     = 2
)

var DataSourcePriority = map[string]int{
	DataSourceManual:        DataSourceManualPriority,
	DataSourceFileMetadata:  DataSourceFileMetadataPriority,
	DataSourceExistingCover: DataSourceFileMetadataPriority,
	DataSourceEPUBMetadata:  DataSourceFileMetadataPriority,
	DataSourceCBZMetadata:   DataSourceFileMetadataPriority,
	DataSourceM4BMetadata:   DataSourceFileMetadataPriority,
	DataSourceFilepath:      DataSourceFilepathPriority,
}
