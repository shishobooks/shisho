package books

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// emptyRemovedEntities has every list empty, not nil, so a response always
// carries arrays.
func emptyRemovedEntities() RemovedEntities {
	return RemovedEntities{
		SeriesIDs:    []int{},
		PersonIDs:    []int{},
		GenreIDs:     []int{},
		TagIDs:       []int{},
		PublisherIDs: []int{},
	}
}

// RemovedEntitiesOf reports which of the Series, People, Genres, Tags, and
// Publishers that book linked to no longer exist. A caller loads the Book
// with RetrieveBook before deleting it or one of its Files, and calls this
// afterwards. It checks what exists rather than collecting what this
// request's orphan cleanup deleted, so an entity another request's cleanup
// removed in between is reported too.
func (svc *Service) RemovedEntitiesOf(ctx context.Context, book *models.Book) (RemovedEntities, error) {
	var seriesIDs, personIDs, genreIDs, tagIDs, publisherIDs []int
	for _, bs := range book.BookSeries {
		seriesIDs = append(seriesIDs, bs.SeriesID)
	}
	for _, a := range book.Authors {
		personIDs = append(personIDs, a.PersonID)
	}
	for _, bg := range book.BookGenres {
		genreIDs = append(genreIDs, bg.GenreID)
	}
	for _, bt := range book.BookTags {
		tagIDs = append(tagIDs, bt.TagID)
	}
	for _, f := range book.Files {
		for _, n := range f.Narrators {
			personIDs = append(personIDs, n.PersonID)
		}
		if f.PublisherID != nil {
			publisherIDs = append(publisherIDs, *f.PublisherID)
		}
	}

	removed := emptyRemovedEntities()
	for _, kind := range []struct {
		table string
		ids   []int
		into  *[]int
	}{
		{"series", seriesIDs, &removed.SeriesIDs},
		{"persons", personIDs, &removed.PersonIDs},
		{"genres", genreIDs, &removed.GenreIDs},
		{"tags", tagIDs, &removed.TagIDs},
		{"publishers", publisherIDs, &removed.PublisherIDs},
	} {
		missing, err := svc.missingIDs(ctx, kind.table, kind.ids)
		if err != nil {
			return emptyRemovedEntities(), err
		}
		*kind.into = missing
	}
	return removed, nil
}

// missingIDs returns the ids that have no row in table, sorted.
func (svc *Service) missingIDs(ctx context.Context, table string, ids []int) ([]int, error) {
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(ids) == 0 {
		return []int{}, nil
	}
	var existing []int
	err := svc.db.NewSelect().
		Table(table).
		Column("id").
		Where("id IN (?)", bun.List(ids)).
		Scan(ctx, &existing)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return slices.DeleteFunc(ids, func(id int) bool {
		return slices.Contains(existing, id)
	}), nil
}

// removedEntities is RemovedEntitiesOf for a response, joined with what the
// request's own orphan cleanup deleted. That cleanup also removes entities
// the Book no longer linked to, such as the Publisher of a File deleted
// earlier while the Book stayed. The delete has already happened, so a failed
// check is logged and reports only the cleanup's deletions rather than
// failing the request.
func (h *handler) removedEntities(ctx context.Context, log logger.Logger, book *models.Book, cleaned RemovedEntities) RemovedEntities {
	removed, err := h.bookService.RemovedEntitiesOf(ctx, book)
	if err != nil {
		log.Warn("failed to check which of a deleted book's entities remain", logger.Data{"book_id": book.ID, "error": err.Error()})
	}
	join := func(a, b []int) []int {
		ids := append(append([]int{}, a...), b...)
		slices.Sort(ids)
		return slices.Compact(ids)
	}
	return RemovedEntities{
		SeriesIDs:    join(removed.SeriesIDs, cleaned.SeriesIDs),
		PersonIDs:    join(removed.PersonIDs, cleaned.PersonIDs),
		GenreIDs:     join(removed.GenreIDs, cleaned.GenreIDs),
		TagIDs:       join(removed.TagIDs, cleaned.TagIDs),
		PublisherIDs: join(removed.PublisherIDs, cleaned.PublisherIDs),
	}
}
