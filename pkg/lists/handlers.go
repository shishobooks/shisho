package lists

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	listsService *Service
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListListsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	opts := ListListsOptions{
		UserID: user.ID,
		Limit:  &params.Limit,
		Offset: &params.Offset,
	}

	lists, total, err := h.listsService.ListListsWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	result := make([]ListResponse, len(lists))
	for i, l := range lists {
		result[i], err = h.buildListResponse(ctx, l, user)
		if err != nil {
			return err
		}
	}

	response := ListListsResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

// buildListResponse adds the count of the list's books the user can see and
// the user's effective permission to a list. The list and retrieve routes
// share it, so both return the same ListResponse.
func (h *handler) buildListResponse(ctx context.Context, list *models.List, user *models.User) (ListResponse, error) {
	bookCount, err := h.listsService.GetListBookCount(ctx, list.ID, user.GetAccessibleLibraryIDs())
	if err != nil {
		return ListResponse{}, errors.WithStack(err)
	}
	return ListResponse{
		List:       *list,
		BookCount:  bookCount,
		Permission: h.effectivePermission(ctx, list.ID, list.UserID, user.ID),
	}, nil
}

// effectivePermission returns the requesting user's effective permission on a
// list: "owner" if they own it, otherwise the highest share grant (manager >
// editor > viewer).
func (h *handler) effectivePermission(ctx context.Context, listID, ownerID, userID int) string {
	if ownerID == userID {
		return PermissionOwner
	}
	if canManage, _ := h.listsService.CanManage(ctx, listID, userID); canManage {
		return models.ListPermissionManager
	}
	if canEdit, _ := h.listsService.CanEdit(ctx, listID, userID); canEdit {
		return models.ListPermissionEditor
	}
	return models.ListPermissionViewer
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check view permission
	canView, err := h.listsService.CanView(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canView {
		return errcodes.NotFound("List")
	}

	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	response, err := h.buildListResponse(ctx, list, user)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	params := CreateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	defaultSort := ""
	if params.DefaultSort != nil {
		defaultSort = *params.DefaultSort
	}

	list, err := h.listsService.CreateList(ctx, CreateListOptions{
		UserID:      user.ID,
		Name:        params.Name,
		Description: params.Description,
		IsOrdered:   params.IsOrdered,
		DefaultSort: defaultSort,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, list))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := UpdateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to edit this list")
	}

	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	opts := UpdateListOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != list.Name {
		list.Name = *params.Name
		opts.Columns = append(opts.Columns, "name")
	}
	if params.Description != nil {
		list.Description = params.Description
		opts.Columns = append(opts.Columns, "description")
	}
	if params.IsOrdered != nil && *params.IsOrdered != list.IsOrdered {
		list.IsOrdered = *params.IsOrdered
		opts.Columns = append(opts.Columns, "is_ordered")
	}
	if params.DefaultSort != nil && *params.DefaultSort != list.DefaultSort {
		list.DefaultSort = *params.DefaultSort
		opts.Columns = append(opts.Columns, "default_sort")
	}

	err = h.listsService.UpdateList(ctx, list, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload
	list, err = h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, list))
}

