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

func (h *handler) applyMetadata(c echo.Context) error {
	if h.enrich == nil {
		return errors.New("enrichment dependencies not available")
	}

	var payload PluginApplyPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	if title, ok := payload.Fields["title"].(string); ok && strings.TrimSpace(title) == "" {
		return errcodes.ValidationError("Title cannot be blank")
	}

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

	// Build apply-path overrides from valid selected fields and explicit
	// top-level file-name fields. A pointer to an empty or whitespace-only
	// file name is an intentional clear; an omitted pointer leaves it untouched.
	overrides := convertFieldsToOverrides(payload.Fields, md)
	if payload.FileName != nil {
		fileName := strings.TrimSpace(*payload.FileName)
		fileNameSource, err := canonicalFileNameSource(payload.FileNameSource, payload.PluginScope, payload.PluginID)
		if err != nil {
			return err
		}
		if overrides == nil {
			overrides = &ApplyOverrides{}
		}
		overrides.FileName = &fileName
		overrides.FileNameSource = fileNameSource
	}

	// Extract multi-series entries from fields (array format from identify form).
	if seriesEntries := extractSeriesEntries(payload.Fields); seriesEntries != nil {
		if overrides == nil {
			overrides = &ApplyOverrides{}
		}
		overrides.SeriesEntries = seriesEntries
	}

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
	if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, overrides, log); err != nil {
		return errors.Wrap(err, "failed to apply metadata")
	}

	// Organize files after path-affecting updates and clears. Presence matters
	// for authors, file Name, and series because empty selected values remove
	// metadata that may already be represented in an organized path.
	hasFileName := overrides != nil && overrides.FileName != nil
	hasSeriesEntries := overrides != nil && overrides.SeriesEntries != nil
	hasM4BNarrators := targetFile != nil && targetFile.FileType == models.FileTypeM4B && applyFieldSelected(overrides, "narrators")
	if strings.TrimSpace(md.Title) != "" || applyFieldSelected(overrides, "authors") || hasM4BNarrators || strings.TrimSpace(md.Series) != "" || hasFileName || hasSeriesEntries {
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

// canonicalFileNameSource maps Identify's semantic source intent to the
// canonical metadata source stored on files. A nil or empty intent preserves
// compatibility with older clients by letting persistence default to the
// specific plugin source.
func canonicalFileNameSource(intent *string, pluginScope, pluginID string) (*string, error) {
	if intent == nil || *intent == "" {
		return nil, nil
	}

	var source string
	switch *intent {
	case FileNameSourceIntentPlugin:
		source = models.PluginDataSource(pluginScope, pluginID)
	case FileNameSourceIntentUser:
		source = models.DataSourceManual
	default:
		return nil, errcodes.ValidationError("file_name_source must be one of: plugin, user")
	}

	return &source, nil
}
