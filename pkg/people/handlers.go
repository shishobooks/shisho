package people

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/merge"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sortname"
)

// FileOrganizer defines the interface for organizing files when person metadata changes.
// This is used to break the import cycle between people and books packages.
type FileOrganizer interface {
	// OrganizeBookFiles reorganizes files for a book with the given ID.
	// Returns error only for critical failures (database errors, etc.).
	// File system errors are logged but don't cause failure.
	OrganizeBookFiles(ctx context.Context, bookID int) error

	// RenameNarratedFile renames an M4B file to include the updated narrator name.
	// Returns the new path, or the original path if no rename was needed.
	RenameNarratedFile(ctx context.Context, fileID int) (string, error)

	// GetLibraryOrganizeSetting checks if a library has OrganizeFileStructure enabled.
	GetLibraryOrganizeSetting(ctx context.Context, libraryID int) (bool, error)
}

type handler struct {
	personService    *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer review.BookReviewRecomputer
	fileOrganizer    FileOrganizer // optional, can be nil if not configured
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, person.LibraryID); err != nil {
		return err
	}

	// Get counts
	authoredCount, err := h.personService.GetAuthoredBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	narratedCount, err := h.personService.GetNarratedFileCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PersonConfig, id)

	response := PersonResponse{
		Person:            *person,
		AuthoredBookCount: authoredCount,
		NarratedFileCount: narratedCount,
		Aliases:           aliasList,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListPeopleQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListPeopleOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		Search:    params.Search,
	}

	// Filter by the user's library access
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
	}

	people, total, err := h.personService.ListPeopleWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with counts and aliases
	result := make([]PersonResponse, len(people))
	for i, p := range people {
		authoredCount, _ := h.personService.GetAuthoredBookCount(ctx, p.ID)
		narratedCount, _ := h.personService.GetNarratedFileCount(ctx, p.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PersonConfig, p.ID)
		result[i] = PersonResponse{
			Person:            *p,
			AuthoredBookCount: authoredCount,
			NarratedFileCount: narratedCount,
			Aliases:           aliasList,
		}
	}

	response := ListPeopleResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	params := UpdatePersonPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the person
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, person.LibraryID); err != nil {
		return err
	}

	// The name and aliases are copied into persons_fts, into books_fts for
	// every book the person authors or narrates, and (the name only) into
	// series_fts for every series holding a book they author. The rename
	// commits before SyncAliases runs, so the reindex is deferred to also
	// cover a rejected alias list, and runs after any file reorganization so
	// books_fts picks up moved paths.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PersonIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Keep track of what's been changed
	opts := UpdatePersonOptions{Columns: []string{}}
	nameChanged := false

	if params.Name != nil && *params.Name != person.Name {
		// A rename onto another Person's name is rejected rather than merged,
		// so combining two People stays an explicit merge.
		existing, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
			Name:      params.Name,
			LibraryID: &person.LibraryID,
		})
		if err == nil && existing.ID != id {
			return errcodes.ValidationError("A person with this name already exists. Merge the two people instead.")
		}
		if err != nil && !errors.Is(err, errcodes.NotFound("Person")) {
			return errors.WithStack(err)
		}

		nameChanged = true
		person.Name = *params.Name
		// Regenerate sort name when name changes (unless sort_name_source is manual)
		if person.SortNameSource != models.DataSourceManual {
			person.SortName = sortname.ForPerson(*params.Name)
			person.SortNameSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "name", "sort_name", "sort_name_source")
		} else {
			opts.Columns = append(opts.Columns, "name")
		}
	}

	if params.SortName != nil && *params.SortName != person.SortName {
		if *params.SortName == "" {
			// Empty string means regenerate from name
			person.SortName = sortname.ForPerson(person.Name)
			person.SortNameSource = models.DataSourceFilepath
		} else {
			person.SortName = *params.SortName
			person.SortNameSource = models.DataSourceManual
		}
		opts.Columns = append(opts.Columns, "sort_name", "sort_name_source")
	}

	// Update the model
	err = h.personService.UpdatePerson(ctx, person, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Sync aliases if provided
	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.PersonConfig, id, person.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
		}
	}

	// Reload the model
	person, err = h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// If name changed and file organizer is configured, reorganize associated files
	log := logger.FromContext(ctx)
	if nameChanged && h.fileOrganizer != nil {
		// Check if library has OrganizeFileStructure enabled
		organizeEnabled, err := h.fileOrganizer.GetLibraryOrganizeSetting(ctx, person.LibraryID)
		if err != nil {
			log.Warn("failed to check library organize setting", logger.Data{
				"person_id":  person.ID,
				"library_id": person.LibraryID,
				"error":      err.Error(),
			})
		} else if organizeEnabled {
			authoredBooks, err := h.personService.GetAuthoredBooks(ctx, id)
			if err != nil {
				log.Warn("failed to get authored books after person update", logger.Data{"person_id": id, "error": err.Error()})
			}
			narratedFiles, err := h.personService.GetNarratedFiles(ctx, id)
			if err != nil {
				log.Warn("failed to get narrated files after person update", logger.Data{"person_id": id, "error": err.Error()})
			}

			// Reorganize books where this person is an author
			for _, book := range authoredBooks {
				if err := h.fileOrganizer.OrganizeBookFiles(ctx, book.ID); err != nil {
					log.Warn("failed to reorganize book files after person name change", logger.Data{
						"person_id": person.ID,
						"book_id":   book.ID,
						"error":     err.Error(),
					})
				}
			}

			// Rename M4B files where this person is a narrator
			for _, file := range narratedFiles {
				if _, err := h.fileOrganizer.RenameNarratedFile(ctx, file.ID); err != nil {
					log.Warn("failed to rename narrated file after person name change", logger.Data{
						"person_id": person.ID,
						"file_id":   file.ID,
						"error":     err.Error(),
					})
				}
			}
		}
	}

	// Get counts
	authoredCount, _ := h.personService.GetAuthoredBookCount(ctx, id)
	narratedCount, _ := h.personService.GetNarratedFileCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PersonConfig, id)

	response := PersonResponse{
		Person:            *person,
		AuthoredBookCount: authoredCount,
		NarratedFileCount: narratedCount,
		Aliases:           aliasList,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) authoredBooks(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, person.LibraryID); err != nil {
		return err
	}

	books, total, err := h.personService.GetAuthoredBooksPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListAuthoredBooksResponse{Items: books, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) narratedFiles(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, person.LibraryID); err != nil {
		return err
	}

	files, total, err := h.personService.GetNarratedFilesPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListNarratedFilesResponse{Items: files, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	params := MergePeoplePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch both sides, so a missing target or source is a 404, then run the
	// shared merge checks.
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	source, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &params.SourceID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := merge.CheckPreconditions(user, "person",
		merge.Side{ID: person.ID, LibraryID: person.LibraryID},
		merge.Side{ID: source.ID, LibraryID: source.LibraryID},
	); err != nil {
		return err
	}

	// Every book either person authors or narrates lists the target name and
	// the source name and aliases, which become aliases of the target, and
	// the series holding the source's books now list the target as author.
	// The reindex drops the deleted source's row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PersonIDs: []int{id, params.SourceID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Merge source person into target (this) person
	if err := h.personService.MergePeople(ctx, id, params.SourceID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deletePerson(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Person")
	if err != nil {
		return err
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, person.LibraryID); err != nil {
		return err
	}

	// The books the person authors or narrates list their name, and so do
	// the series holding the authored books. The delete CASCADEs the links
	// away, so collect them first. The reindex drops the person's own row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PersonIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	affectedBookIDs, err := h.personService.DeletePerson(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `authors` or `narrators` is a required field).
	h.reviewRecomputer.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	return c.NoContent(http.StatusNoContent)
}
