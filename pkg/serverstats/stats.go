package serverstats

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// activeWindow is how recent a Reading Session makes its user active.
const activeWindow = 30 * 24 * time.Hour

// endedOutcomes are the Read-through outcomes that end one.
var endedOutcomes = []string{models.ReadThroughOutcomeFinished, models.ReadThroughOutcomeAbandoned}

// earliestAdded is before any Book could have been added to Shisho or read
// in it; an earlier date isn't a real one.
var earliestAdded = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// compute builds the whole Server Stats payload. Everything is counted on
// request; nothing is stored between requests.
//
// Library rows and folders follow the viewer's access, as every other page
// does: a row only for a Library the viewer can access, and its folders only
// with Libraries Read as well. The totals and the volumes' space still cover
// the whole server.
func (h *handler) compute(ctx context.Context, user *models.User) (StatsResponse, error) {
	now := h.now().In(h.location)
	resp := StatsResponse{}

	libraries, err := h.libraries(ctx)
	if err != nil {
		return resp, err
	}
	resp.Totals = sumCounts(libraries)
	resp.Libraries = make([]LibraryStats, 0, len(libraries))
	for _, lib := range libraries {
		if user.HasLibraryAccess(lib.ID) {
			resp.Libraries = append(resp.Libraries, lib)
		}
	}

	databaseBytes, err := h.databaseBytes()
	if err != nil {
		return resp, err
	}
	resp.Caches = h.cacheSizes(ctx)
	var cacheBytes int64
	for _, c := range resp.Caches {
		cacheBytes += c.SizeBytes
	}
	if resp.Volumes, err = h.volumes(ctx, user, libraries, databaseBytes, cacheBytes); err != nil {
		return resp, err
	}
	if err := h.db.NewSelect().Table("users").ColumnExpr("COUNT(*)").Scan(ctx, &resp.Users); err != nil {
		return resp, errors.WithStack(err)
	}
	if err := h.db.NewSelect().Table("devices").ColumnExpr("COUNT(*)").Scan(ctx, &resp.Devices); err != nil {
		return resp, errors.WithStack(err)
	}
	if resp.Growth, err = h.growth(ctx, now); err != nil {
		return resp, err
	}
	resp.CurrentYear = now.Year()
	err = h.db.NewSelect().
		Table("reading_sessions").
		ColumnExpr("COUNT(DISTINCT user_id)").
		Where("ended_at >= ?", now.Add(-activeWindow)).
		Scan(ctx, &resp.ActiveReaders)
	if err != nil {
		return resp, errors.Wrap(err, "count active readers")
	}
	return resp, nil
}

// libraries counts what each Library holds, ordered by name.
func (h *handler) libraries(ctx context.Context) ([]LibraryStats, error) {
	var libs []*models.Library
	if err := h.db.NewSelect().Model(&libs).Column("id", "name").OrderExpr("l.name ASC, l.id ASC").Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	stats := make([]LibraryStats, len(libs))
	byID := make(map[int]*ContentCounts, len(libs))
	for i, lib := range libs {
		stats[i] = LibraryStats{ID: lib.ID, Name: lib.Name, ContentCounts: ContentCounts{Files: []FileTypeCount{}}}
		byID[lib.ID] = &stats[i].ContentCounts
	}

	// Each table's rows per Library, written into the field it counts.
	for _, count := range []struct {
		table string
		field func(*ContentCounts) *int
	}{
		{"books", func(c *ContentCounts) *int { return &c.Books }},
		{"series", func(c *ContentCounts) *int { return &c.Series }},
		{"persons", func(c *ContentCounts) *int { return &c.Persons }},
		{"genres", func(c *ContentCounts) *int { return &c.Genres }},
		{"tags", func(c *ContentCounts) *int { return &c.Tags }},
		{"publishers", func(c *ContentCounts) *int { return &c.Publishers }},
	} {
		var rows []struct {
			LibraryID int `bun:"library_id"`
			Count     int `bun:"count"`
		}
		err := h.db.NewSelect().
			Table(count.table).
			ColumnExpr("library_id, COUNT(*) AS count").
			Group("library_id").
			Scan(ctx, &rows)
		if err != nil {
			return nil, errors.Wrapf(err, "count %s", count.table)
		}
		for _, row := range rows {
			if c := byID[row.LibraryID]; c != nil {
				*count.field(c) = row.Count
			}
		}
	}

	var files []struct {
		LibraryID int    `bun:"library_id"`
		FileType  string `bun:"file_type"`
		FileRole  string `bun:"file_role"`
		Count     int    `bun:"count"`
		Bytes     int64  `bun:"bytes"`
	}
	err := h.db.NewSelect().
		Table("files").
		ColumnExpr("library_id, file_type, file_role, COUNT(*) AS count, COALESCE(SUM(filesize_bytes), 0) AS bytes").
		Group("library_id", "file_type", "file_role").
		Order("library_id", "file_type").
		Scan(ctx, &files)
	if err != nil {
		return nil, errors.Wrap(err, "count files")
	}
	for _, row := range files {
		c := byID[row.LibraryID]
		if c == nil {
			continue
		}
		c.SizeBytes += row.Bytes
		if row.FileRole == models.FileRoleSupplement {
			c.SupplementFiles += row.Count
			continue
		}
		c.Files = append(c.Files, FileTypeCount{FileType: row.FileType, Count: row.Count})
	}
	return stats, nil
}

