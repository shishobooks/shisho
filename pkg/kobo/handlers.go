package kobo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // Register PNG decoder
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
	"golang.org/x/image/draw"
)

type handler struct {
	service       *Service
	bookService   *books.Service
	downloadCache *downloadcache.Cache
}

func newHandler(service *Service, bookService *books.Service, downloadCache *downloadcache.Cache) *handler {
	return &handler{
		service:       service,
		bookService:   bookService,
		downloadCache: downloadCache,
	}
}

// handleInitialization handles GET /v1/initialization.
//
// We deliberately do NOT proxy this request to the real Kobo store. Without a
// valid Kobo OAuth token (we only ever send our own dummy token), the proxied
// response either fails outright or returns a body that includes firmware
// update prompts and other URLs the device follows and aborts on. Instead we
// return a snapshot of the upstream Resources map (sourced verbatim from
// Komga's nativeKoboResources fallback) with the three image URL keys
// overridden to point back at our cover handler. This keeps the device's
// store-related operations pointing at Kobo (so wishlist/store browsing isn't
// broken in stranger ways) while ensuring covers and sync URLs resolve to us.
func (h *handler) handleInitialization(c echo.Context) error {
	// Stub api token header expected by some Kobo firmware versions on
	// initialization (base64 of "{}").
	c.Response().Header().Set("x-kobo-apitoken", "e30=")

	resources, err := buildInitResources(getBaseURL(c))
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"Resources": resources})
}

// handleAuth handles POST /v1/auth/device.
// Returns dummy auth tokens since we use API key auth.
func (h *handler) handleAuth(c echo.Context) error {
	return c.JSON(http.StatusOK, DeviceAuthResponse{
		AccessToken:  "shisho-access-token",
		RefreshToken: "shisho-refresh-token",
		TokenType:    "Bearer",
		TrackingID:   "shisho-tracking",
		UserKey:      "shisho-user",
	})
}

// handleSync handles GET /v1/library/sync.
//
// Returns changes since the last sync point, paginated. The first call snapshots
// current library state into a new in-progress sync point and emits the first
// page of the diff against the previous completed sync point. Subsequent calls
// (signalled by the OngoingSyncPointID/Cursor in the inbound token) continue
// from where the prior page left off, diffing against the frozen snapshot.
// When the final page is emitted, the sync point is marked complete and stale
// snapshots are cleaned up. While more pages remain, X-Kobo-Sync: continue is
// set to tell the device to immediately fetch the next page.
func (h *handler) handleSync(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}
	scope := GetScopeFromContext(ctx)

	inToken := decodeSyncToken(c.Request().Header.Get("X-Kobo-SyncToken"))

	ongoing, prevID, isContinuation, err := h.resolveSyncPoint(ctx, apiKey.ID, apiKey.UserID, scope, inToken)
	if err != nil {
		return err
	}

	// Diff the (possibly snapshotted) current state against the previous
	// completed sync point. For continuations, we re-derive ScopedFiles from
	// the frozen snapshot so paging stays consistent across calls.
	currentFiles := ScopedFilesFromSnapshot(ongoing.Books)
	changes, err := h.service.DetectChanges(ctx, apiKey.ID, prevID, currentFiles)
	if err != nil {
		return errors.WithStack(err)
	}

	allEntries := combineChanges(changes)
	start := inToken.Cursor
	if start < 0 || start > len(allEntries) {
		start = 0
	}
	end := start + syncItemLimit
	hasMore := end < len(allEntries)
	if !hasMore {
		end = len(allEntries)
	}
	page := allEntries[start:end]

	baseURL := getBaseURL(c)
	response := buildSyncResponseFromEntries(ctx, page, baseURL, h.bookService)

	// Token + completion bookkeeping.
	var outToken SyncToken
	if hasMore {
		c.Response().Header().Set("X-Kobo-Sync", "continue")
		outToken = SyncToken{
			LastSyncPointID:    inToken.LastSyncPointID,
			OngoingSyncPointID: ongoing.ID,
			PrevSyncPointID:    prevID,
			Cursor:             end,
		}
	} else {
		if err := h.service.MarkSyncPointCompleted(ctx, apiKey.ID, ongoing.ID); err != nil {
			return errors.WithStack(err)
		}
		outToken = SyncToken{LastSyncPointID: ongoing.ID}

		// Cleanup older completed sync points (fire and forget). Run on a
		// detached context but keep the request logger so a hung/failed cleanup
		// is observable instead of silently swallowed.
		cleanupLog := log
		go func() {
			if err := h.service.CleanupOldSyncPoints(context.Background(), apiKey.ID); err != nil {
				cleanupLog.Warn("kobo cleanup old sync points failed", logger.Data{
					"api_key_id": apiKey.ID,
					"error":      err.Error(),
				})
			}
		}()
	}

	tokenJSON, _ := json.Marshal(outToken)
	c.Response().Header().Set("X-Kobo-SyncToken", base64.StdEncoding.EncodeToString(tokenJSON))

	log.Info("kobo sync page emitted", logger.Data{
		"api_key_id":     apiKey.ID,
		"scope":          scope.Type,
		"continuation":   isContinuation,
		"page_start":     start,
		"page_end":       end,
		"total_entries":  len(allEntries),
		"has_more":       hasMore,
		"sync_point_id":  ongoing.ID,
		"prev_sync_id":   prevID,
		"snapshot_files": len(currentFiles),
	})

	return c.JSON(http.StatusOK, response)
}

