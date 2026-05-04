package plugins

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// enrichDeps holds dependencies for metadata persistence (apply/enrich).
// Uses interfaces to avoid circular imports with the books package.
type enrichDeps struct {
	bookStore       bookStore
	relStore        relationStore
	identStore      identifierStore
	personFinder    personFinder
	genreFinder     genreFinder
	tagFinder       tagFinder
	publisherFinder publisherFinder
	imprintFinder   imprintFinder
	searchIndexer   searchIndexer
	pageExtractor   pageExtractor
}

// bookStore provides core book and file CRUD operations.
type bookStore interface {
	UpdateBook(ctx context.Context, book *models.Book, columns []string) error
	RetrieveBook(ctx context.Context, bookID int) (*models.Book, error)
	UpdateFile(ctx context.Context, file *models.File, columns []string) error
	DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error)
	CreateNarrator(ctx context.Context, narrator *models.Narrator) error
	OrganizeBookFiles(ctx context.Context, book *models.Book) error
}

// relationStore provides book relationship CRUD operations.
type relationStore interface {
	DeleteAuthors(ctx context.Context, bookID int) error
	CreateAuthor(ctx context.Context, author *models.Author) error
	DeleteBookSeries(ctx context.Context, bookID int) error
	CreateBookSeries(ctx context.Context, bs *models.BookSeries) error
	FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error)
	DeleteBookGenres(ctx context.Context, bookID int) error
	CreateBookGenre(ctx context.Context, bg *models.BookGenre) error
	DeleteBookTags(ctx context.Context, bookID int) error
	CreateBookTag(ctx context.Context, bt *models.BookTag) error
}

// identifierStore provides file identifier CRUD operations.
type identifierStore interface {
	DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error)
	BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error
}

// personFinder finds or creates persons for author and narrator associations.
type personFinder interface {
	FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error)
}

// genreFinder finds or creates genres.
type genreFinder interface {
	FindOrCreateGenre(ctx context.Context, name string, libraryID int) (*models.Genre, error)
}

// tagFinder finds or creates tags.
type tagFinder interface {
	FindOrCreateTag(ctx context.Context, name string, libraryID int) (*models.Tag, error)
}

// publisherFinder finds or creates publishers.
type publisherFinder interface {
	FindOrCreatePublisher(ctx context.Context, name string, libraryID int) (*models.Publisher, error)
}

// imprintFinder finds or creates imprints.
type imprintFinder interface {
	FindOrCreateImprint(ctx context.Context, name string, libraryID int) (*models.Imprint, error)
}

// searchIndexer updates the search index after metadata changes. Each entity
// type has its own FTS table (books_fts, series_fts, persons_fts, genres_fts,
// tags_fts), and rows in those tables are populated only by explicit Index*
// calls — they are NOT maintained by triggers on the underlying table. Any
// entity created via the apply path must therefore be re-indexed here, or it
// will be invisible to the search-driven dropdowns in the UI.
type searchIndexer interface {
	IndexBook(ctx context.Context, book *models.Book) error
	IndexSeries(ctx context.Context, series *models.Series) error
	IndexPerson(ctx context.Context, person *models.Person) error
	IndexGenre(ctx context.Context, genre *models.Genre) error
	IndexTag(ctx context.Context, tag *models.Tag) error
	IndexPublisher(ctx context.Context, publisher *models.Publisher) error
	IndexImprint(ctx context.Context, imprint *models.Imprint) error
}

// pageExtractor renders a page from a page-based file (CBZ/PDF) and writes
// it as that file's cover image. Returns the cover filename (not a full path)
// and the MIME type of the extracted image.
type pageExtractor interface {
	ExtractCoverPage(file *models.File, bookFilepath string, page int, log logger.Logger) (filename, mimeType string, err error)
}

type handler struct {
	service   *Service
	manager   *Manager
	installer *Installer
	db        *bun.DB
	enrich    *enrichDeps
}

// NewHandler creates a handler for testing and external route registration.
func NewHandler(service *Service, manager *Manager, installer *Installer) *handler { //nolint:revive // unexported return is intentional for same-package tests
	return &handler{service: service, manager: manager, installer: installer}
}

// Exported handler methods for testing.
func (h *handler) GetImage(c echo.Context) error              { return h.getImage(c) }
func (h *handler) GetManifest(c echo.Context) error           { return h.getManifest(c) }
func (h *handler) GetLibraryOrder(c echo.Context) error       { return h.getLibraryOrder(c) }
func (h *handler) SetLibraryOrder(c echo.Context) error       { return h.setLibraryOrder(c) }
func (h *handler) ResetLibraryOrder(c echo.Context) error     { return h.resetLibraryOrder(c) }
func (h *handler) ResetAllLibraryOrders(c echo.Context) error { return h.resetAllLibraryOrders(c) }
