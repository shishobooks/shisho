package publishers

import (
	"context"
	"net/http"
	"strings"

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
)

type handler struct {
	publisherService *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer review.BookReviewRecomputer
}

// setParentError renders a SetParent failure. A parent the hierarchy cannot
// accept is a 422; a publisher that does not exist keeps its 404, and any
// other failure is a 500.
func setParentError(err error) error {
	if errors.Is(err, ErrInvalidParent) || errors.Is(err, ErrParentOtherLibrary) || errors.Is(err, ErrParentCycle) {
		return errcodes.ValidationError(err.Error())
	}
	return errors.WithStack(err)
}

// buildPublisherResponse assembles the full single-publisher API response
// (PublisherResponse) for the given publisher: rolled-up file counts, aliases
// as a flat []string, the ancestor chain, descendant ids, and flattened direct
// children. retrieve, update, and merge all use this so a mutation returns the
// same full shape the detail page reads (enabling setQueryData on the client).
func (h *handler) buildPublisherResponse(ctx context.Context, publisher *models.Publisher) (PublisherResponse, error) {
	id := publisher.ID

	fileCount, err := h.publisherService.GetFileCount(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	ancestors, err := h.publisherService.GetAncestors(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}
	ancestorList := make([]AncestorResponse, len(ancestors))
	for i, a := range ancestors {
		ancestorList[i] = AncestorResponse{ID: a.ID, Name: a.Name}
	}

	descendantIDs, err := h.publisherService.GetDescendantIDs(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	children, err := h.publisherService.GetChildren(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}
	childList := make([]ChildResponse, len(children))
	for i, ch := range children {
		childList[i] = ChildResponse{ID: ch.ID, Name: ch.Name, FileCount: ch.FileCount}
	}

	descendantFileCount, err := h.publisherService.GetFileCountForPublisherIDs(ctx, descendantIDs)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	return PublisherResponse{
		Publisher:           *publisher,
		FileCount:           fileCount,
		DescendantFileCount: descendantFileCount,
		Aliases:             aliasList,
		Ancestors:           ancestorList,
		DescendantIDs:       descendantIDs,
		Children:            childList,
	}, nil
}

// buildPublisherListItem assembles one row of the publisher list: the file
// counts, the descendant counts, the parent's name, and the flat alias list.
// pageNames maps the ids of the publishers on the current page to their names,
// so a parent on the same page needs no lookup. A failed lookup fails the
// request instead of rendering a zero count or no aliases.
func (h *handler) buildPublisherListItem(ctx context.Context, p *models.Publisher, pageNames map[int]string) (PublisherListItem, error) {
	fileCount, err := h.publisherService.GetFileCount(ctx, p.ID)
	if err != nil {
		return PublisherListItem{}, errors.WithStack(err)
	}
	descendantIDs, err := h.publisherService.GetDescendantIDs(ctx, p.ID)
	if err != nil {
		return PublisherListItem{}, errors.WithStack(err)
	}
	descendantFileCount, err := h.publisherService.GetFileCountForPublisherIDs(ctx, descendantIDs)
	if err != nil {
		return PublisherListItem{}, errors.WithStack(err)
	}
	aliasList, err := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, p.ID)
	if err != nil {
		return PublisherListItem{}, errors.WithStack(err)
	}

	var parentName *string
	if p.ParentID != nil {
		if name, ok := pageNames[*p.ParentID]; ok {
			parentName = &name
		} else {
			// The parent is not on the current page, so look it up. A
			// dangling parent_id leaves the name empty rather than failing
			// the whole page with a 404.
			parent, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: p.ParentID})
			if err != nil && !errors.Is(err, errcodes.NotFound("Publisher")) {
				return PublisherListItem{}, errors.WithStack(err)
			}
			if err == nil {
				parentName = &parent.Name
			}
		}
	}

	return PublisherListItem{
		Publisher:                *p,
		FileCount:                fileCount,
		DescendantFileCount:      descendantFileCount,
		DescendantPublisherCount: len(descendantIDs),
		ParentName:               parentName,
		Aliases:                  aliasList,
	}, nil
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, publisher.LibraryID); err != nil {
		return err
	}

	response, err := h.buildPublisherResponse(ctx, publisher)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListPublishersQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListPublishersOptions{
		Limit:      &params.Limit,
		Offset:     &params.Offset,
		LibraryID:  params.LibraryID,
		Search:     params.Search,
		ExcludeIDs: params.ExcludeIDs,
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
	}

	publishers, total, err := h.publisherService.ListPublishersWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Build a lookup map of publisher ID -> name for parent resolution
	publisherNameMap := make(map[int]string, len(publishers))
	for _, p := range publishers {
		publisherNameMap[p.ID] = p.Name
	}

	result := make([]PublisherListItem, len(publishers))
	for i, p := range publishers {
		result[i], err = h.buildPublisherListItem(ctx, p, publisherNameMap)
		if err != nil {
			return err
		}
	}

	response := ListPublishersResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	params := UpdatePublisherPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, publisher.LibraryID); err != nil {
		return err
	}

	// The rename commits before SyncAliases runs, so the reindex is deferred
	// to also cover a rejected alias list. A rename onto an existing publisher
	// merges into it, and the reindex drops the merged row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PublisherIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Handle parent update before name-change/merge so that the parent change
	// applies even when a rename triggers a merge (which returns early).
	// resolvedParentID tracks the parent ID resolved from either path so the
	// merge-transfer block below can use it.
	var resolvedParentID *int
	var parentWasSet bool
	if params.ParentID.Set {
		resolvedParentID = params.ParentID.Value
		parentWasSet = true
		if err := h.publisherService.SetParent(ctx, id, params.ParentID.Value); err != nil {
			return setParentError(err)
		}
	} else if params.ParentName != nil {
		// Resolve parent by name: find or create a publisher with the given name
		// in the same library, then set it as the parent.
		parentPublisher, err := h.publisherService.FindOrCreatePublisher(ctx, *params.ParentName, publisher.LibraryID)
		if err != nil {
			return errors.WithStack(err)
		}
		resolvedParentID = &parentPublisher.ID
		parentWasSet = true
		// Index the parent publisher in case it was just created
		affected.PublisherIDs = append(affected.PublisherIDs, parentPublisher.ID)
		if err := h.publisherService.SetParent(ctx, id, &parentPublisher.ID); err != nil {
			return setParentError(err)
		}
	}

	if params.Name != nil && *params.Name != publisher.Name {
		newName := strings.TrimSpace(*params.Name)
		if newName == "" {
			return errcodes.ValidationError("Publisher name cannot be empty")
		}

		existing, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
			Name:      &newName,
			LibraryID: &publisher.LibraryID,
		})
		if err == nil && existing.ID != id {
			// If a parent was set on the source publisher above, transfer it to
			// the merge target so the intent is preserved.
			if parentWasSet {
				if err := h.publisherService.SetParent(ctx, existing.ID, resolvedParentID); err != nil {
					// Non-fatal: merge succeeded, log and continue
					log := logger.FromContext(ctx)
					log.Warn("failed to set parent on merge target", logger.Data{"publisher_id": existing.ID, "error": err.Error()})
				}
			}

			affected.PublisherIDs = append(affected.PublisherIDs, existing.ID)
			err = h.publisherService.MergePublishers(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			// Re-retrieve to pick up parent_id change
			existing, err = h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &existing.ID})
			if err != nil {
				return errors.WithStack(err)
			}
			response, err := h.buildPublisherResponse(ctx, existing)
			if err != nil {
				return err
			}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		publisher.Name = newName
		opts := UpdatePublisherOptions{Columns: []string{"name"}}
		err = h.publisherService.UpdatePublisher(ctx, publisher, opts)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.PublisherConfig, id, publisher.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
		}
	}

	publisher, err = h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	response, err := h.buildPublisherResponse(ctx, publisher)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, publisher.LibraryID); err != nil {
		return err
	}

	files, total, err := h.publisherService.GetFilesPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListPublisherFilesResponse{Items: files, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	params := MergePublishersPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch both sides, so a missing target or source is a 404, then run the
	// shared merge checks.
	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	source, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &params.SourceID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := merge.CheckPreconditions(user, "publisher",
		merge.Side{ID: publisher.ID, LibraryID: publisher.LibraryID},
		merge.Side{ID: source.ID, LibraryID: source.LibraryID},
	); err != nil {
		return err
	}

	// The target's publishers_fts row gains the source name as an alias, and
	// the reindex drops the deleted source's row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PublisherIDs: []int{id, params.SourceID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	if err := h.publisherService.MergePublishers(ctx, id, params.SourceID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) setChild(c echo.Context) error {
	ctx := c.Request().Context()
	parentID, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	params := SetChildPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	parent, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &parentID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, parent.LibraryID); err != nil {
		return err
	}

	// SetParent validates same-library, cycle detection, and sets the parent
	if err := h.publisherService.SetParent(ctx, params.ChildID, &parentID); err != nil {
		return setParentError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deletePublisher(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Publisher")
	if err != nil {
		return err
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, publisher.LibraryID); err != nil {
		return err
	}

	// The reindex drops the deleted publisher's publishers_fts row. books_fts
	// has no publisher column, so the affected Books need no reindex.
	affected := h.searchService.CollectAffected(ctx, search.Affected{PublisherIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	affectedBookIDs, err := h.publisherService.DeletePublisher(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Clearing publisher_id can flip the books' Reviewed completeness state
	// (when `publisher` is a required field), so recompute it for every
	// affected book.
	h.reviewRecomputer.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	return c.NoContent(http.StatusNoContent)
}
