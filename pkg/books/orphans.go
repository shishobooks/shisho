package books

import (
	"context"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/tags"
)

// OrphanCleanupServices are the services CleanupOrphanedEntities uses.
type OrphanCleanupServices struct {
	Books      *Service
	People     *people.Service
	Genres     *genres.Service
	Tags       *tags.Service
	Publishers *publishers.Service
	Search     *search.Service
}

// CleanupOrphanedEntities deletes the series, people, genres, tags, and
// publishers that no Book or File references any more, and removes each from
// its FTS table, which foreign keys do not clean up. Failures are logged and
// skipped so one failing kind does not block the rest. The book and file
// delete handlers and the worker's scan and monitor all call it.
func CleanupOrphanedEntities(ctx context.Context, log logger.Logger, svcs OrphanCleanupServices) {
	kinds := []struct {
		plural   string
		singular string
		idKey    string
		cleanup  func(context.Context) ([]int, error)
		unindex  func(context.Context, int) error
	}{
		{"series", "series", "series_id", svcs.Books.CleanupOrphanedSeries, svcs.Search.DeleteFromSeriesIndex},
		{"people", "person", "person_id", svcs.People.CleanupOrphanedPeople, svcs.Search.DeleteFromPersonIndex},
		{"genres", "genre", "genre_id", svcs.Genres.CleanupOrphanedGenres, svcs.Search.DeleteFromGenreIndex},
		{"tags", "tag", "tag_id", svcs.Tags.CleanupOrphanedTags, svcs.Search.DeleteFromTagIndex},
		{"publishers", "publisher", "publisher_id", svcs.Publishers.CleanupOrphanedPublishers, svcs.Search.DeleteFromPublisherIndex},
	}
	for _, kind := range kinds {
		deletedIDs, err := kind.cleanup(ctx)
		if err != nil {
			log.Err(err).Warn("failed to cleanup orphaned " + kind.plural)
			continue
		}
		for _, id := range deletedIDs {
			if err := kind.unindex(ctx, id); err != nil {
				log.Err(err).Warn("failed to remove orphaned "+kind.singular+" from search index", logger.Data{kind.idKey: id})
			}
		}
		if len(deletedIDs) > 0 {
			log.Info("cleaned up orphaned "+kind.plural, logger.Data{"count": len(deletedIDs)})
		}
	}
}