func sumCounts(libraries []LibraryStats) ContentCounts {
	total := ContentCounts{Files: []FileTypeCount{}}
	files := map[string]int{}
	for _, lib := range libraries {
		total.Books += lib.Books
		total.SupplementFiles += lib.SupplementFiles
		total.Series += lib.Series
		total.Persons += lib.Persons
		total.Genres += lib.Genres
		total.Tags += lib.Tags
		total.Publishers += lib.Publishers
		total.SizeBytes += lib.SizeBytes
		for _, f := range lib.Files {
			files[f.FileType] += f.Count
		}
	}
	for fileType, count := range files {
		total.Files = append(total.Files, FileTypeCount{FileType: fileType, Count: count})
	}
	sort.Slice(total.Files, func(i, j int) bool { return total.Files[i].FileType < total.Files[j].FileType })
	return total
}

// volumes reports the space on each filesystem holding a library path, the
// data directory, or the cache directory, each filesystem once. A path that
// can't be read (an unmounted drive) is left out rather than failing the
// page.
func (h *handler) volumes(ctx context.Context, user *models.User, libraries []LibraryStats, databaseBytes, cacheBytes int64) ([]Volume, error) {
	names := make(map[int]string, len(libraries))
	for _, lib := range libraries {
		names[lib.ID] = lib.Name
	}
	var libraryPaths []*models.LibraryPath
	if err := h.db.NewSelect().Model(&libraryPaths).Column("library_id", "filepath").Order("filepath").Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	sort.SliceStable(libraryPaths, func(i, j int) bool {
		return names[libraryPaths[i].LibraryID] < names[libraryPaths[j].LibraryID]
	})

	// Every path is statted, so each volume's space is reported, but a
	// library folder is listed only to a viewer who could see it elsewhere.
	type statPath struct {
		VolumePath
		shown     bool
		mustExist bool
		// nested is a library folder inside another folder of the same
		// library, whose files that folder's size already counts.
		nested bool
	}
	canReadLibraries := user.HasPermission(models.ResourceLibraries, models.OperationRead)
	paths := make([]statPath, 0, len(libraryPaths)+2)
	for _, lp := range libraryPaths {
		size, err := h.libraryPathBytes(ctx, lp)
		if err != nil {
			return nil, err
		}
		paths = append(paths, statPath{
			VolumePath: VolumePath{Kind: VolumePathKindLibrary, Name: names[lp.LibraryID], Path: lp.Filepath, SizeBytes: size},
			shown:      canReadLibraries && user.HasLibraryAccess(lp.LibraryID),
			mustExist:  true,
			nested:     insideAnother(lp, libraryPaths),
		})
	}
	if h.databasePath != "" && h.databasePath != ":memory:" {
		paths = append(paths, statPath{
			VolumePath: VolumePath{Kind: VolumePathKindData, Path: absolute(filepath.Dir(h.databasePath)), SizeBytes: databaseBytes},
			shown:      true,
			mustExist:  true,
		})
	}
	if h.cacheDir != "" {
		paths = append(paths, statPath{
			VolumePath: VolumePath{Kind: VolumePathKindCache, Path: absolute(h.cacheDir), SizeBytes: cacheBytes},
			shown:      true,
		})
	}

	volumes := []Volume{}
	index := map[uint64]int{}
	for _, p := range paths {
		stat, err := statVolumeWithin(ctx, h.statTimeout, h.statVolume, p.Path, p.mustExist)
		if err != nil {
			logger.FromContext(ctx).Warn("could not read volume space", logger.Data{"path": p.Path, "error": err.Error()})
			continue
		}
		i, ok := index[stat.id]
		if !ok {
			i = len(volumes)
			index[stat.id] = i
			volumes = append(volumes, Volume{Paths: []VolumePath{}, TotalBytes: stat.total, UsedBytes: stat.used, FreeBytes: stat.free})
		}
		switch p.Kind {
		case VolumePathKindLibrary:
			if !p.nested {
				volumes[i].LibraryBytes += p.SizeBytes
			}
		case VolumePathKindData:
			volumes[i].DatabaseBytes += p.SizeBytes
		case VolumePathKindCache:
			volumes[i].CacheBytes += p.SizeBytes
		}
		if p.shown {
			volumes[i].Paths = append(volumes[i].Paths, p.VolumePath)
		}
	}
	return volumes, nil
}