// decodeSyncToken parses the base64-encoded JSON token from the X-Kobo-SyncToken
// header. Malformed or empty tokens silently produce a zero-value token, which
// triggers a fresh sync.
func decodeSyncToken(header string) SyncToken {
	var token SyncToken
	if header == "" {
		return token
	}
	tokenBytes, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return SyncToken{}
	}
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return SyncToken{}
	}
	return token
}

// resolveSyncPoint returns the sync point we should page out, the prev sync
// point ID to diff against, and whether this is a continuation.
//
//   - If the inbound token names an in-progress ongoing point that still exists,
//     we reuse it (continuation).
//   - Otherwise we snapshot current scoped files into a new in-progress point
//     and use the inbound LastSyncPointID as the prev baseline.
func (h *handler) resolveSyncPoint(
	ctx context.Context,
	apiKeyID string,
	userID int,
	scope *SyncScope,
	inToken SyncToken,
) (*SyncPoint, string, bool, error) {
	if inToken.OngoingSyncPointID != "" {
		// GetSyncPointByID enforces apiKeyID at the SQL layer, so a token
		// bearing another tenant's sync-point UUID won't match.
		sp, err := h.service.GetSyncPointByID(ctx, apiKeyID, inToken.OngoingSyncPointID)
		// Only honor the ongoing point if it still exists and has not been
		// completed by another concurrent caller (which would mean re-emitting
		// changes we've already sent).
		if err == nil && sp.CompletedAt == nil {
			return sp, inToken.PrevSyncPointID, true, nil
		}
	}

	// Fresh sync: snapshot current state.
	scopedFiles, err := h.service.GetScopedFiles(ctx, userID, scope)
	if err != nil {
		return nil, "", false, errors.WithStack(err)
	}
	sp, err := h.service.CreateSyncPoint(ctx, apiKeyID, scopedFiles)
	if err != nil {
		return nil, "", false, errors.WithStack(err)
	}
	return sp, inToken.LastSyncPointID, false, nil
}

// handleDownload handles GET /v1/books/:bookId/file/epub.
// Serves files as KePub.
func (h *handler) handleDownload(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	bookID := c.Param("bookId")

	fileID, ok := ParseShishoID(bookID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the file with relations from the book's files
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file
	}

	// Generate KePub
	cachedPath, _, err := h.downloadCache.GetOrGenerateKepub(ctx, book, fileWithRelations)
	if err != nil {
		if errors.Is(err, filegen.ErrKepubNotSupported) {
			log.Warn("kepub not supported for file, serving original", logger.Data{"file_id": fileID})
			return serveFileWithHeaders(c, file.Filepath, book.Title+".epub")
		}
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			log.Warn("kepub generation failed, serving original", logger.Data{"file_id": fileID, "error": genErr.Message})
			return serveFileWithHeaders(c, file.Filepath, book.Title+".epub")
		}
		return errors.WithStack(err)
	}

	return serveFileWithHeaders(c, cachedPath, book.Title+".kepub.epub")
}

