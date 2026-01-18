# CBZ Viewer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a web-based CBZ viewer with page-by-page reading, image preloading, keyboard navigation, persistent user settings, and chapter-aware progress tracking.

**Architecture:** Backend adds page-serving endpoint with on-demand extraction and caching, plus a new settings package for viewer preferences. Frontend creates a full-screen reader with preloading, URL-synced navigation, and a chapter-aware progress bar. The viewer is accessed from the book detail page.

**Tech Stack:** Go/Echo/Bun (backend), React/TypeScript/TailwindCSS/Tanstack Query (frontend), SQLite (database)

---

## Task 1: Database Migration for User Settings

**Files:**
- Create: `pkg/migrations/20260116100000_add_user_settings.go`
- Reference: `pkg/migrations/20260116000000_add_chapters.go` (pattern)

**Step 1: Create the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE user_settings (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
				viewer_preload_count INTEGER NOT NULL DEFAULT 3,
				viewer_fit_mode TEXT NOT NULL DEFAULT 'fit-height'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for fast lookup by user
		_, err = db.Exec(`CREATE INDEX ix_user_settings_user_id ON user_settings(user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS user_settings")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration to verify it applies**

Run: `make db:migrate`
Expected: Migration runs without error

**Step 3: Verify rollback works**

Run: `make db:rollback && make db:migrate`
Expected: Rollback succeeds, then migration re-applies

**Step 4: Commit**

```bash
git add pkg/migrations/20260116100000_add_user_settings.go
git commit -m "feat: add user_settings table migration"
```

---

## Task 2: User Settings Model

**Files:**
- Create: `pkg/models/user_settings.go`
- Reference: `pkg/models/file.go`, `pkg/models/chapter.go` (patterns)

**Step 1: Create the model file**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FitMode = typeof FitModeHeight | typeof FitModeOriginal;
	FitModeHeight   = "fit-height"
	FitModeOriginal = "original"
)

type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings,alias:us" tstype:"-"`

	ID                 int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID             int       `bun:",notnull,unique" json:"user_id"`
	ViewerPreloadCount int       `bun:",notnull,default:3" json:"viewer_preload_count"`
	ViewerFitMode      string    `bun:",notnull,default:'fit-height'" json:"viewer_fit_mode" tstype:"FitMode"`
}

// DefaultUserSettings returns a UserSettings with default values.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		ViewerPreloadCount: 3,
		ViewerFitMode:      FitModeHeight,
	}
}
```

**Step 2: Generate TypeScript types**

Run: `make tygo`
Expected: Types generated (may say "Nothing to be done" if already up-to-date)

**Step 3: Commit**

```bash
git add pkg/models/user_settings.go
git commit -m "feat: add UserSettings model"
```

---

## Task 3: Settings Service

**Files:**
- Create: `pkg/settings/service.go`
- Reference: `pkg/chapters/service.go` (pattern)

**Step 1: Create the service file**

```go
package settings

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// GetViewerSettings retrieves viewer settings for a user, returning defaults if none exist.
func (svc *Service) GetViewerSettings(ctx context.Context, userID int) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	err := svc.db.NewSelect().
		Model(settings).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return defaults if no settings exist
			defaults := models.DefaultUserSettings()
			defaults.UserID = userID
			return defaults, nil
		}
		return nil, errors.WithStack(err)
	}

	return settings, nil
}

// UpdateViewerSettings updates viewer settings for a user, creating if not exists.
func (svc *Service) UpdateViewerSettings(ctx context.Context, userID int, preloadCount int, fitMode string) (*models.UserSettings, error) {
	now := time.Now()

	settings := &models.UserSettings{
		CreatedAt:          now,
		UpdatedAt:          now,
		UserID:             userID,
		ViewerPreloadCount: preloadCount,
		ViewerFitMode:      fitMode,
	}

	_, err := svc.db.NewInsert().
		Model(settings).
		On("CONFLICT (user_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("viewer_preload_count = EXCLUDED.viewer_preload_count").
		Set("viewer_fit_mode = EXCLUDED.viewer_fit_mode").
		Returning("*").
		Exec(ctx)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return settings, nil
}
```

**Step 2: Commit**

```bash
git add pkg/settings/service.go
git commit -m "feat: add settings service for viewer preferences"
```

---

## Task 4: Settings Validators

**Files:**
- Create: `pkg/settings/validators.go`
- Reference: `pkg/chapters/validators.go` (pattern)

**Step 1: Create the validators file**

```go
package settings

import "github.com/shishobooks/shisho/pkg/models"

// ViewerSettingsPayload is the request body for updating viewer settings.
type ViewerSettingsPayload struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
}