// insideAnother reports whether lp sits inside another folder of the same
// library.
func insideAnother(lp *models.LibraryPath, all []*models.LibraryPath) bool {
	for _, other := range all {
		if other != lp && other.LibraryID == lp.LibraryID &&
			strings.HasPrefix(lp.Filepath, strings.TrimRight(other.Filepath, "/")+"/") {
			return true
		}
	}
	return false
}

// absolute resolves a configured path against the server's working
// directory: a relative one means nothing to someone reading the page.
func absolute(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// libraryPathBytes is the stored size of the library's files under lp,
// supplements included. A prefix compare rather than LIKE, so a "%" or "_"
// in a folder name matches only itself.
func (h *handler) libraryPathBytes(ctx context.Context, lp *models.LibraryPath) (int64, error) {
	dir := strings.TrimRight(lp.Filepath, "/") + "/"
	var size int64
	err := h.db.NewSelect().
		Table("files").
		ColumnExpr("COALESCE(SUM(filesize_bytes), 0)").
		Where("library_id = ?", lp.LibraryID).
		// substr counts characters, not bytes.
		Where("substr(filepath, 1, ?) = ?", utf8.RuneCountInString(dir), dir).
		Scan(ctx, &size)
	if err != nil {
		return 0, errors.Wrapf(err, "size of %s", lp.Filepath)
	}
	return size, nil
}

// databaseBytes is the database file plus its write-ahead log.
func (h *handler) databaseBytes() (int64, error) {
	if h.databasePath == "" || h.databasePath == ":memory:" {
		return 0, nil
	}
	var total int64
	for _, path := range []string{h.databasePath, h.databasePath + "-wal"} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, errors.WithStack(err)
		}
		total += info.Size()
	}
	return total, nil
}

// cacheSizes reports what Administration > Cache shows. A cache that can't
// be measured leaves the caches out rather than failing the page; the Cache
// page reports the error.
func (h *handler) cacheSizes(ctx context.Context) []CacheSize {
	infos, err := h.caches.Sizes()
	if err != nil {
		logger.FromContext(ctx).Warn("could not measure the caches", logger.Data{"error": err.Error()})
		return []CacheSize{}
	}
	sizes := make([]CacheSize, len(infos))
	for i, info := range infos {
		sizes[i] = CacheSize{ID: info.ID, Name: info.Name, SizeBytes: info.SizeBytes, FileCount: info.FileCount}
	}
	return sizes
}

// growth is Books by the month they were added in the server's time zone,
// from the first month with one to the current month. SQLite can't convert
// to an IANA zone, so the months are worked out here. A date after this
// month (a clock that was ahead) counts in this month, and one before
// earliestAdded (a zero date written by hand) in the first real month, so
// the months stay bounded and add up to the number of Books.
func (h *handler) growth(ctx context.Context, now time.Time) ([]GrowthMonth, error) {
	var added []time.Time
	if err := h.db.NewSelect().Table("books").Column("created_at").Scan(ctx, &added); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(added) == 0 {
		return []GrowthMonth{}, nil
	}
	end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, h.location)
	counts := map[string]int{}
	first := end
	undated := 0
	for _, t := range added {
		if t.Before(earliestAdded) {
			undated++
			continue
		}
		t = t.In(h.location)
		if t.After(now) {
			t = now
		}
		counts[t.Format("2006-01")]++
		if t.Before(first) {
			first = t
		}
	}
	counts[first.Format("2006-01")] += undated
	months := []GrowthMonth{}
	for m := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, h.location); !m.After(end); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01")
		months = append(months, GrowthMonth{Month: key, Books: counts[key]})
	}
	return months, nil
}

