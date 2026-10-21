// Package bookpreviews picks a few books to show as covers on each entry of a
// people, genre, tag, or publisher list. It sits below those packages because
// pkg/books imports them.
package bookpreviews

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

// Limit is the most books an entry previews.
const Limit = 3

// Source selects the books linked to an entity, as rows of (entity_id,
// book_id, priority) for the entity ids bound to every `?`. Lower priority
// books come first; within a priority, the newest released, then the newest
// added.
type Source struct {
	links string
	binds int
}

var (
	// Genres previews the books in each genre.
	Genres = Source{links: `SELECT genre_id, book_id, 0 FROM book_genres WHERE genre_id IN (?)`, binds: 1}
	// Tags previews the books with each tag.
	Tags = Source{links: `SELECT tag_id, book_id, 0 FROM book_tags WHERE tag_id IN (?)`, binds: 1}
	// People previews the books each person wrote, then the books of files
	// they narrated, so a narrator who wrote nothing still shows covers.
	People = Source{links: `SELECT person_id, book_id, 0 FROM authors WHERE person_id IN (?)
		UNION ALL
		SELECT n.person_id, f.book_id, 1 FROM narrators n JOIN files f ON f.id = n.file_id WHERE n.person_id IN (?)`, binds: 2}
	// Publishers previews the books of each publisher's files and its
	// sub-publishers' files, as a publisher's file count counts both.
	Publishers = Source{links: `SELECT tree.root_id, f.book_id, 0 FROM (
			WITH RECURSIVE publisher_tree(root_id, id) AS (
				SELECT id, id FROM publishers WHERE id IN (?)
				UNION
				SELECT pt.root_id, p.id FROM publishers p JOIN publisher_tree pt ON p.parent_id = pt.id
			)
			SELECT root_id, id FROM publisher_tree
		) tree JOIN files f ON f.publisher_id = tree.id`, binds: 1}
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// Previews holds each entity's preview books, keyed by entity id.
type Previews map[int][]PreviewBook

// For returns an entity's preview books, an empty slice when it has none so
// the field serializes as [].
func (p Previews) For(entityID int) []PreviewBook {
	if books := p[entityID]; books != nil {
		return books
	}
	return []PreviewBook{}
}

type pick struct {
	EntityID int `bun:"entity_id"`
	BookID   int `bun:"book_id"`
}

// Load returns up to Limit preview books for each of entityIDs, in display
// order, in one query for the picks and one for the books. Entities without
// books are absent from the map.
func (s *Service) Load(ctx context.Context, source Source, entityIDs []int) (Previews, error) {
	result := Previews{}
	if len(entityIDs) == 0 {
		return result, nil
	}

	// Release dates come from the release-date sort's definition, so the
	// covers lead with the books that sort reads as newest. Undated books
	// follow, newest added first.
	query := fmt.Sprintf(`WITH links(entity_id, book_id, priority) AS (%s),
		deduped AS (
			SELECT entity_id, book_id, MIN(priority) AS priority
			FROM links
			GROUP BY entity_id, book_id
		),
		dated AS (
			SELECT d.entity_id, d.book_id, d.priority, b.created_at, %s AS released
			FROM deduped d
			JOIN books b ON b.id = d.book_id
		),
		ranked AS (
			SELECT entity_id, book_id,
				ROW_NUMBER() OVER (
					PARTITION BY entity_id
					ORDER BY priority, released IS NULL, released DESC, created_at DESC, book_id DESC
				) AS rn
			FROM dated
		)
		SELECT entity_id, book_id FROM ranked WHERE rn <= ? ORDER BY entity_id, rn`,
		source.links, sortspec.BookReleaseDate)

	args := make([]any, 0, source.binds+1)
	for range source.binds {
		args = append(args, bun.List(entityIDs))
	}
	args = append(args, Limit)

	var picks []pick
	if err := s.db.NewRaw(query, args...).Scan(ctx, &picks); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(picks) == 0 {
		return result, nil
	}

	bookIDs := make([]int, 0, len(picks))
	for _, p := range picks {
		bookIDs = append(bookIDs, p.BookID)
	}
	var books []*models.Book
	err := s.db.NewSelect().
		Model(&books).
		// Only the files a cover can come from; supplements never are.
		Relation("Files", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("f.file_role != ?", models.FileRoleSupplement)
		}).
		Relation("Library").
		Where("b.id IN (?)", bun.List(bookIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	byID := make(map[int]*models.Book, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}

	for _, p := range picks {
		book := byID[p.BookID]
		if book == nil {
			continue
		}
		aspectRatio := ""
		if book.Library != nil {
			aspectRatio = book.Library.CoverAspectRatio
		}
		result[p.EntityID] = append(result[p.EntityID], PreviewBook{
			ID:            book.ID,
			Title:         book.Title,
			CoverCacheKey: covers.CacheKey(book.Files, aspectRatio),
			CoverAspect:   coverAspect(book.Files, aspectRatio),
		})
	}
	return result, nil
}

// coverAspect is the shape the gallery gives a book's cover (BookItem's
// getAspectRatioClass): the library's preference when it names one shape,
// else the shape of the file the cover comes from, else, with no cover, the
// file types the book has.
func coverAspect(files []*models.File, libraryAspect string) string {
	switch libraryAspect {
	case models.CoverAspectRatioAudiobook:
		return covers.ThumbnailAspectSquare
	case models.CoverAspectRatioBook:
		return covers.ThumbnailAspectBook
	}
	if f := covers.SelectFile(files, libraryAspect); f != nil {
		if f.FileType == models.FileTypeM4B {
			return covers.ThumbnailAspectSquare
		}
		return covers.ThumbnailAspectBook
	}
	var hasBook, hasAudiobook bool
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			continue
		}
		hasBook = hasBook || models.IsEbookFileType(f.FileType)
		hasAudiobook = hasAudiobook || f.FileType == models.FileTypeM4B
	}
	if hasAudiobook && (libraryAspect == models.CoverAspectRatioAudiobookFallbackBook || !hasBook) {
		return covers.ThumbnailAspectSquare
	}
	return covers.ThumbnailAspectBook
}
