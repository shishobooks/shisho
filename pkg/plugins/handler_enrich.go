package plugins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// searchMetadata runs search() across all enricher plugins available for manual invocation
// and returns aggregated results.
func (h *handler) searchMetadata(c echo.Context) error {
	ctx := c.Request().Context()

	var payload PluginSearchPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	// Look up the book with relations first (needed for library access check and libraryID)
	var book *models.Book
	var err error
	if h.enrich != nil {
		book, err = h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
	} else if h.db != nil {
		var b models.Book
		err = h.db.NewSelect().Model(&b).
			Where("b.id = ?", payload.BookID).
			Relation("Files").
			Scan(ctx)
		if err == nil {
			book = &b
		}
	} else {
		return errcodes.BadRequest("search dependencies not available")
	}
	if err != nil || book == nil {
		return errcodes.NotFound("Book")
	}

	// Check library access
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	// Get enricher runtimes available for manual invocation using the book's library
	runtimes, err := h.manager.GetManualRuntimes(ctx, models.PluginHookMetadataEnricher, book.LibraryID)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(runtimes) == 0 {
		return c.JSON(http.StatusOK, PluginSearchResponse{
			Results:      []EnrichSearchResult{},
			TotalPlugins: 0,
		})
	}

	// Build flat search context from payload
	searchCtx := map[string]interface{}{
		"query": payload.Query,
	}
	if payload.Author != "" {
		searchCtx["author"] = payload.Author
	}
	if len(payload.Identifiers) > 0 {
		ids := make([]map[string]interface{}, len(payload.Identifiers))
		for i, id := range payload.Identifiers {
			ids[i] = map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			}
		}
		searchCtx["identifiers"] = ids
	}

	// Select the target file. resolveTargetFile prefers an explicitly pinned
	// FileID, otherwise falls back to the first FileRoleMain — supplements
	// never represent the book, so feeding their hints to enrichers (or
	// scoping the read-only sandbox to a supplement's path) would mislead
	// enrichment for books like an M4B + supplement-PDF where the supplement
	// could land first in book.Files.
	targetFile := resolveTargetFile(book.Files, payload.FileID)

	// Add file hints from the target file (non-modifiable context)
	var fileType string
	var targetFilePath string
	if targetFile != nil {
		f := targetFile
		fileType = f.FileType
		fileCtx := map[string]interface{}{
			"fileType": f.FileType,
			"filePath": f.Filepath,
		}
		if f.AudiobookDurationSeconds != nil {
			fileCtx["duration"] = *f.AudiobookDurationSeconds
		}
		if f.PageCount != nil {
			fileCtx["pageCount"] = *f.PageCount
		}
		fileCtx["filesizeBytes"] = f.FilesizeBytes
		searchCtx["file"] = fileCtx
		targetFilePath = f.Filepath
	}

	log := logger.FromContext(ctx)
	runSearch := func(ctx context.Context, rt *Runtime) (*SearchResponse, error) {
		return h.manager.RunMetadataSearch(ctx, rt, searchCtx, targetFilePath)
	}
	disabledFields := func(ctx context.Context, rt *Runtime) []string {
		manifest := rt.Manifest()
		if manifest.Capabilities.MetadataEnricher == nil {
			return nil
		}
		declaredFields := manifest.Capabilities.MetadataEnricher.Fields
		effectiveSettings, fErr := h.service.GetEffectiveFieldSettings(ctx, book.LibraryID, rt.Scope(), rt.PluginID(), declaredFields)
		if fErr != nil {
			return nil
		}
		var out []string
		for field, enabled := range effectiveSettings {
			if !enabled {
				out = append(out, field)
			}
		}
		return out
	}

	allResults, pluginErrors, skippedPlugins := aggregateEnricherSearches(ctx, runtimes, fileType, runSearch, disabledFields, log)

	return c.JSON(http.StatusOK, PluginSearchResponse{
		Results:        allResults,
		Errors:         pluginErrors,
		SkippedPlugins: skippedPlugins,
		TotalPlugins:   len(runtimes),
	})
}

// enricherRuntime is the subset of *Runtime that aggregateEnricherSearches
// needs. It exists so the aggregation loop can be unit-tested with a fake
// runtime instead of a full Goja VM.
type enricherRuntime interface {
	Scope() string
	PluginID() string
	Manifest() *Manifest
}

