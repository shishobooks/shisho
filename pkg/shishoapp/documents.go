package shishoapp

import (
	"archive/zip"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

// buildEntries turns log entries into the feed's entries for userID,
// reading each record as it is now in tx, the snapshot the entries came
// from. A record the entry doesn't mark deleted always exists in that
// snapshot, since its delete would have moved its entry.
func buildEntries(ctx context.Context, tx bun.IDB, user *models.User, entries []*changefeed.Entry) ([]FeedEntry, error) {
	userID := user.ID
	ids := map[string][]int{}
	for _, entry := range entries {
		if !entry.Deleted {
			ids[entry.Kind] = append(ids[entry.Kind], entry.RecordID)
		}
	}
	bookIDs, libraryIDs := ids[changefeed.KindBook], ids[changefeed.KindLibrary]
	positionFileIDs, readThroughIDs := ids[changefeed.KindReadingPosition], ids[changefeed.KindReadThrough]
	books, err := loadBooks(ctx, tx, bookIDs)
	if err != nil {
		return nil, err
	}
	series, err := loadSeries(ctx, tx, ids[changefeed.KindSeries])
	if err != nil {
		return nil, err
	}
	persons, err := loadPersons(ctx, tx, ids[changefeed.KindPerson])
	if err != nil {
		return nil, err
	}
	genres, err := loadGenres(ctx, tx, ids[changefeed.KindGenre])
	if err != nil {
		return nil, err
	}
	tags, err := loadTags(ctx, tx, ids[changefeed.KindTag])
	if err != nil {
		return nil, err
	}
	publishers, err := loadPublishers(ctx, tx, ids[changefeed.KindPublisher])
	if err != nil {
		return nil, err
	}
	userBooks, err := loadUserBooks(ctx, tx, userID, ids[changefeed.KindUserBook])
	if err != nil {
		return nil, err
	}
	lists, err := loadLists(ctx, tx, user, ids[changefeed.KindList])
	if err != nil {
		return nil, err
	}
	libraries, err := loadLibraries(ctx, tx, userID, libraryIDs)
	if err != nil {
		return nil, err
	}
	positions, err := loadPositions(ctx, tx, userID, positionFileIDs)
	if err != nil {
		return nil, err
	}
	readThroughs, err := loadReadThroughs(ctx, tx, userID, readThroughIDs)
	if err != nil {
		return nil, err
	}
	bookmarks, err := loadBookmarks(ctx, tx, userID, ids[changefeed.KindBookmark])
	if err != nil {
		return nil, err
	}

	out := make([]FeedEntry, 0, len(entries))
	for _, entry := range entries {
		item := FeedEntry{Kind: entry.Kind, ID: entry.RecordID, Deleted: entry.Deleted}
		if entry.LibraryID != nil {
			item.LibraryID = *entry.LibraryID
		}
		if !entry.Deleted {
			var ok bool
			switch entry.Kind {
			case changefeed.KindSeries:
				item.Series, ok = series[entry.RecordID]
			case changefeed.KindPerson:
				item.Person, ok = persons[entry.RecordID]
			case changefeed.KindGenre:
				item.Genre, ok = genres[entry.RecordID]
			case changefeed.KindTag:
				item.Tag, ok = tags[entry.RecordID]
			case changefeed.KindPublisher:
				item.Publisher, ok = publishers[entry.RecordID]
			case changefeed.KindUserBook:
				item.UserBook, ok = userBooks[entry.RecordID]
			case changefeed.KindList:
				item.List, ok = lists[entry.RecordID]
			case changefeed.KindBook:
				item.Book, ok = books[entry.RecordID]
			case changefeed.KindLibrary:
				item.Library, ok = libraries[entry.RecordID]
			case changefeed.KindReadingPosition:
				item.ReadingPosition, ok = positions[entry.RecordID]
			case changefeed.KindReadThrough:
				item.ReadThrough, ok = readThroughs[entry.RecordID]
			case changefeed.KindBookmark:
				item.Bookmark, ok = bookmarks[entry.RecordID]
			default:
				// A kind this server doesn't build documents for yet.
			}
			if !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func loadLibraries(ctx context.Context, tx bun.IDB, userID int, ids []int) (map[int]*FeedLibrary, error) {
	out := map[int]*FeedLibrary{}
	if len(ids) == 0 {
		return out, nil
	}
	var libraries []*models.Library
	if err := tx.NewSelect().Model(&libraries).Where("l.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	var settings []*models.UserLibrarySettings
	err := tx.NewSelect().Model(&settings).
		Where("uls.user_id = ?", userID).
		Where("uls.library_id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sorts := map[int]string{}
	for _, setting := range settings {
		// A spec the web app wouldn't apply is no sort at all.
		if setting.SortSpec == nil {
			continue
		}
		if _, err := sortspec.Parse(*setting.SortSpec); err == nil {
			sorts[setting.LibraryID] = *setting.SortSpec
		}
	}
	for _, library := range libraries {
		out[library.ID] = &FeedLibrary{
			ID: library.ID, Name: library.Name, CoverAspectRatio: library.CoverAspectRatio,
			SortSpec: sorts[library.ID],
		}
	}
	return out, nil
}

func loadBooks(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedBook, error) {
	out := map[int]*FeedBook{}
	if len(ids) == 0 {
		return out, nil
	}
	var books []*models.Book
	err := tx.NewSelect().Model(&books).
		Relation("Library").
		Relation("Authors", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("a.sort_order ASC")
		}).
		Relation("Authors.Person").
		Relation("BookSeries", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("bs.sort_order ASC")
		}).
		Relation("BookSeries.Series").
		Relation("Files", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("f.id ASC")
		}).
		Relation("Files.Narrators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
		Relation("Files.Chapters", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("ch.sort_order ASC", "ch.id ASC")
		}).
		Relation("Files.Publisher").
		Relation("Files.Identifiers", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("fi.id ASC")
		}).
		Relation("BookGenres.Genre").
		Relation("BookTags.Tag").
		Where("b.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, book := range books {
		// Reading a CBZ's pages opens its archive, so a sync the app gave
		// up on stops here.
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
		out[book.ID] = bookDocument(ctx, book)
	}
	return out, nil
}

// bookDocument is book as the app keeps it. The triggers move a Book's
// entry when anything read here changes.
func bookDocument(ctx context.Context, book *models.Book) *FeedBook {
	doc := &FeedBook{
		ID:          book.ID,
		LibraryID:   book.LibraryID,
		Title:       book.Title,
		Subtitle:    book.Subtitle,
		SortTitle:   book.SortTitle,
		Description: book.Description,
		CreatedAt:   book.CreatedAt,
		Authors:     []FeedAuthor{},
		Series:      []FeedSeriesMembership{},
		Genres:      []FeedGenreRef{},
		Tags:        []FeedTagRef{},
		Files:       []FeedFile{},
	}
	for _, bookGenre := range book.BookGenres {
		if bookGenre.Genre != nil {
			doc.Genres = append(doc.Genres, FeedGenreRef{GenreID: bookGenre.GenreID, Name: bookGenre.Genre.Name})
		}
	}
	slices.SortFunc(doc.Genres, func(a, b FeedGenreRef) int { return compareNames(a.Name, b.Name) })
	for _, bookTag := range book.BookTags {
		if bookTag.Tag != nil {
			doc.Tags = append(doc.Tags, FeedTagRef{TagID: bookTag.TagID, Name: bookTag.Tag.Name})
		}
	}
	slices.SortFunc(doc.Tags, func(a, b FeedTagRef) int { return compareNames(a.Name, b.Name) })
	for _, author := range book.Authors {
		if author.Person == nil {
			continue
		}
		doc.Authors = append(doc.Authors, FeedAuthor{
			PersonID: author.PersonID,
			Name:     author.Person.Name,
			SortName: author.Person.SortName,
			Role:     author.Role,
		})
	}
	for _, membership := range book.BookSeries {
		if membership.Series == nil {
			continue
		}
		doc.Series = append(doc.Series, FeedSeriesMembership{
			SeriesID:  membership.SeriesID,
			Name:      membership.Series.Name,
			SortName:  membership.Series.SortName,
			Number:    membership.SeriesNumber,
			NumberEnd: membership.SeriesNumberEnd,
			Unit:      membership.SeriesNumberUnit,
		})
	}
	for _, file := range book.Files {
		narrators := []FeedNarrator{}
		for _, narrator := range file.Narrators {
			if narrator.Person == nil {
				continue
			}
			narrators = append(narrators, FeedNarrator{
				PersonID: narrator.PersonID, Name: narrator.Person.Name, SortName: narrator.Person.SortName,
			})
		}
		var publisher *FeedPublisherRef
		if file.Publisher != nil {
			publisher = &FeedPublisherRef{PublisherID: file.Publisher.ID, Name: file.Publisher.Name}
		}
		identifiers := []FeedIdentifier{}
		for _, identifier := range file.Identifiers {
			identifiers = append(identifiers, FeedIdentifier{Type: identifier.Type, Value: identifier.Value})
		}
		doc.Files = append(doc.Files, FeedFile{
			Publisher:                publisher,
			Identifiers:              identifiers,
			AudiobookCodec:           file.AudiobookCodec,
			AudiobookBitrateBps:      file.AudiobookBitrateBps,
			Language:                 file.Language,
			Abridged:                 file.Abridged,
			URL:                      file.URL,
			ReadingDirection:         file.ReadingDirection,
			Filepath:                 file.Filepath,
			FileModifiedAt:           utcPointer(file.FileModifiedAt),
			CreatedAt:                file.CreatedAt.UTC(),
			Pages:                    filePages(ctx, file),
			CoverVersion:             fileCoverVersion(file),
			Narrators:                narrators,
			Chapters:                 chapterDocuments(file.Chapters),
			ID:                       file.ID,
			FileType:                 file.FileType,
			FileRole:                 file.FileRole,
			SizeBytes:                file.FilesizeBytes,
			DisplayName:              file.ResolveDisplayName(),
			ReleaseDate:              file.ReleaseDate,
			PageCount:                file.PageCount,
			AudiobookDurationSeconds: file.AudiobookDurationSeconds,
		})
	}
	aspect := ""
	if book.Library != nil {
		aspect = book.Library.CoverAspectRatio
	}
	doc.CoverVersion = covers.CacheKey(book.Files, aspect)
	return doc
}

// chapterDocuments orders chapters as a table of contents: each parent's
// children, by their sort order, right after it.
func chapterDocuments(chapters []*models.Chapter) []FeedChapter {
	children := map[int][]*models.Chapter{}
	var roots []*models.Chapter
	byID := map[int]bool{}
	for _, chapter := range chapters {
		byID[chapter.ID] = true
	}
	for _, chapter := range chapters {
		if chapter.ParentID != nil && byID[*chapter.ParentID] {
			children[*chapter.ParentID] = append(children[*chapter.ParentID], chapter)
		} else {
			roots = append(roots, chapter)
		}
	}
	out := make([]FeedChapter, 0, len(chapters))
	var add func(list []*models.Chapter)
	add = func(list []*models.Chapter) {
		for _, chapter := range list {
			out = append(out, FeedChapter{
				ID: chapter.ID, ParentID: chapter.ParentID, Title: chapter.Title,
				StartTimestampMs: chapter.StartTimestampMs, StartPage: chapter.StartPage, Href: chapter.Href,
			})
			add(children[chapter.ID])
		}
	}
	add(roots)
	return out
}

// loadPositions reads userID's Reading Positions in fileIDs, by File.
func loadPositions(ctx context.Context, tx bun.IDB, userID int, fileIDs []int) (map[int]*FeedReadingPosition, error) {
	out := map[int]*FeedReadingPosition{}
	if len(fileIDs) == 0 {
		return out, nil
	}
	var positions []*models.ReadingPosition
	err := tx.NewSelect().Model(&positions).
		Where("rp.user_id = ?", userID).
		Where("rp.file_id IN (?)", bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	deviceIDs := []string{}
	for _, position := range positions {
		if position.SourceDeviceID != nil {
			deviceIDs = append(deviceIDs, *position.SourceDeviceID)
		}
	}
	names, err := loadDeviceNames(ctx, tx, deviceIDs)
	if err != nil {
		return nil, err
	}
	for _, position := range positions {
		var name *string
		if position.SourceDeviceID != nil {
			if n, ok := names[*position.SourceDeviceID]; ok {
				name = &n
			}
		}
		out[position.FileID] = &FeedReadingPosition{
			FileID: position.FileID, CFI: position.CFI, Page: position.Page, Seconds: position.Seconds,
			Percentage: position.Percentage, OccurredAt: position.OccurredAt.UTC(), Source: position.Source,
			DeviceID: position.SourceDeviceID, DeviceName: name,
		}
	}
	return out, nil
}

// loadDeviceNames reads the names of the Devices in ids, by id.
func loadDeviceNames(ctx context.Context, tx bun.IDB, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   string `bun:"id"`
		Name string `bun:"name"`
	}
	err := tx.NewSelect().Table("devices").Column("id", "name").
		Where("id IN (?)", bun.List(ids)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

// loadReadThroughs reads userID's Read-throughs among ids.
func loadReadThroughs(ctx context.Context, tx bun.IDB, userID int, ids []int) (map[int]*FeedReadThrough, error) {
	out := map[int]*FeedReadThrough{}
	if len(ids) == 0 {
		return out, nil
	}
	var readThroughs []*models.ReadThrough
	err := tx.NewSelect().Model(&readThroughs).
		Where("rt.user_id = ?", userID).
		Where("rt.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, rt := range readThroughs {
		out[rt.ID] = &FeedReadThrough{
			ID: rt.ID, BookID: rt.BookID, BeginDay: rt.BeginDay, EndDay: rt.EndDay, Outcome: rt.Outcome, Paused: rt.Paused,
		}
	}
	// When each was last read, counted as reading.LastReadExpr counts it.
	var lastRead []struct {
		ReadThroughID int       `bun:"read_through_id"`
		At            time.Time `bun:"at"`
	}
	err = tx.NewRaw(`SELECT rpu.read_through_id, MAX(rpu.occurred_at) AS at FROM reading_position_updates AS rpu
		WHERE rpu.user_id = ? AND rpu.read_through_id IN (?) AND NOT rpu.opened
		AND NOT EXISTS (SELECT 1 FROM files AS f WHERE f.id = rpu.file_id AND f.file_role = ?)
		GROUP BY rpu.read_through_id`, userID, bun.List(ids), models.FileRoleSupplement).
		Scan(ctx, &lastRead)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range lastRead {
		if rt := out[row.ReadThroughID]; rt != nil {
			at := row.At
			rt.LastReadAt = &at
		}
	}
	return out, nil
}

// loadBookmarks reads userID's Bookmarks among ids, with the id the app made
// each under, if it made it.
func loadBookmarks(ctx context.Context, tx bun.IDB, userID int, ids []int) (map[int]*FeedBookmark, error) {
	out := map[int]*FeedBookmark{}
	if len(ids) == 0 {
		return out, nil
	}
	var bookmarks []*models.Bookmark
	err := tx.NewSelect().Model(&bookmarks).
		Where("bm.user_id = ?", userID).
		Where("bm.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var links []journalBookmark
	err = tx.NewSelect().Model(&links).
		Where("user_id = ?", userID).
		Where("bookmark_id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	localIDs := map[int]string{}
	for _, link := range links {
		if link.BookmarkID != nil {
			localIDs[*link.BookmarkID] = link.LocalID
		}
	}
	for _, bookmark := range bookmarks {
		doc := &FeedBookmark{
			ID: bookmark.ID, FileID: bookmark.FileID, Seconds: bookmark.Seconds, Note: bookmark.Note,
			CreatedAt: bookmark.CreatedAt.UTC(),
		}
		if localID, ok := localIDs[bookmark.ID]; ok {
			doc.LocalID = &localID
		}
		out[bookmark.ID] = doc
	}
	return out, nil
}

// fileCoverVersion keys file's own cover, or is empty when the File has no
// cover.
func fileCoverVersion(file *models.File) string {
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return ""
	}
	return covers.FileCacheKey(file)
}

// compareNames orders names as the web app lists them, ignoring case.
func compareNames(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// filePages is a CBZ's pages in the server's order (ADR 0025), or none for
// any other File Type or a CBZ that can't be read.
func filePages(ctx context.Context, file *models.File) []string {
	pages := []string{}
	if file.FileType != models.FileTypeCBZ {
		return pages
	}
	archive, err := zip.OpenReader(file.Filepath)
	if err != nil {
		// The Book goes out without them; the app reads pages from the file
		// it downloads, and gets them when the File next changes.
		logger.FromContext(ctx).Warn("CBZ pages not read for the Shisho app", logger.Data{"file_id": file.ID, "error": err.Error()})
		return pages
	}
	defer archive.Close()
	for _, page := range cbz.PageImages(&archive.Reader) {
		pages = append(pages, page.Name)
	}
	return pages
}

// aliasNames reads the aliases of the ids in table, keyed by parent id, in
// the order they were added.
func aliasNames(ctx context.Context, tx bun.IDB, table, parent string, ids []int) (map[int][]string, error) {
	var rows []struct {
		Parent int    `bun:"parent"`
		Name   string `bun:"name"`
	}
	err := tx.NewSelect().Table(table).
		ColumnExpr("? AS parent", bun.Ident(parent)).Column("name").
		Where("? IN (?)", bun.Ident(parent), bun.List(ids)).
		OrderExpr("id ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := map[int][]string{}
	for _, id := range ids {
		out[id] = []string{}
	}
	for _, row := range rows {
		out[row.Parent] = append(out[row.Parent], row.Name)
	}
	return out, nil
}

func loadSeries(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedSeries, error) {
	out := map[int]*FeedSeries{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []*models.Series
	if err := tx.NewSelect().Model(&rows).Where("s.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	aliases, err := aliasNames(ctx, tx, "series_aliases", "series_id", ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &FeedSeries{
			ID: row.ID, LibraryID: row.LibraryID, Name: row.Name, SortName: row.SortName,
			Description: row.Description, Aliases: aliases[row.ID],
		}
	}
	return out, nil
}

func loadPersons(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedPerson, error) {
	out := map[int]*FeedPerson{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []*models.Person
	if err := tx.NewSelect().Model(&rows).Where("p.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	aliases, err := aliasNames(ctx, tx, "person_aliases", "person_id", ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &FeedPerson{ID: row.ID, LibraryID: row.LibraryID, Name: row.Name, SortName: row.SortName, Aliases: aliases[row.ID]}
	}
	return out, nil
}

func loadGenres(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedGenre, error) {
	out := map[int]*FeedGenre{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []*models.Genre
	if err := tx.NewSelect().Model(&rows).Where("g.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	aliases, err := aliasNames(ctx, tx, "genre_aliases", "genre_id", ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &FeedGenre{ID: row.ID, LibraryID: row.LibraryID, Name: row.Name, Aliases: aliases[row.ID]}
	}
	return out, nil
}

func loadTags(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedTag, error) {
	out := map[int]*FeedTag{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []*models.Tag
	if err := tx.NewSelect().Model(&rows).Where("t.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	aliases, err := aliasNames(ctx, tx, "tag_aliases", "tag_id", ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &FeedTag{ID: row.ID, LibraryID: row.LibraryID, Name: row.Name, Aliases: aliases[row.ID]}
	}
	return out, nil
}

func loadPublishers(ctx context.Context, tx bun.IDB, ids []int) (map[int]*FeedPublisher, error) {
	out := map[int]*FeedPublisher{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []*models.Publisher
	if err := tx.NewSelect().Model(&rows).Where("pub.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	aliases, err := aliasNames(ctx, tx, "publisher_aliases", "publisher_id", ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = &FeedPublisher{ID: row.ID, LibraryID: row.LibraryID, Name: row.Name, ParentID: row.ParentID, Aliases: aliases[row.ID]}
	}
	return out, nil
}

// loadUserBooks reads userID's own records of the Books in bookIDs, by Book.
func loadUserBooks(ctx context.Context, tx bun.IDB, userID int, bookIDs []int) (map[int]*FeedUserBook, error) {
	out := map[int]*FeedUserBook{}
	if len(bookIDs) == 0 {
		return out, nil
	}
	var rows []*models.UserBook
	err := tx.NewSelect().Model(&rows).
		Where("ub.user_id = ?", userID).
		Where("ub.book_id IN (?)", bun.List(bookIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		out[row.BookID] = &FeedUserBook{BookID: row.BookID, Rating: row.Rating, ToReadPosition: row.ToReadPosition, Reference: row.Reference}
	}
	return out, nil
}

// loadLists reads the Lists among ids that user owns or that are shared
// with them, with only the Books in Libraries they can access.
func loadLists(ctx context.Context, tx bun.IDB, user *models.User, ids []int) (map[int]*FeedList, error) {
	out := map[int]*FeedList{}
	if len(ids) == 0 {
		return out, nil
	}
	var lists []*models.List
	err := tx.NewSelect().Model(&lists).
		Relation("User").
		Relation("ListShares", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("ls.id ASC")
		}).
		Relation("ListShares.User").
		Relation("ListBooks", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.Join("JOIN books AS lbb ON lbb.id = lb.book_id").
				OrderExpr("lb.sort_order IS NULL, lb.sort_order ASC, lb.added_at DESC, lb.id ASC")
			if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
				if len(libraryIDs) == 0 {
					return q.Where("FALSE")
				}
				q = q.Where("lbb.library_id IN (?)", bun.List(libraryIDs))
			}
			return q
		}).
		Where("l.id IN (?)", bun.List(ids)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, list := range lists {
		permission := ""
		if list.UserID == user.ID {
			permission = FeedListOwner
		}
		shares := []FeedListShare{}
		for _, share := range list.ListShares {
			if share.UserID == user.ID && permission == "" {
				permission = share.Permission
			}
			entry := FeedListShare{User: AppUser{ID: share.UserID}, Permission: share.Permission}
			if share.User != nil {
				entry.User.Username = share.User.Username
			}
			shares = append(shares, entry)
		}
		if permission == "" {
			continue
		}
		// Who else it's shared with is the owner's and managers' to see.
		if permission != FeedListOwner && permission != FeedListManager {
			shares = []FeedListShare{}
		}
		doc := &FeedList{
			ID: list.ID, Name: list.Name, Description: list.Description, IsOrdered: list.IsOrdered,
			DefaultSort: list.DefaultSort, Permission: permission, Books: []FeedListBook{}, Shares: shares,
			Owner: AppUser{ID: list.UserID},
		}
		if list.User != nil {
			doc.Owner.Username = list.User.Username
		}
		for _, listBook := range list.ListBooks {
			doc.Books = append(doc.Books, FeedListBook{BookID: listBook.BookID, SortOrder: listBook.SortOrder, AddedAt: listBook.AddedAt.UTC()})
		}
		out[list.ID] = doc
	}
	return out, nil
}

func utcPointer(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
