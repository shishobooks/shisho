package books

import (
	"context"

	"github.com/shishobooks/shisho/pkg/models"
)

// PluginMetadataStore adapts Service to the narrow store interfaces the plugin
// apply path depends on (plugins.EnrichDeps BookStore, RelStore, IdentStore),
// like PluginPageExtractor does for cover pages.
type PluginMetadataStore struct {
	svc *Service
}

// NewPluginMetadataStore wraps svc for use as plugins.EnrichDeps stores.
func NewPluginMetadataStore(svc *Service) *PluginMetadataStore {
	return &PluginMetadataStore{svc: svc}
}

func (a *PluginMetadataStore) UpdateBook(ctx context.Context, book *models.Book, columns []string) error {
	return a.svc.UpdateBook(ctx, book, UpdateBookOptions{Columns: columns})
}

func (a *PluginMetadataStore) RetrieveBook(ctx context.Context, bookID int) (*models.Book, error) {
	return a.svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
}

func (a *PluginMetadataStore) DeleteAuthors(ctx context.Context, bookID int) error {
	return a.svc.DeleteAuthors(ctx, bookID)
}

func (a *PluginMetadataStore) CreateAuthor(ctx context.Context, author *models.Author) error {
	return a.svc.CreateAuthor(ctx, author)
}

func (a *PluginMetadataStore) DeleteBookSeries(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookSeries(ctx, bookID)
}

func (a *PluginMetadataStore) CreateBookSeries(ctx context.Context, bs *models.BookSeries) error {
	return a.svc.CreateBookSeries(ctx, bs)
}

func (a *PluginMetadataStore) FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error) {
	return a.svc.FindOrCreateSeries(ctx, name, libraryID, nameSource)
}

func (a *PluginMetadataStore) DeleteBookGenres(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookGenres(ctx, bookID)
}

func (a *PluginMetadataStore) CreateBookGenre(ctx context.Context, bg *models.BookGenre) error {
	return a.svc.CreateBookGenre(ctx, bg)
}

func (a *PluginMetadataStore) DeleteBookTags(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookTags(ctx, bookID)
}

func (a *PluginMetadataStore) CreateBookTag(ctx context.Context, bt *models.BookTag) error {
	return a.svc.CreateBookTag(ctx, bt)
}

func (a *PluginMetadataStore) DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error) {
	return a.svc.DeleteIdentifiersForFile(ctx, fileID)
}

func (a *PluginMetadataStore) BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error {
	return a.svc.BulkCreateFileIdentifiers(ctx, fileIdentifiers)
}

func (a *PluginMetadataStore) UpdateFile(ctx context.Context, file *models.File, columns []string) error {
	return a.svc.UpdateFile(ctx, file, UpdateFileOptions{Columns: columns})
}

func (a *PluginMetadataStore) DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error) {
	return a.svc.DeleteNarratorsForFile(ctx, fileID)
}

func (a *PluginMetadataStore) CreateNarrator(ctx context.Context, narrator *models.Narrator) error {
	return a.svc.CreateNarrator(ctx, narrator)
}

func (a *PluginMetadataStore) OrganizeBookFiles(ctx context.Context, book *models.Book) error {
	return a.svc.OrganizeBookFiles(ctx, book)
}