// ViewerSettingsResponse is the response for viewer settings.
type ViewerSettingsResponse struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
}

// ValidFitModes returns all valid fit mode values.
func ValidFitModes() []string {
	return []string{models.FitModeHeight, models.FitModeOriginal}
}

// IsValidFitMode returns true if the fit mode is valid.
func IsValidFitMode(mode string) bool {
	for _, valid := range ValidFitModes() {
		if mode == valid {
			return true
		}
	}
	return false
}
```

**Step 2: Commit**

```bash
git add pkg/settings/validators.go
git commit -m "feat: add settings validators"
```

---

## Task 5: Settings Handlers

**Files:**
- Create: `pkg/settings/handlers.go`
- Reference: `pkg/chapters/handlers.go` (pattern)

**Step 1: Create the handlers file**

```go
package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	settingsService *Service
}

func (h *handler) getViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	settings, err := h.settingsService.GetViewerSettings(ctx, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
	})
}

func (h *handler) updateViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	var payload ViewerSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate preload count (1-10)
	if payload.PreloadCount < 1 || payload.PreloadCount > 10 {
		return errcodes.ValidationError("preload_count must be between 1 and 10")
	}

	// Validate fit mode
	if !IsValidFitMode(payload.FitMode) {
		return errcodes.ValidationError("fit_mode must be 'fit-height' or 'original'")
	}

	settings, err := h.settingsService.UpdateViewerSettings(ctx, user.ID, payload.PreloadCount, payload.FitMode)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
	})
}
```

**Step 2: Commit**

```bash
git add pkg/settings/handlers.go
git commit -m "feat: add settings handlers"
```

---

## Task 6: Settings Routes

**Files:**
- Create: `pkg/settings/routes.go`
- Modify: `pkg/server/server.go`
- Reference: `pkg/chapters/routes.go` (pattern)

**Step 1: Create the routes file**

```go
package settings

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	h := &handler{
		settingsService: NewService(db),
	}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/viewer", h.getViewerSettings)
	g.PUT("/viewer", h.updateViewerSettings)
}
```

**Step 2: Add import and register routes in server.go**

In `pkg/server/server.go`, add import:
```go
"github.com/shishobooks/shisho/pkg/settings"
```

After line 76 (`filesystem.RegisterRoutesWithAuth(e, authMiddleware)`), add:
```go
// Settings routes (require authentication)
settings.RegisterRoutes(e, db, authMiddleware)
```

**Step 3: Run tests to verify**

Run: `make check`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/settings/routes.go pkg/server/server.go
git commit -m "feat: register settings routes"
```

---

## Task 7: CBZ Page Cache Package

**Files:**
- Create: `pkg/cbzpages/cache.go`
- Reference: `pkg/downloadcache/cache.go` (pattern), `pkg/cbz/cbz.go` (zip handling)

**Step 1: Create the cache file**