func (h *handler) delete(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Only owner can delete
	isOwner, err := h.listsService.IsOwner(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !isOwner {
		return errcodes.Forbidden("Only the owner can delete this list")
	}

	err = h.listsService.DeleteList(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// Book handlers

func (h *handler) listBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check view permission
	canView, err := h.listsService.CanView(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canView {
		return errcodes.NotFound("List")
	}

	// Get list to determine default sort
	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	sort := list.DefaultSort
	if params.Sort != nil {
		sort = *params.Sort
	}

	opts := ListBooksOptions{
		ListID:     id,
		LibraryIDs: user.GetAccessibleLibraryIDs(),
		Sort:       sort,
		Limit:      &params.Limit,
		Offset:     &params.Offset,
	}

	listBooks, total, err := h.listsService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, lb := range listBooks {
		if lb.Book != nil {
			aspectRatio := ""
			if lb.Book.Library != nil {
				aspectRatio = lb.Book.Library.CoverAspectRatio
			}
			lb.Book.CoverCacheKey = covers.CacheKey(lb.Book.Files, aspectRatio)
		}
	}

	response := ListListBooksResponse{Items: listBooks, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) addBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := AddBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to add books to this list")
	}

	// Every book must exist and sit in a library the user can access. An
	// unknown id is a 404 rather than a foreign-key failure, and nothing is
	// added unless every id passes.
	bookLibraryIDs, err := h.listsService.BookLibraryIDs(ctx, params.BookIDs)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, bookID := range params.BookIDs {
		libraryID, ok := bookLibraryIDs[bookID]
		if !ok {
			return errcodes.NotFound("Book")
		}
		if !user.HasLibraryAccess(libraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.listsService.AddBooks(ctx, AddBooksOptions{
		ListID:        id,
		BookIDs:       params.BookIDs,
		AddedByUserID: user.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) removeBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := RemoveBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to remove books from this list")
	}

	err = h.listsService.RemoveBooks(ctx, RemoveBooksOptions{
		ListID:  id,
		BookIDs: params.BookIDs,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) reorderBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := ReorderBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to reorder books in this list")
	}

	err = h.listsService.ReorderBooks(ctx, ReorderBooksOptions{
		ListID:  id,
		BookIDs: params.BookIDs,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// Share handlers

func (h *handler) listShares(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission to view shares
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to view shares for this list")
	}

	shares, err := h.listsService.ListShares(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, shares))
}

func (h *handler) createShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := CreateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to share this list")
	}

	// Can't share with yourself
	if params.UserID == user.ID {
		return errcodes.ValidationError("You cannot share a list with yourself")
	}

	// Only an active user can receive a share. An unknown id gets the same
	// answer so the route cannot be used to probe for accounts.
	active, err := h.listsService.IsActiveUser(ctx, params.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !active {
		return errcodes.ValidationError("You can only share a list with an active user")
	}

	// Can't share with the list owner
	isOwner, err := h.listsService.IsOwner(ctx, id, params.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	if isOwner {
		return errcodes.ValidationError("Cannot share with the list owner")
	}

	// Can't share with someone who already has a share
	hasShare, err := h.listsService.HasShare(ctx, id, params.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	if hasShare {
		return errcodes.ValidationError("User already has access to this list")
	}

	share, err := h.listsService.CreateShare(ctx, CreateShareOptions{
		ListID:         id,
		UserID:         params.UserID,
		Permission:     params.Permission,
		SharedByUserID: user.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, share))
}

func (h *handler) updateShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	shareID, err := httputil.ParamID(c, "shareId", "Share")
	if err != nil {
		return err
	}

	params := UpdateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to update shares for this list")
	}

	err = h.listsService.UpdateShare(ctx, shareID, params.Permission)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	shareID, err := httputil.ParamID(c, "shareId", "Share")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to remove shares from this list")
	}

	err = h.listsService.DeleteShare(ctx, shareID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) checkVisibility(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	params := CheckVisibilityQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to check visibility for this list")
	}

	// TODO: Get target user's library access
	// For now, return placeholder - need to inject users service
	visible, total, err := h.listsService.CheckBookVisibility(ctx, id, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	response := CheckVisibilityResponse{Visible: visible, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

// Template handlers

func (h *handler) createFromTemplate(c echo.Context) error {
	ctx := c.Request().Context()

	templateName := c.Param("name")

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	var name, description string
	var isOrdered bool
	var defaultSort string

	switch templateName {
	case "tbr":
		name = "To Be Read"
		description = "Books I want to read next"
		isOrdered = true
		defaultSort = models.ListSortManual
	case "favorites":
		name = "Favorites"
		description = "My favorite books"
		isOrdered = false
		defaultSort = models.ListSortAddedAtDesc
	default:
		return errcodes.NotFound("Template")
	}

	list, err := h.listsService.CreateList(ctx, CreateListOptions{
		UserID:      user.ID,
		Name:        name,
		Description: &description,
		IsOrdered:   isOrdered,
		DefaultSort: defaultSort,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, list))
}

func (h *handler) templates(c echo.Context) error {
	templates := []ListTemplate{
		{
			Name:        "tbr",
			DisplayName: "To Be Read",
			Description: "Books I want to read next",
			IsOrdered:   true,
			DefaultSort: models.ListSortManual,
		},
		{
			Name:        "favorites",
			DisplayName: "Favorites",
			Description: "My favorite books",
			IsOrdered:   false,
			DefaultSort: models.ListSortAddedAtDesc,
		},
	}

	return errors.WithStack(c.JSON(http.StatusOK, templates))
}

func (h *handler) moveBookPosition(c echo.Context) error {
	ctx := c.Request().Context()

	listID, err := httputil.ParamID(c, "id", "List")
	if err != nil {
		return err
	}

	bookID, err := httputil.ParamID(c, "bookId", "Book")
	if err != nil {
		return err
	}

	params := MoveBookPositionPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, listID, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to reorder books in this list")
	}

	// Verify list is ordered
	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &listID})
	if err != nil {
		return errors.WithStack(err)
	}
	if !list.IsOrdered {
		return errcodes.ValidationError("Cannot move books in an unordered list")
	}

	err = h.listsService.MoveBookToPosition(ctx, listID, bookID, params.Position)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
