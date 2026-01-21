package lists

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
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

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	// Augment with book counts
	type ListWithCount struct {
		*models.List
		BookCount  int    `json:"book_count"`
		Permission string `json:"permission"` // owner, manager, editor, viewer
	}

	result := make([]ListWithCount, len(lists))
	libraryIDs := user.GetAccessibleLibraryIDs()

	for i, l := range lists {
		count, _ := h.listsService.GetListBookCount(ctx, l.ID, libraryIDs)

		// Determine permission level
		permission := "viewer"
		if l.UserID == user.ID {
			permission = "owner"
		} else if canManage, _ := h.listsService.CanManage(ctx, l.ID, user.ID); canManage {
			permission = "manager"
		} else if canEdit, _ := h.listsService.CanEdit(ctx, l.ID, user.ID); canEdit {
			permission = "editor"
		}

		result[i] = ListWithCount{l, count, permission}
	}

	return errors.WithStack(c.JSON(http.StatusOK, echo.Map{
		"lists": result,
		"total": total,
	}))
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	// Get book count
	libraryIDs := user.GetAccessibleLibraryIDs()
	bookCount, _ := h.listsService.GetListBookCount(ctx, id, libraryIDs)

	// Determine permission
	permission := "viewer"
	if list.UserID == user.ID {
		permission = "owner"
	} else if canManage, _ := h.listsService.CanManage(ctx, id, user.ID); canManage {
		permission = "manager"
	} else if canEdit, _ := h.listsService.CanEdit(ctx, id, user.ID); canEdit {
		permission = "editor"
	}

	return errors.WithStack(c.JSON(http.StatusOK, echo.Map{
		"list":       list,
		"book_count": bookCount,
		"permission": permission,
	}))
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	params := CreateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := UpdateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	return errors.WithStack(c.JSON(http.StatusOK, echo.Map{
		"books": listBooks,
		"total": total,
	}))
}

func (h *handler) addBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := AddBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to add books to this list")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := RemoveBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := ReorderBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := CreateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	shareID, err := strconv.Atoi(c.Param("shareId"))
	if err != nil {
		return errcodes.NotFound("Share")
	}

	params := UpdateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	shareID, err := strconv.Atoi(c.Param("shareId"))
	if err != nil {
		return errcodes.NotFound("Share")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := CheckVisibilityQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	return errors.WithStack(c.JSON(http.StatusOK, echo.Map{
		"visible": visible,
		"total":   total,
	}))
}

// Template handlers

func (h *handler) createFromTemplate(c echo.Context) error {
	ctx := c.Request().Context()

	templateName := c.Param("name")

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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
	templates := []map[string]interface{}{
		{
			"name":         "tbr",
			"display_name": "To Be Read",
			"description":  "Books I want to read next",
			"is_ordered":   true,
			"default_sort": models.ListSortManual,
		},
		{
			"name":         "favorites",
			"display_name": "Favorites",
			"description":  "My favorite books",
			"is_ordered":   false,
			"default_sort": models.ListSortAddedAtDesc,
		},
	}

	return errors.WithStack(c.JSON(http.StatusOK, templates))
}

func (h *handler) moveBookPosition(c echo.Context) error {
	ctx := c.Request().Context()

	listID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	bookID, err := strconv.Atoi(c.Param("bookId"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	params := MoveBookPositionPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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
