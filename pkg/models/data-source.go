package models

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceEPUBMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual       = "manual"
	DataSourceEPUBMetadata = "epub_metadata"
	DataSourceM4BMetadata  = "m4b_metadata"
	DataSourceFilepath     = "filepath"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority = iota
	DataSourceEPUBMetadataPriority
	DataSourceM4BMetadataPriority
	DataSourceFilepathPriority
)

var DataSourcePriority = map[string]int{
	DataSourceManual:       DataSourceManualPriority,
	DataSourceEPUBMetadata: DataSourceEPUBMetadataPriority,
	DataSourceM4BMetadata:  DataSourceM4BMetadataPriority,
	DataSourceFilepath:     DataSourceFilepathPriority,
}
