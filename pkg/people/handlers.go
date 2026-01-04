package people

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sortname"
)

type handler struct {
	personService *Service
	searchService *search.Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
	}

	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
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

	response := struct {
		*models.Person
		AuthoredBookCount int `json:"authored_book_count"`
		NarratedFileCount int `json:"narrated_file_count"`
	}{person, authoredCount, narratedCount}

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

	// Filter by user's library access if user is in context
	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	people, total, err := h.personService.ListPeopleWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with counts
	type PersonWithCounts struct {
		*models.Person
		AuthoredBookCount int `json:"authored_book_count"`
		NarratedFileCount int `json:"narrated_file_count"`
	}
	result := make([]PersonWithCounts, len(people))
	for i, p := range people {
		authoredCount, _ := h.personService.GetAuthoredBookCount(ctx, p.ID)
		narratedCount, _ := h.personService.GetNarratedFileCount(ctx, p.ID)
		result[i] = PersonWithCounts{p, authoredCount, narratedCount}
	}

	response := map[string]interface{}{
		"people": result,
		"total":  total,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
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
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Keep track of what's been changed
	opts := UpdatePersonOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != person.Name {
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

	// Reload the model
	person, err = h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Update FTS index for this person
	log := logger.FromContext(ctx)
	if err := h.searchService.IndexPerson(ctx, person); err != nil {
		log.Warn("failed to update search index for person", logger.Data{"person_id": person.ID, "error": err.Error()})
	}

	// Get counts
	authoredCount, _ := h.personService.GetAuthoredBookCount(ctx, id)
	narratedCount, _ := h.personService.GetNarratedFileCount(ctx, id)

	response := struct {
		*models.Person
		AuthoredBookCount int `json:"authored_book_count"`
		NarratedFileCount int `json:"narrated_file_count"`
	}{person, authoredCount, narratedCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) authoredBooks(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	books, err := h.personService.GetAuthoredBooks(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, books))
}

func (h *handler) narratedFiles(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	files, err := h.personService.GetNarratedFiles(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, files))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
	}

	params := MergePeoplePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the target person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Merge source person into target (this) person
	err = h.personService.MergePeople(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deletePerson(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Person")
	}

	// Fetch the person to check library access
	person, err := h.personService.RetrievePerson(ctx, RetrievePersonOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(person.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.personService.DeletePerson(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromPersonIndex(ctx, id); err != nil {
		log.Warn("failed to remove person from search index", logger.Data{"person_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
