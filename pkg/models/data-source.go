package models

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual        = "manual"
	DataSourceExistingCover = "existing_cover"
	DataSourceEPUBMetadata  = "epub_metadata"
	DataSourceCBZMetadata   = "cbz_metadata"
	DataSourceM4BMetadata   = "m4b_metadata"
	DataSourceFilepath      = "filepath"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority = iota
	DataSourceExistingCoverPriority
	DataSourceEPUBMetadataPriority
	DataSourceCBZMetadataPriority
	DataSourceM4BMetadataPriority
	DataSourceFilepathPriority
)

var DataSourcePriority = map[string]int{
	DataSourceManual:        DataSourceManualPriority,
	DataSourceExistingCover: DataSourceExistingCoverPriority,
	DataSourceEPUBMetadata:  DataSourceEPUBMetadataPriority,
	DataSourceCBZMetadata:   DataSourceCBZMetadataPriority,
	DataSourceM4BMetadata:   DataSourceM4BMetadataPriority,
	DataSourceFilepath:      DataSourceFilepathPriority,
}
