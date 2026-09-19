package plugins

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// Resolve the entire collection before deleting anything. Aliases and Primary
// Names compare by resource identity; an unsuccessful lookup must not turn
// a partial result into an attributed collection.
func (h *handler) applyAuthors(ctx context.Context, book *models.Book, proposed []mediafile.ParsedAuthor, attr applyAttribution, log logger.Logger) (bool, error) {
	resolved := make([]*models.Author, 0, len(proposed))
	for _, entry := range proposed {
		if entry.Name == "" {
			continue
		}
		person, err := h.enrich.personFinder.FindOrCreatePerson(ctx, entry.Name, book.LibraryID)
		if err != nil {
			return false, errors.Wrap(err, "failed to resolve author")
		}
		var role *string
		if entry.Role != "" {
			role = &entry.Role
		}
		resolved = append(resolved, &models.Author{BookID: book.ID, PersonID: person.ID, Person: person, Role: role, SortOrder: len(resolved) + 1})
	}
	same := slices.EqualFunc(book.Authors, resolved, func(a, b *models.Author) bool {
		return a.PersonID == b.PersonID && roleValue(a.Role) == roleValue(b.Role)
	})
	if same && (len(resolved) > 0 || book.AuthorSource == "") {
		return false, nil
	}
	if !same {
		if err := h.enrich.relStore.DeleteAuthors(ctx, book.ID); err != nil {
			return false, errors.Wrap(err, "failed to delete authors")
		}
		for _, author := range resolved {
			if err := h.enrich.relStore.CreateAuthor(ctx, author); err != nil {
				return false, errors.Wrap(err, "failed to create author")
			}
			if h.enrich.searchIndexer != nil && !slices.ContainsFunc(book.Authors, func(old *models.Author) bool { return old.PersonID == author.PersonID }) {
				if err := h.enrich.searchIndexer.IndexPerson(ctx, author.Person); err != nil {
					log.Warn("failed to index author", logger.Data{"error": err.Error()})
				}
			}
		}
	}
	book.AuthorSource = ""
	if len(resolved) > 0 {
		book.AuthorSource = attr.sourceFor("authors")
	}
	if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"author_source"}); err != nil {
		return false, errors.Wrap(err, "failed to update author source")
	}
	return !same, nil
}

func roleValue(role *string) string {
	if role == nil {
		return ""
	}
	return *role
}

func (h *handler) applyGenres(ctx context.Context, book *models.Book, proposed []string, attr applyAttribution, log logger.Logger) (bool, error) {
	old := make(map[int]struct{}, len(book.BookGenres))
	for _, entry := range book.BookGenres {
		old[entry.GenreID] = struct{}{}
	}
	resolved := make(map[int]*models.Genre, len(proposed))
	for _, name := range proposed {
		if name == "" {
			continue
		}
		genre, err := h.enrich.genreFinder.FindOrCreateGenre(ctx, name, book.LibraryID)
		if err != nil {
			return false, errors.Wrap(err, "failed to resolve genre")
		}
		resolved[genre.ID] = genre
	}
	ids := make(map[int]struct{}, len(resolved))
	for id := range resolved {
		ids[id] = struct{}{}
	}
	same := equalIntSets(old, ids)
	if same && (len(resolved) > 0 || book.GenreSource == nil) {
		return false, nil
	}
	if !same {
		if err := h.enrich.relStore.DeleteBookGenres(ctx, book.ID); err != nil {
			return false, errors.Wrap(err, "failed to delete genres")
		}
		for id, genre := range resolved {
			if err := h.enrich.relStore.CreateBookGenre(ctx, &models.BookGenre{BookID: book.ID, GenreID: id}); err != nil {
				return false, errors.Wrap(err, "failed to create book genre")
			}
			if _, attached := old[id]; !attached && h.enrich.searchIndexer != nil {
				if err := h.enrich.searchIndexer.IndexGenre(ctx, genre); err != nil {
					log.Warn("failed to index genre", logger.Data{"error": err.Error()})
				}
			}
		}
	}
	book.GenreSource = collectionSource(len(resolved), attr.sourceFor("genres"))
	if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"genre_source"}); err != nil {
		return false, errors.Wrap(err, "failed to update genre source")
	}
	return !same, nil
}