// handleCover handles GET /v1/books/:imageId/thumbnail/:w/:h/*.
// Serves resized cover images.
func (h *handler) handleCover(c echo.Context) error {
	ctx := c.Request().Context()
	imageID := c.Param("imageId")

	fileID, ok := ParseShishoID(imageID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return errcodes.NotFound("Cover")
	}

	// Resolve via the file's parent dir — book.Filepath may be a synthetic
	// organized-folder path that doesn't exist on disk for root-level files.
	coverPath := filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)

	// Stat source cover for Last-Modified + conditional GET short-circuit.
	// This runs before the resize so revalidated requests skip the expensive
	// decode/resize/encode work. In the no-dimensions branch below, c.File
	// also sets Last-Modified from the same file mtime — the values match
	// (second precision), so the overlap is harmless.
	coverStat, err := os.Stat(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	modTime := coverStat.ModTime().UTC().Truncate(time.Second)
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	c.Response().Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
	if ims := c.Request().Header.Get("If-Modified-Since"); ims != "" {
		if t, parseErr := http.ParseTime(ims); parseErr == nil && !modTime.After(t) {
			c.Response().WriteHeader(http.StatusNotModified)
			return nil
		}
	}

	// Parse requested dimensions
	widthStr := c.Param("w")
	heightStr := c.Param("h")
	width, _ := strconv.Atoi(widthStr)
	height, _ := strconv.Atoi(heightStr)

	if width == 0 || height == 0 {
		// Serve original if dimensions not specified. c.File handles
		// Last-Modified/If-Modified-Since automatically for this branch.
		return c.File(coverPath)
	}

	// Open and resize the image
	imgFile, err := os.Open(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	defer imgFile.Close()

	srcImg, _, err := image.Decode(imgFile)
	if err != nil {
		return errors.WithStack(err)
	}

	// Resize maintaining aspect ratio, fitting within requested dimensions
	srcBounds := srcImg.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// Calculate target dimensions maintaining aspect ratio
	targetW, targetH := fitDimensions(srcW, srcH, width, height)

	dstImg := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.BiLinear.Scale(dstImg, dstImg.Bounds(), srcImg, srcBounds, draw.Over, nil)

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().WriteHeader(http.StatusOK)
	return jpeg.Encode(c.Response().Writer, dstImg, &jpeg.Options{Quality: 80})
}

// handleMetadata handles GET /v1/library/:bookId/metadata.
// Returns metadata for Shisho books, proxies for unknown books.
func (h *handler) handleMetadata(c echo.Context) error {
	ctx := c.Request().Context()
	bookID := c.Param("bookId")

	fileID, ok := ParseShishoID(bookID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	baseURL := getBaseURL(c)
	var coverFilename string
	if file.CoverImageFilename != nil {
		coverFilename = *file.CoverImageFilename
	}
	coverCacheKey := ComputeMetadataHashFromBook(book, coverFilename)
	if len(coverCacheKey) > 8 {
		coverCacheKey = coverCacheKey[:8]
	}
	metadata := buildBookMetadata(book, file, coverCacheKey, baseURL)

	// Kobo expects metadata wrapped in an array
	return c.JSON(http.StatusOK, []*BookMetadata{metadata})
}

// changeKind tags the type of a single sync entry so the page builder can
// emit the right wrapper without a second pass.
type changeKind int

const (
	changeAdded changeKind = iota
	changeChanged
	changeRemoved
)

// changeEntry is one item in the deterministically-ordered combined diff list
// that handleSync slices for pagination.
type changeEntry struct {
	File ScopedFile
	Kind changeKind
}

// combineChanges flattens Added/Changed/Removed into a single deterministically
// ordered list. The order — Added (by FileID asc), Changed (by FileID asc),
// Removed (by FileID asc) — must be stable across calls so a paginated cursor
// remains valid.
//
// Input slices are cloned before sorting so callers that retain a reference
// to the source SyncChanges don't observe an ordering side effect.
func combineChanges(c *SyncChanges) []changeEntry {
	sortedClone := func(in []ScopedFile) []ScopedFile {
		out := make([]ScopedFile, len(in))
		copy(out, in)
		sort.Slice(out, func(i, j int) bool { return out[i].FileID < out[j].FileID })
		return out
	}
	added := sortedClone(c.Added)
	changed := sortedClone(c.Changed)
	removed := sortedClone(c.Removed)

	out := make([]changeEntry, 0, len(added)+len(changed)+len(removed))
	for _, f := range added {
		out = append(out, changeEntry{File: f, Kind: changeAdded})
	}
	for _, f := range changed {
		out = append(out, changeEntry{File: f, Kind: changeChanged})
	}
	for _, f := range removed {
		out = append(out, changeEntry{File: f, Kind: changeRemoved})
	}
	return out
}

// buildSyncResponseFromEntries materializes a slice of changeEntry into the
// JSON array Kobo expects. nil result for missing books (e.g. deleted between
// snapshot and emit) is silently dropped.
func buildSyncResponseFromEntries(ctx context.Context, entries []changeEntry, baseURL string, bookService *books.Service) []interface{} {
	response := make([]interface{}, 0, len(entries))
	for _, e := range entries {
		switch e.Kind {
		case changeAdded, changeChanged:
			entry := buildNewEntitlement(ctx, e.File, baseURL, bookService)
			if entry != nil {
				response = append(response, entry)
			}
		case changeRemoved:
			response = append(response, &ChangedEntitlement{
				ChangedEntitlement: &EntitlementChangeWrapper{
					BookEntitlement: &BookEntitlementChange{
						ID:        ShishoID(e.File.FileID),
						IsRemoved: true,
					},
					BookMetadata: buildRemovedBookMetadata(e.File.FileID),
				},
			})
		}
	}
	return response
}

// buildRemovedBookMetadata returns a stub BookMetadata for a removed book.
// The book record may already be gone, so we synthesize the required GUID-shaped
// fields from the file ID. Kobo firmware requires a metadata object alongside
// the IsRemoved entitlement to deindex the book on-device.
func buildRemovedBookMetadata(fileID int) *BookMetadata {
	bookID := ShishoID(fileID)
	return &BookMetadata{
		Categories:      []string{dummyCategoryID},
		CoverImageID:    bookID,
		CrossRevisionID: bookID,
		CurrentDisplayPrice: &DisplayPrice{
			CurrencyCode: "USD",
			TotalAmount:  0,
		},
		EntitlementID: bookID,
		Genre:         dummyCategoryID,
		Language:      "en",
		RevisionID:    bookID,
		WorkID:        bookID,
	}
}

// buildNewEntitlement creates a NewEntitlement sync entry for a file.
func buildNewEntitlement(ctx context.Context, f ScopedFile, baseURL string, bookService *books.Service) *NewEntitlement {
	file, err := bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &f.FileID})
	if err != nil {
		return nil
	}

	book, err := bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return nil
	}

	// MetadataHash already captures title + author + cover filename. Use a
	// short prefix as the device-cache-busting suffix so the device refreshes
	// thumbnails when any of those change.
	coverCacheKey := ""
	if len(f.MetadataHash) >= 8 {
		coverCacheKey = f.MetadataHash[:8]
	}
	metadata := buildBookMetadata(book, file, coverCacheKey, baseURL)
	now := time.Now()

	// Mirror the coverCacheKey guard above — computeFileHash returns 16 chars
	// today, but that's not statically enforced and a future hash change
	// shouldn't be able to panic the sync loop.
	revisionSuffix := f.FileHash
	if len(revisionSuffix) >= 8 {
		revisionSuffix = revisionSuffix[:8]
	}

	return &NewEntitlement{
		NewEntitlement: &EntitlementWrapper{
			BookEntitlement: &BookEntitlement{
				Accessibility:       "Full",
				ActivePeriod:        &ActivePeriod{From: book.CreatedAt},
				Created:             book.CreatedAt,
				CrossRevisionID:     ShishoID(f.FileID),
				ID:                  ShishoID(f.FileID),
				IsHiddenFromArchive: false,
				IsLocked:            false,
				IsRemoved:           false,
				LastModified:        now,
				OriginCategory:      "Imported",
				RevisionID:          fmt.Sprintf("%s-%s", ShishoID(f.FileID), revisionSuffix),
				Status:              "Active",
			},
			BookMetadata: metadata,
		},
	}
}

