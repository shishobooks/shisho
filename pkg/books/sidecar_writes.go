package books

import (
	"context"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// Sidecars name People, Series, Genres, Tags, and Publishers, so renaming or
// merging one rewrites the sidecars of every Book that holds it. Otherwise
// the next scan reads the old name back from a stale sidecar: it would
// re-create a renamed resource under its old name, or relabel the field
// `sidecar` and lock it against plugins (ADR 0027). Organizing renames
// folders but doesn't rewrite every sidecar, so this runs either way.
//
// Writes are best-effort, as for every other sidecar write: a Book whose
// sidecars can't be written, such as one in a read-only library, is logged
// and skipped, and the rename still stands.

// WriteSidecarsForPerson rewrites the sidecars of every Book the Person
// authored or narrated a File of.
func (svc *Service) WriteSidecarsForPerson(ctx context.Context, personID int) {
	svc.writeSidecarsFor(ctx, "person", personID, `
		SELECT book_id FROM authors WHERE person_id = ?
		UNION
		SELECT f.book_id FROM narrators n JOIN files f ON f.id = n.file_id WHERE n.person_id = ?`, personID, personID)
}

// WriteSidecarsForSeries rewrites the sidecars of every Book in the Series.
func (svc *Service) WriteSidecarsForSeries(ctx context.Context, seriesID int) {
	svc.writeSidecarsFor(ctx, "series", seriesID, `SELECT book_id FROM book_series WHERE series_id = ?`, seriesID)
}

// WriteSidecarsForGenre rewrites the sidecars of every Book with the Genre.
func (svc *Service) WriteSidecarsForGenre(ctx context.Context, genreID int) {
	svc.writeSidecarsFor(ctx, "genre", genreID, `SELECT book_id FROM book_genres WHERE genre_id = ?`, genreID)
}

// WriteSidecarsForTag rewrites the sidecars of every Book with the Tag.
func (svc *Service) WriteSidecarsForTag(ctx context.Context, tagID int) {
	svc.writeSidecarsFor(ctx, "tag", tagID, `SELECT book_id FROM book_tags WHERE tag_id = ?`, tagID)
}

// WriteSidecarsForPublisher rewrites the sidecars of every Book with a File
// published by the Publisher.
func (svc *Service) WriteSidecarsForPublisher(ctx context.Context, publisherID int) {
	svc.writeSidecarsFor(ctx, "publisher", publisherID, `SELECT DISTINCT book_id FROM files WHERE publisher_id = ?`, publisherID)
}

// writeSidecarsFor rewrites the book and file sidecars of every Book query
// selects. It ignores ctx cancellation: the rename it follows has committed,
// and a sidecar left with the old name is what this exists to prevent.
func (svc *Service) writeSidecarsFor(ctx context.Context, kind string, id int, query string, args ...any) {
	ctx = context.WithoutCancel(ctx)
	log := logger.FromContext(ctx)

	var bookIDs []int
	if err := svc.db.NewRaw(query, args...).Scan(ctx, &bookIDs); err != nil {
		log.Warn("failed to list books whose sidecars name a renamed resource", logger.Data{"kind": kind, "id": id, "error": err.Error()})
		return
	}
	for _, bookID := range bookIDs {
		book, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
		if err != nil {
			log.Warn("failed to load book to rewrite its sidecars", logger.Data{"kind": kind, "id": id, "book_id": bookID, "error": err.Error()})
			continue
		}
		if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
			log.Warn("failed to rewrite book sidecar", logger.Data{"kind": kind, "id": id, "book_id": bookID, "error": err.Error()})
		}
		for _, file := range book.Files {
			if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
				log.Warn("failed to rewrite file sidecar", logger.Data{"kind": kind, "id": id, "file_id": file.ID, "error": err.Error()})
			}
		}
	}
}
