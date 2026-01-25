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
// Proxies to Kobo store and injects custom image URL templates.
func (h *handler) handleInitialization(c echo.Context) error {
	koboPath := StripKoboPrefix(c.Request().URL.Path)
	targetURL := koboStoreBaseURL + koboPath

	proxyReq, err := http.NewRequestWithContext(c.Request().Context(), "GET", targetURL, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	// Forward headers
	for _, hdr := range []string{"Authorization", "User-Agent"} {
		if v := c.Request().Header.Get(hdr); v != "" {
			proxyReq.Header.Set(hdr, v)
		}
	}

	resp, err := koboStoreClient.Do(proxyReq)
	if err != nil {
		// Return minimal initialization response if Kobo store is unreachable
		return c.JSON(http.StatusOK, map[string]interface{}{
			"Resources": map[string]interface{}{},
		})
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"Resources": map[string]interface{}{},
		})
	}

	// Inject custom image URLs
	resources, ok := data["Resources"].(map[string]interface{})
	if !ok {
		resources = map[string]interface{}{}
		data["Resources"] = resources
	}

	baseURL := getBaseURL(c)
	resources["image_host"] = baseURL
	resources["image_url_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg"
	resources["image_url_quality_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg"

	return c.JSON(http.StatusOK, data)
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
// Returns changes since the last sync point.
func (h *handler) handleSync(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}
	scope := GetScopeFromContext(ctx)

	// Parse sync token
	var lastSyncPointID string
	if tokenHeader := c.Request().Header.Get("X-Kobo-SyncToken"); tokenHeader != "" {
		tokenBytes, err := base64.StdEncoding.DecodeString(tokenHeader)
		if err == nil {
			var token SyncToken
			if err := json.Unmarshal(tokenBytes, &token); err == nil {
				lastSyncPointID = token.LastSyncPointID
			}
		}
	}

	// Get current files in scope
	scopedFiles, err := h.service.GetScopedFiles(ctx, apiKey.UserID, scope)
	if err != nil {
		return errors.WithStack(err)
	}

	// Detect changes
	changes, err := h.service.DetectChanges(ctx, lastSyncPointID, scopedFiles)
	if err != nil {
		return errors.WithStack(err)
	}

	// Create new sync point
	sp, err := h.service.CreateSyncPoint(ctx, apiKey.ID, scopedFiles)
	if err != nil {
		return errors.WithStack(err)
	}

	// Cleanup old sync points (fire and forget)
	go func() {
		_ = h.service.CleanupOldSyncPoints(context.Background(), apiKey.ID)
	}()

	log.Info("kobo sync completed", logger.Data{
		"api_key_id": apiKey.ID,
		"scope":      scope.Type,
		"added":      len(changes.Added),
		"removed":    len(changes.Removed),
		"changed":    len(changes.Changed),
		"total":      len(scopedFiles),
	})

	// Build response
	baseURL := getBaseURL(c)
	response := buildSyncResponse(ctx, changes, baseURL, h.bookService)

	// Set new sync token
	newToken := SyncToken{LastSyncPointID: sp.ID}
	tokenJSON, _ := json.Marshal(newToken)
	c.Response().Header().Set("X-Kobo-SyncToken", base64.StdEncoding.EncodeToString(tokenJSON))

	return c.JSON(http.StatusOK, response)
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
	if file.CoverImagePath == nil || *file.CoverImagePath == "" {
		return errcodes.NotFound("Cover")
	}

	// Get the book to determine the cover path base dir
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Determine cover directory (same logic as eReader handler)
	var coverDir string
	if info, statErr := os.Stat(book.Filepath); statErr == nil && !info.IsDir() {
		coverDir = filepath.Dir(book.Filepath)
	} else {
		coverDir = book.Filepath
	}

	coverPath := filepath.Join(coverDir, *file.CoverImagePath)

	// Parse requested dimensions
	widthStr := c.Param("w")
	heightStr := c.Param("h")
	width, _ := strconv.Atoi(widthStr)
	height, _ := strconv.Atoi(heightStr)

	if width == 0 || height == 0 {
		// Serve original if dimensions not specified
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
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")
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
	metadata := buildBookMetadata(book, file, baseURL)

	// Kobo expects metadata wrapped in an array
	return c.JSON(http.StatusOK, []*BookMetadata{metadata})
}

// buildSyncResponse creates the array of sync change entries.
func buildSyncResponse(ctx context.Context, changes *SyncChanges, baseURL string, bookService *books.Service) []interface{} {
	// Initialize as empty slice (not nil) so JSON marshals to [] not null
	response := make([]interface{}, 0)

	// Added books
	for _, f := range changes.Added {
		entry := buildNewEntitlement(ctx, f, baseURL, bookService)
		if entry != nil {
			response = append(response, entry)
		}
	}

	// Changed books
	for _, f := range changes.Changed {
		entry := buildNewEntitlement(ctx, f, baseURL, bookService)
		if entry != nil {
			response = append(response, entry)
		}
	}

	// Removed books
	for _, f := range changes.Removed {
		response = append(response, &ChangedEntitlement{
			ChangedEntitlement: &EntitlementChangeWrapper{
				BookEntitlement: &BookEntitlementChange{
					ID:        ShishoID(f.FileID),
					IsRemoved: true,
				},
			},
		})
	}

	return response
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

	metadata := buildBookMetadata(book, file, baseURL)
	now := time.Now()

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
				RevisionID:          fmt.Sprintf("%s-%s", ShishoID(f.FileID), f.FileHash[:8]),
				Status:              "Active",
			},
			BookMetadata: metadata,
		},
	}
}

// Dummy UUID used for category/genre fields (same pattern as calibre-web).
const dummyCategoryID = "00000000-0000-0000-0000-000000000001"

// buildBookMetadata constructs the Kobo BookMetadata from our book/file.
func buildBookMetadata(book *models.Book, file *models.File, baseURL string) *BookMetadata {
	bookID := ShishoID(file.ID)

	metadata := &BookMetadata{
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
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
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
