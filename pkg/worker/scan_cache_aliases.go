package worker

import (
	"context"

	"github.com/shishobooks/shisho/pkg/aliases"
)

// aliasServiceAdapter wraps aliases.Service to implement the AliasLister interface.
type aliasServiceAdapter struct {
	svc *aliases.Service
}

func NewAliasServiceAdapter(svc *aliases.Service) AliasLister {
	return &aliasServiceAdapter{svc: svc}
}

func (a *aliasServiceAdapter) ListPersonAliases(ctx context.Context, personID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.PersonConfig, personID)
}

func (a *aliasServiceAdapter) ListGenreAliases(ctx context.Context, genreID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.GenreConfig, genreID)
}

func (a *aliasServiceAdapter) ListTagAliases(ctx context.Context, tagID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.TagConfig, tagID)
}

func (a *aliasServiceAdapter) ListSeriesAliases(ctx context.Context, seriesID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.SeriesConfig, seriesID)
}

func (a *aliasServiceAdapter) ListPublisherAliases(ctx context.Context, publisherID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.PublisherConfig, publisherID)
}

func (a *aliasServiceAdapter) ListImprintAliases(ctx context.Context, imprintID int) ([]string, error) {
	return a.svc.ListAliases(ctx, aliases.ImprintConfig, imprintID)
}