// reading totals reading across the server for one year in the server's
// time zone, or all time when year is nil, never by user or by Book.
// Read-through end days are calendar days, so they're compared as stored.
func (h *handler) reading(ctx context.Context, year *int) (ServerReadingResponse, error) {
	resp := ServerReadingResponse{Year: year}

	// The range as session instants and as Read-through end days.
	sessions := func(q *bun.SelectQuery) *bun.SelectQuery { return q }
	ended := func(q *bun.SelectQuery) *bun.SelectQuery { return q }
	if year != nil {
		start := time.Date(*year, time.January, 1, 0, 0, 0, 0, h.location)
		end := start.AddDate(1, 0, 0)
		sessions = func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("started_at >= ? AND started_at < ?", start, end)
		}
		ended = func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("end_day >= ? AND end_day < ?", start.Format("2006-01-02"), end.Format("2006-01-02"))
		}
	}

	var seconds []struct {
		Kind    *string `bun:"kind"`
		Seconds int64   `bun:"seconds"`
	}
	err := h.db.NewSelect().
		Table("reading_sessions").
		ColumnExpr("kind, COALESCE(SUM(duration), 0) AS seconds").
		Apply(sessions).
		Group("kind").
		Scan(ctx, &seconds)
	if err != nil {
		return resp, errors.Wrap(err, "sum reading sessions")
	}
	for _, row := range seconds {
		// A Session with no recorded kind counts as reading.
		if row.Kind != nil && *row.Kind == models.ReadingKindListening {
			resp.ListeningSeconds += row.Seconds
		} else {
			resp.ReadingSeconds += row.Seconds
		}
	}

	var outcomes []struct {
		Outcome string `bun:"outcome"`
		Count   int    `bun:"count"`
	}
	err = h.db.NewSelect().
		Table("read_throughs").
		ColumnExpr("outcome, COUNT(*) AS count").
		Where("outcome IN (?)", bun.List(endedOutcomes)).
		Apply(ended).
		Group("outcome").
		Scan(ctx, &outcomes)
	if err != nil {
		return resp, errors.Wrap(err, "count ended read-throughs")
	}
	for _, row := range outcomes {
		if row.Outcome == models.ReadThroughOutcomeFinished {
			resp.ReadThroughsFinished = row.Count
		} else {
			resp.DidNotFinish = row.Count
		}
	}

	// Readers: anyone with a Session started in the range or a Read-through
	// that ended in it.
	readerSessions := h.db.NewSelect().Table("reading_sessions").Column("user_id").Apply(sessions)
	readerEnds := h.db.NewSelect().Table("read_throughs").Column("user_id").
		Where("outcome IN (?)", bun.List(endedOutcomes)).
		Apply(ended)
	err = h.db.NewSelect().
		TableExpr("(?) AS readers", readerSessions.Union(readerEnds)).
		ColumnExpr("COUNT(*)").
		Scan(ctx, &resp.Readers)
	if err != nil {
		return resp, errors.Wrap(err, "count readers")
	}

	resp.Years, err = h.readingYears(ctx)
	return resp, err
}

// readingYears runs from the current year back to the first year with a
// Reading Session or an ended Read-through, newest first.
func (h *handler) readingYears(ctx context.Context) ([]int, error) {
	current := h.now().In(h.location).Year()
	first := current

	var firstSession []time.Time
	err := h.db.NewSelect().Table("reading_sessions").ColumnExpr("MIN(started_at)").
		Having("COUNT(*) > 0").Scan(ctx, &firstSession)
	if err != nil {
		return nil, errors.Wrap(err, "first reading session")
	}
	if len(firstSession) > 0 {
		first = min(first, firstSession[0].In(h.location).Year())
	}
	var firstEnd []string
	err = h.db.NewSelect().Table("read_throughs").ColumnExpr("MIN(end_day)").
		Where("end_day IS NOT NULL").Having("COUNT(*) > 0").Scan(ctx, &firstEnd)
	if err != nil {
		return nil, errors.Wrap(err, "first read-through end")
	}
	if len(firstEnd) > 0 {
		if t, err := time.Parse("2006-01-02", firstEnd[0]); err == nil {
			first = min(first, t.Year())
		}
	}

	// An earlier date is a mistake (a mistyped end day, a zero timestamp)
	// rather than reading; it still counts toward all time.
	first = max(first, earliestAdded.Year())
	years := make([]int, 0, current-first+1)
	for y := current; y >= first; y-- {
		years = append(years, y)
	}
	return years, nil
}