func (h *handler) applyTags(ctx context.Context, book *models.Book, proposed []string, attr applyAttribution, log logger.Logger) (bool, error) {
	old := make(map[int]struct{}, len(book.BookTags))
	for _, entry := range book.BookTags {
		old[entry.TagID] = struct{}{}
	}
	resolved := make(map[int]*models.Tag, len(proposed))
	for _, name := range proposed {
		if name == "" {
			continue
		}
		tag, err := h.enrich.tagFinder.FindOrCreateTag(ctx, name, book.LibraryID)
		if err != nil {
			return false, errors.Wrap(err, "failed to resolve tag")
		}
		resolved[tag.ID] = tag
	}
	ids := make(map[int]struct{}, len(resolved))
	for id := range resolved {
		ids[id] = struct{}{}
	}
	same := equalIntSets(old, ids)
	if same && (len(resolved) > 0 || book.TagSource == nil) {
		return false, nil
	}
	if !same {
		if err := h.enrich.relStore.DeleteBookTags(ctx, book.ID); err != nil {
			return false, errors.Wrap(err, "failed to delete tags")
		}
		for id, tag := range resolved {
			if err := h.enrich.relStore.CreateBookTag(ctx, &models.BookTag{BookID: book.ID, TagID: id}); err != nil {
				return false, errors.Wrap(err, "failed to create book tag")
			}
			if _, attached := old[id]; !attached && h.enrich.searchIndexer != nil {
				if err := h.enrich.searchIndexer.IndexTag(ctx, tag); err != nil {
					log.Warn("failed to index tag", logger.Data{"error": err.Error()})
				}
			}
		}
	}
	book.TagSource = collectionSource(len(resolved), attr.sourceFor("tags"))
	if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"tag_source"}); err != nil {
		return false, errors.Wrap(err, "failed to update tag source")
	}
	return !same, nil
}

// The caller flushes narrator_source with the other file-level columns.
func (h *handler) applyNarrators(ctx context.Context, libraryID int, file *models.File, proposed []string, attr applyAttribution, log logger.Logger) (bool, error) {
	resolved := make([]*models.Narrator, 0, len(proposed))
	for _, name := range proposed {
		if name == "" {
			continue
		}
		person, err := h.enrich.personFinder.FindOrCreatePerson(ctx, name, libraryID)
		if err != nil {
			return false, errors.Wrap(err, "failed to resolve narrator")
		}
		resolved = append(resolved, &models.Narrator{FileID: file.ID, PersonID: person.ID, Person: person, SortOrder: len(resolved) + 1})
	}
	same := slices.EqualFunc(file.Narrators, resolved, func(a, b *models.Narrator) bool { return a.PersonID == b.PersonID })
	if same && (len(resolved) > 0 || file.NarratorSource == nil) {
		return false, nil
	}
	if !same {
		if _, err := h.enrich.bookStore.DeleteNarratorsForFile(ctx, file.ID); err != nil {
			return false, errors.Wrap(err, "failed to delete narrators")
		}
		for _, narrator := range resolved {
			if err := h.enrich.bookStore.CreateNarrator(ctx, narrator); err != nil {
				return false, errors.Wrap(err, "failed to create narrator")
			}
			if h.enrich.searchIndexer != nil && !slices.ContainsFunc(file.Narrators, func(old *models.Narrator) bool { return old.PersonID == narrator.PersonID }) {
				if err := h.enrich.searchIndexer.IndexPerson(ctx, narrator.Person); err != nil {
					log.Warn("failed to index narrator", logger.Data{"error": err.Error()})
				}
			}
		}
	}
	file.NarratorSource = collectionSource(len(resolved), attr.sourceFor("narrators"))
	return true, nil
}

func collectionSource(length int, source string) *string {
	if length == 0 {
		return nil
	}
	return &source
}
