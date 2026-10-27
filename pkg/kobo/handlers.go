package kobo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // Register PNG decoder
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
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
// Returns dummy auth tokens since Devices authenticate by the credential
// in the URL.
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
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	scope := device.Scope()

	inToken := decodeSyncToken(c.Request().Header.Get("X-Kobo-SyncToken"))

	ongoing, prevID, isContinuation, err := h.resolveSyncPoint(ctx, device, user, inToken)
	if err != nil {
		return err
	}

	// Diff the (possibly snapshotted) current state against the previous
	// completed sync point. For continuations, we re-derive ScopedFiles from
	// the frozen snapshot so paging stays consistent across calls.
	currentFiles := ScopedFilesFromSnapshot(ongoing.Books)
	changes, err := h.service.DetectChanges(ctx, device.ID, prevID, currentFiles)
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
		if err := h.service.MarkSyncPointCompleted(ctx, device.ID, ongoing.ID); err != nil {
			return errors.WithStack(err)
		}
		outToken = SyncToken{LastSyncPointID: ongoing.ID}

		// Cleanup older completed sync points (fire and forget). Run on a
		// detached context but keep the request logger so a hung/failed cleanup
		// is observable instead of silently swallowed.
		cleanupLog := log
		go func() {
			if err := h.service.CleanupOldSyncPoints(context.Background(), device.ID); err != nil {
				cleanupLog.Warn("kobo cleanup old sync points failed", logger.Data{
					"device_id": device.ID,
					"error":     err.Error(),
				})
			}
		}()
	}

	tokenJSON, _ := json.Marshal(outToken)
	c.Response().Header().Set("X-Kobo-SyncToken", base64.StdEncoding.EncodeToString(tokenJSON))

	log.Info("kobo sync page emitted", logger.Data{
		"device_id":      device.ID,
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
	device *devices.Device,
	user *models.User,
	inToken SyncToken,
) (*SyncPoint, string, bool, error) {
	if inToken.OngoingSyncPointID != "" {
		// GetSyncPointByID enforces the Device at the SQL layer, so a token
		// bearing another tenant's sync-point UUID won't match.
		sp, err := h.service.GetSyncPointByID(ctx, device.ID, inToken.OngoingSyncPointID)
		// Only honor the ongoing point if it still exists and has not been
		// completed by another concurrent caller (which would mean re-emitting
		// changes we've already sent).
		if err == nil && sp.CompletedAt == nil {
			return sp, inToken.PrevSyncPointID, true, nil
		}
	}

	// Fresh sync: snapshot current state.
	scopedFiles, err := h.service.GetScopedFiles(ctx, user, device)
	if err != nil {
		return nil, "", false, errors.WithStack(err)
	}
	sp, err := h.service.CreateSyncPoint(ctx, device.ID, scopedFiles)
	if err != nil {
		return nil, "", false, errors.WithStack(err)
	}
	return sp, inToken.LastSyncPointID, false, nil
}

// requireEntry returns the Device's entry entryID (from parseEntryID). It
// returns a 404 for an entry the Device no longer delivers within its scope,
// the same response as one that does not exist, so a Device cannot probe or
// fetch files it does not sync. Kobo routes never return 403 for this.
func (h *handler) requireEntry(c echo.Context, entryID string) (*SyncEntry, error) {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return nil, err
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	entry, err := h.service.EntryInScope(c.Request().Context(), user, device, entryID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if entry == nil {
		return nil, errcodes.NotFound("File")
	}
	return entry, nil
}

// handleDownload handles GET /v1/books/:bookId/file/epub, which serves the
// entry's File delivered as the entry's File Type. A Conversion that fails,
// now or partway through an earlier attempt, is a 500, never the Original
// File, which is a File Type the Device's rules did not choose (ADR 0011).
func (h *handler) handleDownload(c echo.Context) error {
	ctx := c.Request().Context()
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.requireEntry(c, entryID)
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: entry.FileID})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := books.RequireFileOnDisk(c, file, "File"); err != nil {
		return err
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

	download, err := books.ResolveDeliveryDownload(c, h.downloadCache, book, fileWithRelations, entry.DeliveredFileType)
	if err != nil {
		return err
	}
	return download.Serve(c, httputil.WithContentType("application/octet-stream"))
}

// handleCover handles GET /v1/books/:imageId/thumbnail/:w/:h/*.
// Serves resized cover images.
func (h *handler) handleCover(c echo.Context) error {
	ctx := c.Request().Context()
	entryID, ok := parseEntryID(c.Param("imageId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.requireEntry(c, entryID)
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: entry.FileID})
	if err != nil {
		return errors.WithStack(err)
	}
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return errcodes.NotFound("Cover")
	}

	// Resolve via the file's parent dir, since book.Filepath may be a
	// synthetic organized-folder path that doesn't exist on disk for
	// root-level files.
	coverPath := covers.FileCoverPath(file)

	// Stat the cover for the conditional GET before the resize, so a
	// revalidated request skips the decode, resize, and encode work.
	coverStat, err := os.Stat(coverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}
	modTime := coverStat.ModTime().UTC().Truncate(time.Second)
	// The image id Shisho syncs carries the cover's cache key after the
	// entry id (buildBookMetadata), so its URL changes with the cover and the
	// Kobo may keep it; a no-cache cover is fetched again on every page of
	// the Kobo's library. A bare entry id names no version, so it
	// revalidates.
	cacheControl := covers.CacheControlNoCache
	if c.Param("imageId") != entryID {
		cacheControl = covers.CacheControlImmutable
	}
	// The cache headers go only on a response that carries the cover (or a
	// 304 for it), never on an error.
	setCoverHeaders := func() {
		c.Response().Header().Set("Cache-Control", cacheControl)
		c.Response().Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
	}
	if ims := c.Request().Header.Get("If-Modified-Since"); ims != "" {
		if t, parseErr := http.ParseTime(ims); parseErr == nil && !modTime.After(t) {
			setCoverHeaders()
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
		// Serve the original if dimensions are not specified. ServeContent
		// sends Last-Modified from the same mtime.
		return httputil.ServeFile(c, coverPath, errcodes.NotFound("Cover"),
			httputil.WithCacheControl(cacheControl))
	}

	// Open and resize the image
	imgFile, err := os.Open(coverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
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

	// Encoded in full first, so the response carries its length, as the
	// Kobo store's images do.
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, dstImg, &jpeg.Options{Quality: 80}); err != nil {
		return errors.WithStack(err)
	}
	setCoverHeaders()
	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Content-Length", strconv.Itoa(encoded.Len()))
	c.Response().WriteHeader(http.StatusOK)
	_, err = c.Response().Write(encoded.Bytes())
	return errors.WithStack(err)
}

// handleMetadata handles GET /v1/library/:bookId/metadata.
// Returns metadata for Shisho books, proxies for unknown books.
func (h *handler) handleMetadata(c echo.Context) error {
	ctx := c.Request().Context()
	entryID, ok := parseEntryID(c.Param("bookId"))
	if !ok {
		return proxyToKoboStore(c)
	}
	entry, err := h.requireEntry(c, entryID)
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: entry.FileID})
	if err != nil {
		return errors.WithStack(err)
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	baseURL := getBaseURL(c)
	cacheKey := coverCacheKey(computeMetadataHash(book, file), coverModifiedAt(ctx, file))
	metadata := buildBookMetadata(book, file, entry.EntryID, entry.DeliveredFileType, cacheKey, baseURL)

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
// ordered list. The order (Added, then Changed, then Removed, each by entry
// id) must be stable across calls so a paginated cursor remains valid.
//
// Input slices are cloned before sorting so callers that retain a reference
// to the source SyncChanges don't observe an ordering side effect.
func combineChanges(c *SyncChanges) []changeEntry {
	sortedClone := func(in []ScopedFile) []ScopedFile {
		out := make([]ScopedFile, len(in))
		copy(out, in)
		sort.Slice(out, func(i, j int) bool { return out[i].EntryID < out[j].EntryID })
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
// JSON array Kobo expects, each under its entry's id and RevisionId. nil
// result for missing books (e.g. deleted between snapshot and emit) is
// silently dropped, as is an entry with no recorded RevisionId, which only a
// snapshot stored before entries had ids can hold.
func buildSyncResponseFromEntries(ctx context.Context, entries []changeEntry, baseURL string, bookService *books.Service) []interface{} {
	response := make([]interface{}, 0, len(entries))
	for _, e := range entries {
		if e.Kind != changeRemoved && e.File.RevisionID == "" {
			continue
		}
		switch e.Kind {
		case changeAdded:
			entry := buildEntitlement(ctx, e.File, baseURL, bookService)
			if entry != nil {
				response = append(response, &NewEntitlement{NewEntitlement: entry})
			}
		case changeChanged:
			entry := buildEntitlement(ctx, e.File, baseURL, bookService)
			if entry != nil {
				response = append(response, &ChangedEntitlement{ChangedEntitlement: entry})
			}
		case changeRemoved:
			response = append(response, &RemovedEntitlement{
				ChangedEntitlement: &EntitlementChangeWrapper{
					BookEntitlement: &BookEntitlementChange{
						ID:        e.File.EntryID,
						IsRemoved: true,
					},
					BookMetadata: buildRemovedBookMetadata(e.File.EntryID),
				},
			})
		}
	}
	return response
}

// buildRemovedBookMetadata returns a stub BookMetadata for a removed entry.
// The book record may already be gone, so we synthesize the required
// GUID-shaped fields from the entry id. Kobo firmware requires a metadata
// object alongside the IsRemoved entitlement to deindex the book on-device.
func buildRemovedBookMetadata(entryID string) *BookMetadata {
	return &BookMetadata{
		Categories:      []string{dummyCategoryID},
		CoverImageID:    entryID,
		CrossRevisionID: entryID,
		CurrentDisplayPrice: &DisplayPrice{
			CurrencyCode: "USD",
			TotalAmount:  0,
		},
		EntitlementID: entryID,
		Genre:         dummyCategoryID,
		Language:      "en",
		RevisionID:    entryID,
		WorkID:        entryID,
	}
}

// buildEntitlement creates the entitlement and metadata for a file, sent
// under its entry's id and RevisionId. The same body goes out for an added
// book and a changed one; only the wrapper differs.
func buildEntitlement(ctx context.Context, f ScopedFile, baseURL string, bookService *books.Service) *EntitlementWrapper {
	file, err := bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &f.FileID})
	if err != nil {
		return nil
	}

	book, err := bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return nil
	}

	metadata := buildBookMetadata(book, file, f.EntryID, f.DeliveredFileType, coverCacheKey(f.MetadataHash, f.CoverModifiedAt), baseURL)
	now := time.Now()

	return &EntitlementWrapper{
		BookEntitlement: &BookEntitlement{
			Accessibility:       "Full",
			ActivePeriod:        &ActivePeriod{From: book.CreatedAt},
			Created:             book.CreatedAt,
			CrossRevisionID:     f.EntryID,
			ID:                  f.EntryID,
			IsHiddenFromArchive: false,
			IsLocked:            false,
			IsRemoved:           false,
			LastModified:        now,
			OriginCategory:      "Imported",
			RevisionID:          f.RevisionID,
			Status:              "Active",
		},
		BookMetadata: metadata,
	}
}