```go
package cbzpages

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// Cache manages extracted CBZ page images.
type Cache struct {
	dir string
}

// NewCache creates a new Cache with the given directory.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

// GetPage returns the path to a cached page image, extracting if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(cbzPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	// Check if page is already cached
	cacheDir := c.pageDir(fileID)
	pattern := filepath.Join(cacheDir, fmt.Sprintf("page_%d.*", pageNum))
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0], mimeTypeFromPath(matches[0]), nil
	}

	// Extract the page from the CBZ
	return c.extractPage(cbzPath, fileID, pageNum)
}

// extractPage extracts a single page from a CBZ file and caches it.
func (c *Cache) extractPage(cbzPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	f, err := os.Open(cbzPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	zipReader, err := zip.NewReader(f, stats.Size())
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Get sorted image files
	imageFiles := getSortedImageFiles(zipReader)
	if pageNum < 0 || pageNum >= len(imageFiles) {
		return "", "", errors.Errorf("page %d out of range (0-%d)", pageNum, len(imageFiles)-1)
	}

	targetFile := imageFiles[pageNum]

	// Create cache directory
	cacheDir := c.pageDir(fileID)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", "", errors.WithStack(err)
	}

	// Extract the page
	ext := strings.ToLower(filepath.Ext(targetFile.Name))
	cachedPath = filepath.Join(cacheDir, fmt.Sprintf("page_%d%s", pageNum, ext))

	r, err := targetFile.Open()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer r.Close()

	outFile, err := os.Create(cachedPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, r)
	if err != nil {
		os.Remove(cachedPath)
		return "", "", errors.WithStack(err)
	}

	return cachedPath, mimeTypeFromPath(cachedPath), nil
}

// pageDir returns the cache directory for a file's pages.
func (c *Cache) pageDir(fileID int) string {
	return filepath.Join(c.dir, "cbz", fmt.Sprintf("%d", fileID))
}

// Invalidate removes all cached pages for a file.
func (c *Cache) Invalidate(fileID int) error {
	return os.RemoveAll(c.pageDir(fileID))
}

// getSortedImageFiles returns a sorted list of image files from a zip reader.
func getSortedImageFiles(zipReader *zip.Reader) []*zip.File {
	var imageFiles []*zip.File
	for _, file := range zipReader.File {
		ext := strings.ToLower(filepath.Ext(file.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			imageFiles = append(imageFiles, file)
		}
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	return imageFiles
}

// mimeTypeFromPath returns the MIME type based on file extension.
func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
```

**Step 2: Commit**

```bash
git add pkg/cbzpages/cache.go
git commit -m "feat: add CBZ page cache for on-demand extraction"
```

---

## Task 8: CBZ Page Handler

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Reference: existing download handler pattern

**Step 1: Add page handler to books/handlers.go**

Add import at top:
```go
"github.com/shishobooks/shisho/pkg/cbzpages"
```

Add field to handler struct:
```go
type handler struct {
	// ... existing fields ...
	pageCache *cbzpages.Cache
}
```

Add handler method (add before the closing brace of the file):
```go
func (h *handler) getPage(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	pageNum, err := strconv.Atoi(c.Param("pageNum"))
	if err != nil {
		return errcodes.ValidationError("Invalid page number")
	}

	// Retrieve file with access check
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Only CBZ files have pages
	if file.FileType != models.FileTypeCBZ {
		return errcodes.ValidationError("Only CBZ files have pages")
	}

	// Validate page number against page count
	if file.PageCount != nil && pageNum >= *file.PageCount {
		return errcodes.NotFound("Page")
	}
	if pageNum < 0 {
		return errcodes.NotFound("Page")
	}

	// Get or extract the page
	cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, pageNum)
	if err != nil {
		return errors.WithStack(err)
	}

	// Set cache headers (cache for 1 year since page content doesn't change)
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Response().Header().Set("Content-Type", mimeType)

	return c.File(cachedPath)
}
```

**Step 2: Update routes.go to initialize cache and register route**

In `RegisterRoutesWithGroup`, add to handler initialization:
```go
pageCache := cbzpages.NewCache(cfg.DownloadCacheDir)

h := &handler{
	// ... existing fields ...
	pageCache: pageCache,
}
```

Add route after existing routes:
```go
g.GET("/files/:id/page/:pageNum", h.getPage)
```

**Step 3: Run tests**

Run: `make check`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/cbzpages/cache.go
git commit -m "feat: add CBZ page serving endpoint"
```

---

## Task 9: Frontend - Settings API Hook

**Files:**
- Create: `app/hooks/queries/settings.ts`
- Modify: `app/types/index.ts` (add export)
- Reference: `app/hooks/queries/books.ts` (pattern)

**Step 1: Create the settings hook file**

```typescript
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export interface ViewerSettings {
  preload_count: number;
  fit_mode: "fit-height" | "original";
}

export enum QueryKey {
  ViewerSettings = "ViewerSettings",
}

export const useViewerSettings = (
  options: Omit<
    UseQueryOptions<ViewerSettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ViewerSettings, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ViewerSettings],
    queryFn: ({ signal }) => {
      return API.request("GET", "/settings/viewer", null, null, signal);
    },
  });
};

interface UpdateViewerSettingsVariables {
  preload_count: number;
  fit_mode: "fit-height" | "original";
}

