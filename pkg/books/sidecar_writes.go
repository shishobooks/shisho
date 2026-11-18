package books

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/uptrace/bun"
)

// Sidecars name People, Series, Genres, Tags, and Publishers, so renaming or
// merging one leaves the sidecars of every Book that holds it naming it by
// an old name. Rewriting them all in the request is too slow for a resource
// on thousands of Books, so the request only marks those Books
// (books.sidecars_stale_at) and queues a sidecar rewrite job (ADR 0027).
//
// A scan never trusts a marked Book's sidecars: it rewrites them from what
// is stored before reading them (RewriteSidecarsIfStale). Otherwise a scan
// that reached the Book before the job would read the old name back, and
// either relabel the field `sidecar` or, when the old name wasn't kept as an
// alias, re-create the old resource and undo the rename.

// sidecarRewriteBatch is how many marked Books the job loads at a time.
const sidecarRewriteBatch = 100

// sidecarKind is how to find the Books whose sidecars name one kind of
// resource, and what renaming it takes.
type sidecarKind struct {
	// name is the resource's name in errors.
	name string
	// table holds the resources, for their library.
	table string
	// resource is the permission resource a rename or merge needs write on.
	resource string
	// books selects the book_id of every Book naming any of the resources
	// bound to each of its ? placeholders.
	books string
}

var sidecarKinds = map[string]sidecarKind{
	SidecarImpactPerson: {name: "Person", table: "persons", resource: models.ResourcePeople, books: `
		SELECT book_id FROM authors WHERE person_id IN (?)
		UNION
		SELECT f.book_id FROM narrators n JOIN files f ON f.id = n.file_id WHERE n.person_id IN (?)`},
	SidecarImpactSeries:    {name: "Series", table: "series", resource: models.ResourceSeries, books: `SELECT book_id FROM book_series WHERE series_id IN (?)`},
	SidecarImpactGenre:     {name: "Genre", table: "genres", resource: models.ResourceBooks, books: `SELECT book_id FROM book_genres WHERE genre_id IN (?)`},
	SidecarImpactTag:       {name: "Tag", table: "tags", resource: models.ResourceBooks, books: `SELECT book_id FROM book_tags WHERE tag_id IN (?)`},
	SidecarImpactPublisher: {name: "Publisher", table: "publishers", resource: models.ResourceBooks, books: `SELECT book_id FROM files WHERE publisher_id IN (?)`},
}

// booksArgs binds ids to every placeholder in the kind's books query.
func (k sidecarKind) booksArgs(ids []int) []any {
	args := make([]any, strings.Count(k.books, "?"))
	for i := range args {
		args[i] = bun.List(ids)
	}
	return args
}

// MarkSidecarsStaleForPerson marks every Book the Person authored or narrated
// a File of, and queues the rewrite.
func (svc *Service) MarkSidecarsStaleForPerson(ctx context.Context, personID int) {
	svc.markSidecarsStale(ctx, SidecarImpactPerson, personID)
}

// MarkSidecarsStaleForSeries marks every Book in the Series, and queues the
// rewrite.
func (svc *Service) MarkSidecarsStaleForSeries(ctx context.Context, seriesID int) {
	svc.markSidecarsStale(ctx, SidecarImpactSeries, seriesID)
}

// MarkSidecarsStaleForGenre marks every Book with the Genre, and queues the
// rewrite.
func (svc *Service) MarkSidecarsStaleForGenre(ctx context.Context, genreID int) {
	svc.markSidecarsStale(ctx, SidecarImpactGenre, genreID)
}

// MarkSidecarsStaleForTag marks every Book with the Tag, and queues the
// rewrite.
func (svc *Service) MarkSidecarsStaleForTag(ctx context.Context, tagID int) {
	svc.markSidecarsStale(ctx, SidecarImpactTag, tagID)
}

// MarkSidecarsStaleForPublisher marks every Book with a File published by
// the Publisher, and queues the rewrite.
func (svc *Service) MarkSidecarsStaleForPublisher(ctx context.Context, publisherID int) {
	svc.markSidecarsStale(ctx, SidecarImpactPublisher, publisherID)
}

// markSidecarsStale marks the Books naming the resource and queues the
// rewrite. It ignores ctx cancellation, since the rename it follows has
// committed, and logs a failure rather than failing the request for the same
// reason.
func (svc *Service) markSidecarsStale(ctx context.Context, kindName string, id int) {
	ctx = context.WithoutCancel(ctx)
	log := logger.FromContext(ctx)
	kind := sidecarKinds[kindName]

	args := append([]any{time.Now()}, kind.booksArgs([]int{id})...)
	res, err := svc.db.NewRaw("UPDATE books SET sidecars_stale_at = ? WHERE id IN ("+kind.books+")", args...).Exec(ctx)
	if err != nil {
		log.Warn("failed to mark sidecars stale after a rename", logger.Data{"kind": kindName, "id": id, "error": err.Error()})
		return
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return
	}
	if err := svc.EnsureSidecarRewriteJob(ctx); err != nil {
		log.Warn("failed to queue the sidecar rewrite job", logger.Data{"kind": kindName, "id": id, "error": err.Error()})
	}
}

// CountSidecarBooks counts the distinct Books naming any of the resources,
// which a rename of one or a merge of two marks for the rewrite job.
func (svc *Service) CountSidecarBooks(ctx context.Context, kindName string, ids []int) (int, error) {
	kind, ok := sidecarKinds[kindName]
	if !ok {
		return 0, errors.Errorf("unknown sidecar kind %q", kindName)
	}
	var count int
	err := svc.db.NewRaw("SELECT COUNT(DISTINCT book_id) FROM ("+kind.books+")", kind.booksArgs(ids)...).Scan(ctx, &count)
	return count, errors.WithStack(err)
}