// Dummy UUID used for category/genre fields (same pattern as calibre-web).
const dummyCategoryID = "00000000-0000-0000-0000-000000000001"

// buildBookMetadata constructs the Kobo BookMetadata for the entry entryID,
// which holds file delivered as deliveredType.
//
// coverCacheKey is appended to CoverImageID so the device's thumbnail cache
// refreshes whenever the underlying book/cover changes (otherwise the device
// reuses the cached image at the entry id indefinitely, even after the entry
// is handed to another File). Pass an empty string to disable suffixing,
// which is only safe when the caller has confirmed no device-cache risk
// (e.g. unit tests).
func buildBookMetadata(book *models.Book, file *models.File, entryID, deliveredType, coverCacheKey, baseURL string) *BookMetadata {
	bookID := entryID
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
		Title:         file.ResolveTitle(book.Title),
		WorkID:        bookID,
		DownloadUrls: []*DownloadURL{
			{
				Format:   downloadFormat(file.FileType, deliveredType),
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

// downloadFormat is the format a Kobo is told a File delivered as
// deliveredType has: KEPUB, or EPUB3 for an EPUB, or EPUB3FL for one with a
// fixed layout, as a CBZ becomes. A CBZ is the only fixed-layout source
// Shisho knows of; an EPUB's own layout is not read. A Kobo never offers a
// book reported as plain EPUB (ADR 0012).
func downloadFormat(sourceType, deliveredType string) string {
	switch {
	case deliveredType == models.FileTypeKePub:
		return "KEPUB"
	case sourceType == models.FileTypeCBZ:
		return "EPUB3FL"
	}
	return "EPUB3"
}

// getBaseURL returns the full base URL of the Kobo route the request came
// through, so download and cover URLs work however the Kobo was set up.
func getBaseURL(c echo.Context) string {
	return httputil.PublicOrigin(c.Request()) + basePath(c.Request().URL.Path)
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
