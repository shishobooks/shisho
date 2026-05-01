package plugins

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type applyPayload struct {
	BookID      int            `json:"book_id" validate:"required"`
	FileID      *int           `json:"file_id"`
	Fields      map[string]any `json:"fields" validate:"required"`
	PluginScope string         `json:"plugin_scope" validate:"required"`
	PluginID    string         `json:"plugin_id" validate:"required"`
}

func (h *handler) applyMetadata(c echo.Context) error {
	if h.enrich == nil {
		return errors.New("enrichment dependencies not available")
	}

	var payload applyPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	// Look up plugin runtime (for httpAccess domain validation on cover download)
	rt := h.manager.GetRuntime(payload.PluginScope, payload.PluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Look up book with all relations
	book, err := h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
	if err != nil {
		return errcodes.NotFound("Book")
	}

	// Library access check
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	// Resolve target file. When the caller doesn't pin a specific FileID,
	// resolveTargetFile falls back to the first FileRoleMain — supplements
	// never represent the book, so applying enriched book-level metadata
	// to a supplement (whose Name is e.g. "Supplement.pdf") would be wrong.
	targetFile := resolveTargetFile(book.Files, payload.FileID)
	if payload.FileID != nil && targetFile == nil {
		return errcodes.NotFound("File")
	}

	// Convert fields map to ParsedMetadata
	md := convertFieldsToMetadata(payload.Fields)

	// Download cover if cover_url set
	if md.CoverURL != "" {
		manifest := rt.Manifest()
		var allowedDomains []string
		if manifest.Capabilities.HTTPAccess != nil {
			allowedDomains = manifest.Capabilities.HTTPAccess.Domains
		}
		DownloadCoverFromURL(ctx, md, allowedDomains, log)
	}

	// Persist metadata (no field filtering — user already selected fields)
	if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, nil, log); err != nil {
		return errors.Wrap(err, "failed to apply metadata")
	}

	// Organize files if title, authors, narrators, or series changed (these affect directory/file names).
	// Trim title/series first so whitespace-only values don't trigger a no-op organize pass —
	// persistMetadata already trims before persisting, so untrimmed values would never change the book.
	// organizeBookFiles checks the library's OrganizeFileStructure setting internally.
	if strings.TrimSpace(md.Title) != "" || len(md.Authors) > 0 || len(md.Narrators) > 0 || strings.TrimSpace(md.Series) != "" {
		freshBook, err := h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
		if err != nil {
			log.Warn("failed to retrieve book for file organization", logger.Data{"book_id": payload.BookID, "error": err.Error()})
		} else {
			if orgErr := h.enrich.bookStore.OrganizeBookFiles(ctx, freshBook); orgErr != nil {
				log.Warn("failed to organize book files after metadata apply", logger.Data{"book_id": book.ID, "error": orgErr.Error()})
			}
		}
	}

	// Reload and return updated book
	updatedBook, err := h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
	if err != nil {
		return errors.Wrap(err, "failed to reload book")
	}

	return c.JSON(http.StatusOK, updatedBook)
}
