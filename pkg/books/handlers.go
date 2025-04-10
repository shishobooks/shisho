package books

import (
	"net/http"
	"path"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	bookService *Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

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
		Books []*Book `json:"books"`
		Total int     `json:"total"`
	}{books, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

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
	fileID := c.Param("id")

	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &fileID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	filepath := path.Join(file.Book.Filepath, file.ID+file.CoverExtension())

	return errors.WithStack(c.File(filepath))
}