export const useUpdateViewerSettings = () => {
  const queryClient = useQueryClient();

  return useMutation<ViewerSettings, ShishoAPIError, UpdateViewerSettingsVariables>({
    mutationFn: (payload) => {
      return API.request("PUT", "/settings/viewer", payload, null);
    },
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.ViewerSettings], data);
    },
  });
};
```

**Step 2: Add export to app/types/index.ts**

Add to exports:
```typescript
export type { ViewerSettings } from "@/hooks/queries/settings";
```

**Step 3: Commit**

```bash
git add app/hooks/queries/settings.ts app/types/index.ts
git commit -m "feat: add viewer settings API hooks"
```

---

## Task 10: Frontend - Chapters API Hook

**Files:**
- Create: `app/hooks/queries/chapters.ts`
- Reference: `app/hooks/queries/books.ts` (pattern)

**Step 1: Create the chapters hook file**

```typescript
import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Chapter } from "@/types";

export enum QueryKey {
  FileChapters = "FileChapters",
}

interface ChaptersResponse {
  chapters: Chapter[];
}

export const useFileChapters = (
  fileId?: number,
  options: Omit<
    UseQueryOptions<Chapter[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Chapter[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(fileId),
    ...options,
    queryKey: [QueryKey.FileChapters, fileId],
    queryFn: async ({ signal }) => {
      const response: ChaptersResponse = await API.request(
        "GET",
        `/books/files/${fileId}/chapters`,
        null,
        null,
        signal,
      );
      return response.chapters;
    },
  });
};
```

**Step 2: Commit**

```bash
git add app/hooks/queries/chapters.ts
git commit -m "feat: add file chapters API hook"
```

---

## Task 11: Frontend - CBZ Reader Page Component

**Files:**
- Create: `app/components/pages/CBZReader.tsx`
- Reference: `app/components/pages/BookDetail.tsx` (patterns)

**Step 1: Create the reader component**

```typescript
import { ArrowLeft, ChevronLeft, ChevronRight, Settings } from "lucide-react";
import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { useBook } from "@/hooks/queries/books";
import { useFileChapters } from "@/hooks/queries/chapters";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";
import type { Chapter } from "@/types";

// Flatten chapters for progress bar (CBZ chapters don't nest)
const flattenChapters = (chapters: Chapter[]): Chapter[] => {
  const result: Chapter[] = [];
  for (const ch of chapters) {
    if (ch.start_page != null) {
      result.push(ch);
    }
    if (ch.children) {
      result.push(...flattenChapters(ch.children.filter(Boolean) as Chapter[]));
    }
  }
  return result;
};

