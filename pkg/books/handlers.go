package books

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
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

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	books, total, err := h.bookService.ListBooksWithTotal(ctx, ListBooksOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
	})
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

func (h *handler) listSeries(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListSeriesQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	series, total, err := h.bookService.ListSeriesWithTotal(ctx, ListSeriesOptions{
		Limit:        &params.Limit,
		Offset:       &params.Offset,
		includeTotal: true,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	response := map[string]interface{}{
		"series": series,
		"total":  total,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) seriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	seriesName := c.Param("name")

	// URL decode the series name
	decodedSeriesName, err := url.QueryUnescape(seriesName)
	if err != nil {
		return errcodes.NotFound("Series")
	}

	books, err := h.bookService.ListBooks(ctx, ListBooksOptions{
		Series: &decodedSeriesName,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, books))
}

func (h *handler) seriesCover(c echo.Context) error {
	ctx := c.Request().Context()
	seriesName := c.Param("name")

	// URL decode the series name
	decodedSeriesName, err := url.QueryUnescape(seriesName)
	if err != nil {
		return errcodes.NotFound("Series")
	}

	// Get the first book in the series
	book, err := h.bookService.GetFirstBookInSeries(ctx, decodedSeriesName)
	if err != nil {
		return errors.WithStack(err)
	}

	// Use the book's cover image
	coverImageFileName := book.ResolveCoverImage()
	if coverImageFileName == "" {
		return errcodes.NotFound("Series cover")
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

	// Construct full path to the cover file
	coverImagePath := filepath.Join(coverDir, coverImageFileName)

	// Set appropriate headers
	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	return errors.WithStack(c.File(coverImagePath))
}