// Dummy UUID used for category/genre fields (same pattern as calibre-web).
const dummyCategoryID = "00000000-0000-0000-0000-000000000001"

// buildBookMetadata constructs the Kobo BookMetadata from our book/file.
//
// coverCacheKey is appended to CoverImageID so the device's thumbnail cache
// refreshes whenever the underlying book/cover changes (otherwise the device
// reuses the cached image at "shisho-{fileID}" indefinitely, even if the
// underlying file ID gets remapped to a different book on a rescan). Pass an
// empty string to disable suffixing — only safe when the caller has confirmed
// no device-cache risk (e.g. unit tests).
func buildBookMetadata(book *models.Book, file *models.File, coverCacheKey string, baseURL string) *BookMetadata {
	bookID := ShishoID(file.ID)
	coverImageID := bookID
	if coverCacheKey != "" {
		coverImageID = bookID + "-" + coverCacheKey
	}

	language := "en"
	if file.Language != nil && *file.Language != "" {
		language = *file.Language
	}
	metadata := &BookMetadata{
		Categories:      []string{dummyCategoryID},
		CoverImageID:    coverImageID,
		CrossRevisionID: bookID,
		CurrentDisplayPrice: &DisplayPrice{
			CurrencyCode: "USD",
			TotalAmount:  0,
		},
		EntitlementID: bookID,
		Genre:         dummyCategoryID,
		Language:      language,
		RevisionID:    bookID,
		Title:         book.Title,
		WorkID:        bookID,
		DownloadUrls: []*DownloadURL{
			{
				Format:   "KEPUB",
				Platform: "Generic",
				Size:     file.FilesizeBytes,
				URL:      fmt.Sprintf("%s/v1/books/%s/file/epub", baseURL, bookID),
			},
		},
	}

	// Authors - populate both ContributorRoles and Contributors
	if book.Authors != nil {
		for _, a := range book.Authors {
			if a.Person != nil {
				metadata.ContributorRoles = append(metadata.ContributorRoles, &ContributorRole{
					Name: a.Person.Name,
				})
				metadata.Contributors = append(metadata.Contributors, a.Person.Name)
			}
		}
	}

	// Description
	if book.Description != nil {
		metadata.Description = *book.Description
	}

	// Subtitle
	if book.Subtitle != nil && *book.Subtitle != "" {
		metadata.SubTitle = *book.Subtitle
	}

	// Series (NOTE: field is SeriesNumber *float64, not Number)
	if len(book.BookSeries) > 0 && book.BookSeries[0].Series != nil && book.BookSeries[0].SeriesNumber != nil {
		metadata.Series = &Series{
			Name:        book.BookSeries[0].Series.Name,
			Number:      *book.BookSeries[0].SeriesNumber,
			NumberFloat: *book.BookSeries[0].SeriesNumber,
		}
	}

	// Publisher
	if file.Publisher != nil {
		metadata.Publisher = &Publisher{Name: file.Publisher.Name}
	}

	// Publication date
	if file.ReleaseDate != nil {
		metadata.PublicationDate = file.ReleaseDate.Format("2006-01-02")
	}

	return metadata
}