// aggregateEnricherSearches runs each enricher's search() hook in order and
// aggregates the results, per-plugin errors, and file-type skips.
//
// It checks the request context for cancellation before invoking each plugin
// and stops early when the client has gone away, since there is no point
// fanning out to the remaining plugins (and their outbound API calls) for a
// search the client has already superseded. A per-plugin failure that is
// itself caused by client cancellation (context.Canceled) is dropped rather
// than reported as a plugin failure or logged as a warning; a genuine
// per-plugin timeout (context.DeadlineExceeded) is still reported.
func aggregateEnricherSearches[T enricherRuntime](
	ctx context.Context,
	runtimes []T,
	fileType string,
	runSearch func(context.Context, T) (*SearchResponse, error),
	disabledFields func(context.Context, T) []string,
	log logger.Logger,
) (results []EnrichSearchResult, pluginErrors []PluginSearchError, skippedPlugins []PluginSearchSkipped) {
	results = make([]EnrichSearchResult, 0)
	for _, rt := range runtimes {
		// Stop early if the client has cancelled the request. Continuing would
		// fan out to plugins (and their external APIs) for a superseded search.
		if ctx.Err() != nil {
			break
		}

		manifest := rt.Manifest()
		// Skip plugins that don't handle this file type.
		if fileType != "" {
			enricherCap := manifest.Capabilities.MetadataEnricher
			if enricherCap == nil {
				continue
			}
			handles := false
			for _, ft := range enricherCap.FileTypes {
				if ft == fileType {
					handles = true
					break
				}
			}
			if !handles {
				skippedPlugins = append(skippedPlugins, PluginSearchSkipped{
					PluginScope: rt.Scope(),
					PluginID:    rt.PluginID(),
					PluginName:  manifest.Name,
				})
				continue
			}
		}

		resp, sErr := runSearch(ctx, rt)
		if sErr != nil {
			// Don't report a per-plugin failure that's just the client
			// cancelling the request, since it's not a plugin malfunction, and
			// logging it would be noise. A genuine deadline timeout is a real
			// failure and is still reported.
			if errors.Is(sErr, context.Canceled) {
				continue
			}
			log.Warn("enricher search failed", logger.Data{
				"scope":  rt.Scope(),
				"plugin": rt.PluginID(),
				"error":  sErr.Error(),
			})
			pluginErrors = append(pluginErrors, PluginSearchError{
				PluginScope: rt.Scope(),
				PluginID:    rt.PluginID(),
				PluginName:  manifest.Name,
				Message:     sErr.Error(),
			})
			continue
		}
		if resp == nil {
			continue
		}

		df := disabledFields(ctx, rt)
		for _, md := range resp.Results {
			results = append(results, EnrichSearchResult{
				ParsedMetadata: md,
				PluginScope:    md.PluginScope,
				PluginID:       md.PluginID,
				DisabledFields: df,
			})
		}
	}
	return results, pluginErrors, skippedPlugins
}

// DownloadCoverFromURL fetches a cover image from a URL and populates md.CoverData and md.CoverMimeType.
// Returns true if the download succeeded, false otherwise. Skips if CoverData is already set (precedence rule).
// The URL's domain (and any redirect domains) must be in the plugin's httpAccess.domains allowlist.
func DownloadCoverFromURL(ctx context.Context, md *mediafile.ParsedMetadata, allowedDomains []string, log logger.Logger) bool {
	if len(md.CoverData) > 0 || md.CoverURL == "" {
		return false
	}

	// Validate the cover URL domain against the plugin's allowed domains
	parsedURL, err := url.Parse(md.CoverURL)
	if err != nil {
		log.Warn("failed to parse cover URL", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Warn("cover URL uses unsupported scheme", logger.Data{"url": md.CoverURL, "scheme": parsedURL.Scheme})
		return false
	}
	if err := validateDomain(parsedURL.Host, allowedDomains); err != nil {
		log.Warn("cover URL domain not in plugin's httpAccess.domains", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := validateDomain(req.URL.Host, allowedDomains); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, md.CoverURL, nil)
	if err != nil {
		log.Warn("failed to create cover download request", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn("failed to download cover from URL", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "image/") {
		log.Warn("cover URL returned non-image response", logger.Data{
			"url":          md.CoverURL,
			"status":       resp.StatusCode,
			"content_type": contentType,
		})
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB max
	if err != nil {
		log.Warn("failed to read cover response body", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}

	md.CoverData = body
	md.CoverMimeType = contentType
	return true
}