export default function CBZReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Parse page from URL, default to 0
  const urlPage = parseInt(searchParams.get("page") || "0", 10);
  const [currentPage, setCurrentPage] = useState(isNaN(urlPage) ? 0 : urlPage);

  // Fetch book data for page count
  const { data: book } = useBook(bookId);
  const file = book?.files?.find((f) => f.id === Number(fileId));
  const pageCount = file?.page_count || 0;

  // Fetch chapters
  const { data: chapters = [] } = useFileChapters(
    fileId ? Number(fileId) : undefined,
  );
  const flatChapters = useMemo(() => flattenChapters(chapters), [chapters]);

  // Fetch and update viewer settings
  const { data: settings } = useViewerSettings();
  const updateSettings = useUpdateViewerSettings();
  const preloadCount = settings?.preload_count ?? 3;
  const fitMode = settings?.fit_mode ?? "fit-height";

  // Sync URL with current page
  useEffect(() => {
    const urlPage = parseInt(searchParams.get("page") || "0", 10);
    if (urlPage !== currentPage) {
      setSearchParams({ page: currentPage.toString() }, { replace: true });
    }
  }, [currentPage, searchParams, setSearchParams]);

  // Navigate to page
  const goToPage = useCallback(
    (page: number) => {
      if (page < 0) return;
      if (page >= pageCount) {
        // Navigate back to book detail
        navigate(`/libraries/${libraryId}/books/${bookId}`);
        return;
      }
      setCurrentPage(page);
    },
    [pageCount, navigate, libraryId, bookId],
  );

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.key === "d" || e.key === "D") {
        goToPage(currentPage + 1);
      } else if (e.key === "ArrowLeft" || e.key === "a" || e.key === "A") {
        goToPage(currentPage - 1);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [currentPage, goToPage]);

  // Preload pages
  const preloadedPages = useMemo(() => {
    const pages: number[] = [];
    for (
      let i = Math.max(0, currentPage - preloadCount);
      i <= Math.min(pageCount - 1, currentPage + preloadCount);
      i++
    ) {
      pages.push(i);
    }
    return pages;
  }, [currentPage, preloadCount, pageCount]);

  // Find current chapter
  const currentChapter = useMemo(() => {
    return flatChapters
      .filter((ch) => ch.start_page != null && ch.start_page <= currentPage)
      .at(-1);
  }, [flatChapters, currentPage]);

  // Progress percentage
  const progressPercent =
    pageCount > 1 ? (currentPage / (pageCount - 1)) * 100 : 0;

  // Handle progress bar click
  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const percent = x / rect.width;
    const targetPage = Math.round(percent * (pageCount - 1));
    goToPage(Math.max(0, Math.min(targetPage, pageCount - 1)));
  };

  // Build page URL
  const pageUrl = (page: number) =>
    `/api/books/files/${fileId}/page/${page}`;

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      {/* Header */}
      <header className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <Link
          to={`/libraries/${libraryId}/books/${bookId}`}
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>

        <div className="flex items-center gap-2">
          {/* Chapter dropdown */}
          {flatChapters.length > 0 && (
            <select
              className="text-sm bg-transparent border rounded px-2 py-1"
              value={currentChapter?.id ?? ""}
              onChange={(e) => {
                const ch = flatChapters.find(
                  (c) => c.id === Number(e.target.value),
                );
                if (ch?.start_page != null) {
                  goToPage(ch.start_page);
                }
              }}
            >
              {flatChapters.map((ch) => (
                <option key={ch.id} value={ch.id}>
                  {ch.title}
                </option>
              ))}
            </select>
          )}

          {/* Settings */}
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-64" align="end">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium">
                    Preload Count: {preloadCount}
                  </label>
                  <Slider
                    value={[preloadCount]}
                    min={1}
                    max={10}
                    step={1}
                    className="mt-2"
                    onValueChange={([value]) => {
                      updateSettings.mutate({
                        preload_count: value,
                        fit_mode: fitMode,
                      });
                    }}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">Fit Mode</label>
                  <div className="flex gap-2 mt-2">
                    <Button
                      variant={fitMode === "fit-height" ? "default" : "outline"}
                      size="sm"
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "fit-height",
                        })
                      }
                    >
                      Fit Height
                    </Button>
                    <Button
                      variant={fitMode === "original" ? "default" : "outline"}
                      size="sm"
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "original",
                        })
                      }
                    >
                      Original
                    </Button>
                  </div>
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </header>

      {/* Page Display */}
      <main className="flex-1 flex items-center justify-center overflow-hidden bg-black">
        <img
          src={pageUrl(currentPage)}
          alt={`Page ${currentPage + 1}`}
          className={
            fitMode === "fit-height"
              ? "max-h-full w-auto object-contain"
              : "max-w-full max-h-full object-contain"
          }
        />
        {/* Preloaded images (hidden) */}
        {preloadedPages
          .filter((p) => p !== currentPage)
          .map((p) => (
            <link key={p} rel="prefetch" href={pageUrl(p)} as="image" />
          ))}
      </main>

      {/* Controls */}
      <footer className="border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        {/* Progress Bar */}
        <div className="px-4 pt-3">
          <div
            className="relative h-1.5 bg-muted rounded-full cursor-pointer"
            onClick={handleProgressClick}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
            {/* Chapter markers */}
            {flatChapters.map((ch) => {
              if (ch.start_page == null || pageCount <= 1) return null;
              const pos = (ch.start_page / (pageCount - 1)) * 100;
              return (
                <div
                  key={ch.id}
                  className="absolute top-1/2 -translate-y-1/2 w-0.5 h-2.5 bg-muted-foreground/50"
                  style={{ left: `${pos}%` }}
                  title={ch.title}
                />
              );
            })}
          </div>
          {currentChapter && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentChapter.title}
            </div>
          )}
        </div>

        {/* Navigation buttons */}
        <div className="flex items-center justify-between px-4 py-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => goToPage(currentPage - 1)}
            disabled={currentPage === 0}
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {currentPage + 1} of {pageCount}
          </span>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => goToPage(currentPage + 1)}
          >
            <ChevronRight className="h-5 w-5" />
          </Button>
        </div>
      </footer>
    </div>
  );
}
```

**Step 2: Commit**

```bash
git add app/components/pages/CBZReader.tsx
git commit -m "feat: add CBZ reader page component"
```

---

## Task 12: Frontend - Add Reader Route

**Files:**
- Modify: `app/router.tsx`

**Step 1: Add import and route**

Add import:
```typescript
import CBZReader from "@/components/pages/CBZReader";
```

Add route after the book detail route (after line 192):
```typescript
{
  path: "libraries/:libraryId/books/:bookId/files/:fileId/read",
  element: (
    <ProtectedRoute checkLibraryAccess>
      <CBZReader />
    </ProtectedRoute>
  ),
},
```

**Step 2: Commit**

```bash
git add app/router.tsx
git commit -m "feat: add CBZ reader route"
```

---

## Task 13: Frontend - Add Read Button to Book Detail

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add Read button for CBZ files**

Find the FileRow component and add a Read button for CBZ files. In the button area (near the download/edit buttons), add:

```typescript
{file.file_type === "cbz" && (
  <Link
    to={`/libraries/${libraryId}/books/${book.id}/files/${file.id}/read`}
  >
    <Button variant="outline" size="sm">
      Read
    </Button>
  </Link>
)}
```

**Step 2: Import Link if not already imported**

Ensure `Link` is imported from `react-router-dom`.

**Step 3: Run linting and type check**

Run: `yarn lint`
Expected: No errors

**Step 4: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "feat: add Read button for CBZ files on book detail page"
```