// getBaseURL returns the full base URL for the current Kobo scope.
// Uses scope from context since routes don't have a :scopeType param.
func getBaseURL(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	// Use X-Forwarded-Host if available (set by proxy), otherwise use Host header
	host := c.Request().Header.Get("X-Forwarded-Host")
	if host == "" {
		host = c.Request().Host
	}

	// If host doesn't include a port and X-Forwarded-Port is set, append it
	// This handles clients (like Kobo) that don't include port in Host header
	if !strings.Contains(host, ":") {
		if port := c.Request().Header.Get("X-Forwarded-Port"); port != "" && port != "80" && port != "443" {
			host = host + ":" + port
		}
	}

	apiKey := c.Param("apiKey")
	scope := GetScopeFromContext(c.Request().Context())

	basePath := fmt.Sprintf("/kobo/%s/%s", apiKey, scope.Type)
	if scope.Type == "library" && scope.LibraryID != nil {
		basePath += fmt.Sprintf("/%d", *scope.LibraryID)
	} else if scope.Type == "list" && scope.ListID != nil {
		basePath += fmt.Sprintf("/%d", *scope.ListID)
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, basePath)
}

// serveFileWithHeaders serves a file with proper Content-Type and Content-Disposition headers.
func serveFileWithHeaders(c echo.Context, filepath, filename string) error {
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	httputil.SetAttachmentFilename(c.Response(), filename)
	return c.File(filepath)
}

// fitDimensions calculates target dimensions maintaining aspect ratio.
func fitDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}

	ratioW := float64(maxW) / float64(srcW)
	ratioH := float64(maxH) / float64(srcH)

	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}

	return int(float64(srcW) * ratio), int(float64(srcH) * ratio)
}