// sidecarResourceLibraries returns the library of each resource found, so a
// handler can check access, and how many of ids exist.
func (svc *Service) sidecarResourceLibraries(ctx context.Context, kindName string, ids []int) ([]int, error) {
	kind := sidecarKinds[kindName]
	var libraryIDs []int
	err := svc.db.NewRaw("SELECT library_id FROM "+kind.table+" WHERE id IN (?)", bun.List(ids)).Scan(ctx, &libraryIDs)
	return libraryIDs, errors.WithStack(err)
}

// EnsureSidecarRewriteJob queues a sidecar rewrite job unless one is already
// pending. One running doesn't count: it may have passed Books marked after
// it started, which the pending one then covers.
func (svc *Service) EnsureSidecarRewriteJob(ctx context.Context) error {
	jobService := jobs.NewService(svc.db)
	jobType := models.JobTypeSidecarRewrite
	pending, err := jobService.ListJobs(ctx, jobs.ListJobsOptions{
		Type:     &jobType,
		Statuses: []string{models.JobStatusPending},
		Limit:    pointerutil.Int(1),
	})
	if err != nil {
		return errors.Wrap(err, "check for a pending sidecar rewrite job")
	}
	if len(pending) > 0 {
		return nil
	}
	return errors.Wrap(jobService.CreateJob(ctx, &models.Job{
		Type:       models.JobTypeSidecarRewrite,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobSidecarRewriteData{},
	}), "create sidecar rewrite job")
}

// HasStaleSidecars reports whether any Book is marked for a sidecar rewrite.
func (svc *Service) HasStaleSidecars(ctx context.Context) (bool, error) {
	exists, err := svc.db.NewSelect().Model((*models.Book)(nil)).
		Where("b.sidecars_stale_at IS NOT NULL").Exists(ctx)
	return exists, errors.WithStack(err)
}

// RewriteStaleSidecars rewrites the sidecars of every marked Book, the body
// of the sidecar rewrite job. It walks the marked Books in id order, so a
// Book whose sidecars can't be written stays marked without stopping the
// rest. It returns how many Books it rewrote.
func (svc *Service) RewriteStaleSidecars(ctx context.Context) (int, error) {
	rewritten := 0
	afterID := 0
	for {
		if err := ctx.Err(); err != nil {
			return rewritten, err
		}
		var ids []int
		err := svc.db.NewSelect().Model((*models.Book)(nil)).Column("b.id").
			Where("b.sidecars_stale_at IS NOT NULL").Where("b.id > ?", afterID).
			Order("b.id").Limit(sidecarRewriteBatch).Scan(ctx, &ids)
		if err != nil {
			return rewritten, errors.Wrap(err, "list books with stale sidecars")
		}
		if len(ids) == 0 {
			return rewritten, nil
		}
		for _, id := range ids {
			afterID = id
			if err := svc.rewriteBookSidecars(ctx, id); err != nil {
				logger.FromContext(ctx).Warn("failed to rewrite stale sidecars", logger.Data{"book_id": id, "error": err.Error()})
				continue
			}
			rewritten++
		}
	}
}

// RewriteSidecarsIfStale rewrites a marked Book's sidecars from what is
// stored, for a scan about to read them. It reports false when the Book is
// marked and its sidecars couldn't be rewritten, so the scan must not read
// them.
func (svc *Service) RewriteSidecarsIfStale(ctx context.Context, book *models.Book) bool {
	if book.SidecarsStaleAt == nil {
		return true
	}
	if err := svc.rewriteBookSidecars(ctx, book.ID); err != nil {
		logger.FromContext(ctx).Warn("failed to rewrite stale sidecars before reading them", logger.Data{"book_id": book.ID, "error": err.Error()})
		return false
	}
	book.SidecarsStaleAt = nil
	return true
}

// rewriteBookSidecars writes a Book's book and file sidecars from what is
// stored, then clears its mark if no rename marked it again meanwhile. The
// mark goes only after a successful write, so a server that dies mid-write
// leaves it for the next pass. A failed write leaves it too, so scans keep
// distrusting the Book's sidecars.
func (svc *Service) rewriteBookSidecars(ctx context.Context, bookID int) error {
	// The mark as stored text, compared as text on clearing: the same value
	// read back into a time.Time could format differently.
	var marked sql.NullString
	if err := svc.db.NewRaw("SELECT CAST(sidecars_stale_at AS TEXT) FROM books WHERE id = ?", bookID).
		Scan(ctx, &marked); err != nil {
		return errors.Wrap(err, "read stale sidecar mark")
	}
	if err := svc.writeBookSidecars(ctx, bookID); err != nil {
		return err
	}
	if svc.beforeClearStaleMark != nil {
		svc.beforeClearStaleMark()
	}
	if !marked.Valid {
		return nil
	}
	_, err := svc.db.NewRaw(
		"UPDATE books SET sidecars_stale_at = NULL WHERE id = ? AND CAST(sidecars_stale_at AS TEXT) = ?",
		bookID, marked.String,
	).Exec(context.WithoutCancel(ctx))
	return errors.Wrap(err, "clear stale sidecar mark")
}

func (svc *Service) writeBookSidecars(ctx context.Context, bookID int) error {
	book, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
	if err != nil {
		return errors.Wrap(err, "load book")
	}
	if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
		return errors.Wrap(err, "write book sidecar")
	}
	for _, file := range book.Files {
		if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
			return errors.Wrapf(err, "write file sidecar for file %d", file.ID)
		}
	}
	return nil
}