---

## Task 14: Integration Testing

**Files:**
- Manual testing with running development server

**Step 1: Start development server**

Run: `make start`
Expected: Server starts without errors

**Step 2: Test settings API**

1. Navigate to a book detail page
2. Open browser dev tools Network tab
3. Verify GET `/settings/viewer` returns settings or defaults
4. Open the reader and change settings
5. Verify PUT `/settings/viewer` is called and settings persist

**Step 3: Test page serving**

1. Navigate to a CBZ file's reader view
2. Verify images load from `/api/books/files/:id/page/:pageNum`
3. Check that caching headers are present
4. Navigate between pages and verify preloading works

**Step 4: Test navigation**

1. Use keyboard arrows to navigate
2. Use A/D keys to navigate
3. Click the progress bar to jump to pages
4. Verify URL updates with page number
5. Refresh page and verify it returns to the same page

**Step 5: Test chapter markers**

1. Open a CBZ file that has detected chapters
2. Verify chapter markers appear on progress bar
3. Verify chapter dropdown works
4. Verify current chapter name displays below progress bar

---

## Task 15: Final Cleanup and Verification

**Files:**
- All modified files

**Step 1: Run full check suite**

Run: `make check`
Expected: All tests pass, no lint errors

**Step 2: Review all changes**

Run: `git diff master`
Review changes for any issues.

**Step 3: Create final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: cleanup and polish CBZ viewer implementation"
```

---

## Summary of API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/settings/viewer` | Get viewer settings |
| PUT | `/settings/viewer` | Update viewer settings |
| GET | `/books/files/:id/page/:pageNum` | Get CBZ page image |

## Summary of New Files

**Backend:**
- `pkg/migrations/20260116100000_add_user_settings.go`
- `pkg/models/user_settings.go`
- `pkg/settings/service.go`
- `pkg/settings/validators.go`
- `pkg/settings/handlers.go`
- `pkg/settings/routes.go`
- `pkg/cbzpages/cache.go`

**Frontend:**
- `app/hooks/queries/settings.ts`
- `app/hooks/queries/chapters.ts`
- `app/components/pages/CBZReader.tsx`

**Modified:**
- `pkg/server/server.go` (register settings routes)
- `pkg/books/handlers.go` (add page handler)
- `pkg/books/routes.go` (add page route)
- `app/router.tsx` (add reader route)
- `app/components/pages/BookDetail.tsx` (add Read button)
