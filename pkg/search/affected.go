package search

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/uptrace/bun"
)

// affectedLookupChunk bounds how many ids go into one IN list, well under
// SQLite's bound-parameter limit.
const affectedLookupChunk = 500

// Affected names the entities a mutation touched. ReindexAffected expands it
// to every entity whose FTS row copies one of their columns and rewrites each
// of those rows once:
//
//   - a Person expands to the Books it authors or narrates (books_fts
//     authors and narrators);
//   - a Series expands to its Books (books_fts series_names);
//   - a Book, given or expanded, expands to every Series holding it
//     (series_fts book_titles and book_authors).
//
// Genres, Tags, and Publishers reindex their own rows only, because no other
// FTS table copies them.
type Affected struct {
	BookIDs      []int
	SeriesIDs    []int
	PersonIDs    []int
	GenreIDs     []int
	TagIDs       []int
	PublisherIDs []int
}

// CollectAffected expands a over the links as they stand now and returns the
// result for ReindexAffected. Call it before the mutation: a delete or a
// relink drops links (CASCADE removes a deleted Book's book_series rows), and
// the entities on the far side of a dropped link are found only through the
// links read here. The result is a pointer so ids learned during the mutation,
// such as a Book it creates, can be added to it before the deferred
// ReindexAffected runs.
//
// A lookup failure is logged and the ids given are kept, so the reindex still
// covers them.
func (svc *Service) CollectAffected(ctx context.Context, a Affected) *Affected {
	if svc == nil {
		return &a
	}
	expanded, err := svc.expandAffected(ctx, a)
	if err != nil {
		logger.FromContext(ctx).Warn("failed to collect entities to reindex", logger.Data{"error": err.Error()})
		return &a
	}
	return &expanded
}

// ReindexAffected rewrites the FTS row of every entity in a and in its
// expansion (see Affected), reading the links as they stand after the
// mutation. A row whose entity no longer exists is deleted. Callers defer it
// right after CollectAffected, so it runs after the mutation commits, on the
// error paths too: a handler that fails after committing part of its writes
// still leaves the index matching the database.
//
// It ignores ctx cancellation, because the writes it follows have already
// committed, and logs each failure and moves on, so a search index problem
// never fails the request or job that made the change.
func (svc *Service) ReindexAffected(ctx context.Context, a *Affected) {
	if svc == nil || a == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	log := logger.FromContext(ctx)

	expanded, err := svc.expandAffected(ctx, *a)
	if err != nil {
		log.Warn("failed to expand entities to reindex", logger.Data{"error": err.Error()})
		expanded = *a
	}

	for _, pass := range []struct {
		table ftsSource
		ids   []int
	}{
		{personsFTS, expanded.PersonIDs},
		{booksFTS, expanded.BookIDs},
		{seriesFTS, expanded.SeriesIDs},
		{genresFTS, expanded.GenreIDs},
		{tagsFTS, expanded.TagIDs},
		{publishersFTS, expanded.PublisherIDs},
	} {
		for _, id := range uniqueSorted(pass.ids) {
			if err := svc.reindexRow(ctx, pass.table, id); err != nil {
				log.Warn("failed to reindex search row", logger.Data{"table": pass.table.name, "id": id, "error": err.Error()})
			}
		}
	}
}

// expandAffected returns a with the closure described on Affected added, each
// id once.
func (svc *Service) expandAffected(ctx context.Context, a Affected) (Affected, error) {
	out := Affected{
		PersonIDs:    uniqueSorted(a.PersonIDs),
		GenreIDs:     uniqueSorted(a.GenreIDs),
		TagIDs:       uniqueSorted(a.TagIDs),
		PublisherIDs: uniqueSorted(a.PublisherIDs),
	}
	seriesIDs := uniqueSorted(a.SeriesIDs)

	personBooks, err := svc.lookupIDs(ctx, `
		SELECT book_id FROM authors WHERE person_id IN (?)
		UNION
		SELECT f.book_id FROM narrators n JOIN files f ON f.id = n.file_id WHERE n.person_id IN (?)`, out.PersonIDs)
	if err != nil {
		return a, errors.Wrap(err, "books of people")
	}
	seriesBooks, err := svc.lookupIDs(ctx, `SELECT book_id FROM book_series WHERE series_id IN (?)`, seriesIDs)
	if err != nil {
		return a, errors.Wrap(err, "books of series")
	}
	out.BookIDs = uniqueSorted(slices.Concat(a.BookIDs, personBooks, seriesBooks))

	bookSeries, err := svc.lookupIDs(ctx, `SELECT series_id FROM book_series WHERE book_id IN (?)`, out.BookIDs)
	if err != nil {
		return a, errors.Wrap(err, "series of books")
	}
	out.SeriesIDs = uniqueSorted(slices.Concat(seriesIDs, bookSeries))
	return out, nil
}

// lookupIDs runs query once per chunk of ids, binding the chunk to every
// placeholder in it, and returns the ids it selects.
func (svc *Service) lookupIDs(ctx context.Context, query string, ids []int) ([]int, error) {
	var out []int
	placeholders := strings.Count(query, "?")
	for chunk := range slices.Chunk(ids, affectedLookupChunk) {
		args := make([]any, placeholders)
		for i := range args {
			args[i] = bun.List(chunk)
		}
		var found []int
		if err := svc.db.NewRaw(query, args...).Scan(ctx, &found); err != nil {
			return nil, errors.WithStack(err)
		}
		out = append(out, found...)
	}
	return out, nil
}

// uniqueSorted returns ids sorted with duplicates and non-positive ids
// removed.
func uniqueSorted(ids []int) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
