package books

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

type handler struct {
	bookService *Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListBooksOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		SeriesID:  params.SeriesID,
	}

	// Filter by user's library access if user is in context
	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	books, total, err := h.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Books []*models.Book `json:"books"`
		Total int            `json:"total"`
	}{books, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	// Bind params.
	params := UpdateBookPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the book.
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Keep track of what's been changed.
	opts := UpdateBookOptions{Columns: []string{}}

	if params.Title != nil && *params.Title != book.Title {
		book.Title = *params.Title
		opts.Columns = append(opts.Columns, "title")
	}

	// Update the model.
	err = h.bookService.UpdateBook(ctx, book, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload the model.
	book, err = h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Write sidecar files to keep them in sync with the database
	log := logger.FromContext(ctx)
	if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
		log.Warn("failed to write book sidecar", logger.Data{"error": err.Error()})
	}
	// Also write file sidecars for all files in the book
	for _, file := range book.Files {
		if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
			log.Warn("failed to write file sidecar", logger.Data{"file_id": file.ID, "error": err.Error()})
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

func (h *handler) fileCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	filepath := path.Join(file.Book.Filepath, c.Param("id")+file.CoverExtension())

	return errors.WithStack(c.File(filepath))
}

func (h *handler) bookCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	coverImage := book.ResolveCoverImage()
	if coverImage == "" {
		return errcodes.NotFound("Cover")
	}

	// Determine if this is a root-level book by checking if book.Filepath is a file
	isRootLevelBook := false
	if info, err := os.Stat(book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}

	// Determine the directory where covers are located
	var coverDir string
	if isRootLevelBook {
		// For root-level books, covers are in the same directory as the file
		coverDir = filepath.Dir(book.Filepath)
	} else {
		// For directory-based books, covers are in the book directory
		coverDir = book.Filepath
	}

	coverPath := filepath.Join(coverDir, coverImage)
	return errors.WithStack(c.File(coverPath))
}
