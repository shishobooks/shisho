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

// orphanKind is one kind of shared resource that orphan cleanup deletes once
// nothing references it, with the FTS table it must also leave.
type orphanKind struct {
	plural   string
	singular string
	idKey    string
	cleanup  func(context.Context) ([]int, error)
	unindex  func(context.Context, int) error
}

// CleanupOrphanedEntities deletes the series, people, genres, tags, and
// publishers that no Book or File references any more, and removes each from
// its FTS table, which foreign keys do not clean up. Failures are logged and
// skipped so one failing kind does not block the rest. The book and file
// delete handlers and the worker's scan and monitor all call it.
func CleanupOrphanedEntities(ctx context.Context, log logger.Logger, svcs OrphanCleanupServices) {
	cleanupOrphanKinds(ctx, log, []orphanKind{
		{"series", "series", "series_id", svcs.Books.CleanupOrphanedSeries, svcs.Search.DeleteFromSeriesIndex},
		peopleOrphanKind(svcs),
		{"genres", "genre", "genre_id", svcs.Genres.CleanupOrphanedGenres, svcs.Search.DeleteFromGenreIndex},
		{"tags", "tag", "tag_id", svcs.Tags.CleanupOrphanedTags, svcs.Search.DeleteFromTagIndex},
		{"publishers", "publisher", "publisher_id", svcs.Publishers.CleanupOrphanedPublishers, svcs.Search.DeleteFromPublisherIndex},
	})
}

// CleanupOrphanedPeople runs only the people kind of CleanupOrphanedEntities:
// it deletes the People who no longer author a Book or narrate a File and
// removes each from persons_fts. Deleting one File of a surviving Book calls
// it, because the File's Narrators may have narrated nothing else, and the
// full sweep would widen what a File delete removes.
func CleanupOrphanedPeople(ctx context.Context, log logger.Logger, svcs OrphanCleanupServices) {
	cleanupOrphanKinds(ctx, log, []orphanKind{peopleOrphanKind(svcs)})
}

func peopleOrphanKind(svcs OrphanCleanupServices) orphanKind {
	return orphanKind{"people", "person", "person_id", svcs.People.CleanupOrphanedPeople, svcs.Search.DeleteFromPersonIndex}
}

func cleanupOrphanKinds(ctx context.Context, log logger.Logger, kinds []orphanKind) {
	for _, kind := range kinds {
		deletedIDs, err := kind.cleanup(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned "+kind.plural, logger.Data{"error": err.Error()})
			continue
		}
		for _, id := range deletedIDs {
			if err := kind.unindex(ctx, id); err != nil {
				log.Warn("failed to remove orphaned "+kind.singular+" from search index", logger.Data{kind.idKey: id, "error": err.Error()})
			}
		}
		if len(deletedIDs) > 0 {
			log.Info("cleaned up orphaned "+kind.plural, logger.Data{"count": len(deletedIDs)})
		}
	}
}
