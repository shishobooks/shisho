package serverstats

// Volume path kinds: what Shisho keeps at a path on a volume.
const (
	//tygo:emit export type VolumePathKind = typeof VolumePathKindLibrary | typeof VolumePathKindData | typeof VolumePathKindCache;
	VolumePathKindLibrary = "library"
	VolumePathKindData    = "data"
	VolumePathKindCache   = "cache"
)

// StatsResponse is GET /server-stats: what the server holds, the disk it takes
// and has left, and whether people use it. Nothing in it is broken down by
// user or by Book.
type StatsResponse struct {
	// Libraries holds the contents of each Library the viewer can access,
	// ordered by name.
	Libraries []LibraryStats `json:"libraries"`
	// Totals sums every Library, including any the viewer can't access.
	Totals  ContentCounts `json:"totals"`
	Volumes []Volume      `json:"volumes"`
	// Caches is each cache's size. Its total is the cache path's size in
	// Volumes.
	Caches  []CacheSize `json:"caches"`
	Users   int         `json:"users"`
	Devices int         `json:"devices"`
	// Growth is Books by the month they were added, from the first such
	// month to the current one, with empty months included.
	Growth []GrowthMonth `json:"growth"`
	// ActiveReaders is the number of users with a Reading Session in the
	// last 30 days.
	ActiveReaders int `json:"active_readers"`
	// CurrentYear is the year in the server's time zone, which GET
	// /server-stats/reading defaults to on the page.
	CurrentYear int `json:"current_year"`
}

// ContentCounts is what one Library, or every Library, holds.
type ContentCounts struct {
	Books int `json:"books"`
	// Files counts main Files by File Type, ordered by File Type.
	Files           []FileTypeCount `json:"files"`
	SupplementFiles int             `json:"supplement_files"`
	Series          int             `json:"series"`
	Persons         int             `json:"persons"`
	Genres          int             `json:"genres"`
	Tags            int             `json:"tags"`
	Publishers      int             `json:"publishers"`
	// SizeBytes is the stored size of every File, supplements included.
	SizeBytes int64 `json:"size_bytes"`
}

// LibraryStats is one Library's contents.
type LibraryStats struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	ContentCounts `tstype:",extends"`
}

// FileTypeCount is the number of main Files of one File Type.
type FileTypeCount struct {
	FileType string `json:"file_type" tstype:"FileType"`
	Count    int    `json:"count"`
}

// Volume is one mounted filesystem holding at least one of Shisho's paths.
// Paths leaves out library folders the viewer couldn't see on the library
// pages, so it can be empty.
type Volume struct {
	Paths      []VolumePath `json:"paths"`
	TotalBytes uint64       `json:"total_bytes"`
	UsedBytes  uint64       `json:"used_bytes"`
	FreeBytes  uint64       `json:"free_bytes"`
	// LibraryBytes, DatabaseBytes, and CacheBytes are how much of UsedBytes
	// is Shisho's: library files (including folders hidden from the viewer),
	// the database with its write-ahead log, and the caches. The rest is
	// other data on the volume.
	LibraryBytes  int64 `json:"library_bytes"`
	DatabaseBytes int64 `json:"database_bytes"`
	CacheBytes    int64 `json:"cache_bytes"`
}

// VolumePath is one of Shisho's paths on a Volume. Name is the Library's
// name for a library path, and empty for the data and cache directories.
type VolumePath struct {
	Kind string `json:"kind" tstype:"VolumePathKind"`
	Name string `json:"name"`
	Path string `json:"path"`
	// SizeBytes is what Shisho keeps there: the stored size of the library
	// files under a library path, the database in the data directory, or
	// every cache in the cache directory.
	SizeBytes int64 `json:"size_bytes"`
}

// CacheSize is one cache's size, as Administration > Cache shows it.
type CacheSize struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	FileCount int    `json:"file_count"`
}

// GrowthMonth is the number of Books added in one month (YYYY-MM), in the
// server's time zone.
type GrowthMonth struct {
	Month string `json:"month"`
	Books int    `json:"books"`
}

// ServerReadingQuery picks the range GET /server-stats/reading covers: a year in
// the server's time zone, or all time when Year is omitted.
type ServerReadingQuery struct {
	Year *int `query:"year" json:"year,omitempty" validate:"omitempty,min=1,max=9999"`
}

// ServerReadingResponse is GET /server-stats/reading: reading across every user
// over one range, never broken down by user or by Book. Reading and
// listening come from the kind each Reading Session recorded.
type ServerReadingResponse struct {
	// Year is the year covered, or null for all time.
	Year *int `json:"year" tstype:"number | null,required"`
	// Years runs from the current year back to the first year with any
	// reading, newest first.
	Years            []int `json:"years"`
	ReadingSeconds   int64 `json:"reading_seconds"`
	ListeningSeconds int64 `json:"listening_seconds"`
	// ReadThroughsFinished and DidNotFinish count Read-throughs that ended
	// in the range, rereads included; all time also counts those with no
	// end day.
	ReadThroughsFinished int `json:"read_throughs_finished"`
	DidNotFinish         int `json:"did_not_finish"`
	// Readers is the number of users who read in the range: a Reading
	// Session started in it, or a Read-through ended in it.
	Readers int `json:"readers"`
}
