# Resync Button Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a resync button to TopNav that triggers library-specific scans and displays scan status.

**Architecture:** Add `library_id` column to jobs table to support library-specific scans. Create a new React component that polls the latest scan job for the current library and displays appropriate UI state (idle/active/failed). Integrate into TopNav after the Library Switcher.

**Tech Stack:** Go/Echo/Bun (backend), React/TypeScript/TanStack Query (frontend), SQLite (database)

---

## Task 1: Database Migration - Add library_id Column

**Files:**
- Create: `pkg/migrations/20260113000000_add_library_id_to_jobs.go`

**Step 1: Create migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Add library_id column to jobs table
		_, err := db.Exec(`ALTER TABLE jobs ADD COLUMN library_id INTEGER REFERENCES libraries(id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for filtering scans by library (includes global scans where library_id IS NULL)
		_, err = db.Exec(`CREATE INDEX ix_jobs_type_library_created ON jobs(type, library_id, created_at DESC)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_jobs_type_library_created`)
		if err != nil {
			return errors.WithStack(err)
		}
		// SQLite doesn't support DROP COLUMN, so we'd need to recreate the table
		// For simplicity, we'll leave the column in place on rollback
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration**

Run: `make db:migrate`
Expected: Migration completes successfully

**Step 3: Commit**

```bash
git add pkg/migrations/20260113000000_add_library_id_to_jobs.go
git commit -m "$(cat <<'EOF'
[DB] Add library_id column to jobs table

Adds library_id foreign key to support library-specific scan jobs.
Global scans will have NULL library_id.
EOF
)"
```

---

## Task 2: Update Job Model with LibraryID Field

**Files:**
- Modify: `pkg/models/job.go:25-37`

**Step 1: Add LibraryID field to Job struct**

Update the Job struct to include:

```go
type Job struct {
	bun.BaseModel `bun:"table:jobs,alias:j" tstype:"-"`

	ID         int         `bun:",pk,nullzero" json:"id"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Type       string      `bun:",nullzero" json:"type" tstype:"JobType"`
	Status     string      `bun:",nullzero" json:"status" tstype:"JobStatus"`
	Data       string      `bun:",nullzero" json:"-"`
	DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData"`
	Progress   int         `json:"progress"`
	ProcessID  *string     `json:"process_id,omitempty"`
	LibraryID  *int        `json:"library_id,omitempty"`
}
```

**Step 2: Generate TypeScript types**

Run: `make tygo`
Expected: TypeScript types regenerated (may say "Nothing to be done" if already current)

**Step 3: Commit**

```bash
git add pkg/models/job.go
git commit -m "$(cat <<'EOF'
[Models] Add LibraryID field to Job model

Supports library-specific scan jobs. NULL means global scan.
EOF
)"
```

---

## Task 3: Update Jobs Validators

**Files:**
- Modify: `pkg/jobs/validators.go:3-12`

**Step 1: Add LibraryID to CreateJobPayload and type/library filters to ListJobsQuery**

```go
package jobs

type CreateJobPayload struct {
	Type      string      `json:"type" validate:"required,oneof=export scan"`
	Data      interface{} `json:"data" validate:"required" tstype:"JobExportData | JobScanData"`
	LibraryID *int        `json:"library_id,omitempty"`
}

type ListJobsQuery struct {
	Limit              int      `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset             int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Status             []string `query:"status" json:"status,omitempty" validate:"dive,oneof=pending in_progress completed failed"`
	Type               *string  `query:"type" json:"type,omitempty" validate:"omitempty,oneof=export scan"`
	LibraryIDOrGlobal  *int     `query:"library_id_or_global" json:"library_id_or_global,omitempty"`
}
```

**Step 2: Generate TypeScript types**

Run: `make tygo`
Expected: TypeScript types regenerated

**Step 3: Commit**

```bash
git add pkg/jobs/validators.go
git commit -m "$(cat <<'EOF'
[Jobs] Add library_id to CreateJobPayload and filter options

- CreateJobPayload accepts optional library_id
- ListJobsQuery accepts type and library_id_or_global filters
EOF
)"
```

---

## Task 4: Write Failing Tests for Service Changes

**Files:**
- Modify: `pkg/jobs/service_test.go`

**Step 1: Write test for HasActiveJob with libraryID**

Add tests at the end of the file:

```go
func TestHasActiveJob_WithLibraryID_NoJobs(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_MatchingLibrary(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_DifferentLibrary(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID1 := 1
	libraryID2 := 2
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID1,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// Should not find active job for different library
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID2)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_GlobalJobBlocks(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a global scan job (library_id is NULL)
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  nil,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// Should block any library-specific scan
	libraryID := 1
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_ChecksAny(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// With nil libraryID, should find any active scan
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestListJobs_FilterByType(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create one scan job and one export job
	scanJob := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, scanJob)
	require.NoError(t, err)

	exportJob := &models.Job{
		Type:       models.JobTypeExport,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobExportData{},
	}
	err = svc.CreateJob(ctx, exportJob)
	require.NoError(t, err)

	// Filter by scan type
	scanType := models.JobTypeScan
	jobs, err := svc.ListJobs(ctx, ListJobsOptions{Type: &scanType})
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, models.JobTypeScan, jobs[0].Type)
}

func TestListJobs_FilterByLibraryIDOrGlobal(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID1 := 1
	libraryID2 := 2

	// Create jobs: global, library 1, library 2
	globalJob := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  nil,
	}
	err := svc.CreateJob(ctx, globalJob)
	require.NoError(t, err)

	lib1Job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID1,
	}
	err = svc.CreateJob(ctx, lib1Job)
	require.NoError(t, err)

	lib2Job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID2,
	}
	err = svc.CreateJob(ctx, lib2Job)
	require.NoError(t, err)

	// Filter for library 1 or global - should get global and lib1 jobs
	jobs, err := svc.ListJobs(ctx, ListJobsOptions{LibraryIDOrGlobal: &libraryID1})
	require.NoError(t, err)
	assert.Len(t, jobs, 2)

	// Verify we got the right jobs
	var foundGlobal, foundLib1 bool
	for _, j := range jobs {
		if j.LibraryID == nil {
			foundGlobal = true
		} else if *j.LibraryID == libraryID1 {
			foundLib1 = true
		}
	}
	assert.True(t, foundGlobal, "should include global job")
	assert.True(t, foundLib1, "should include library 1 job")
}
```

**Step 2: Run tests to verify they fail**

Run: `make test`
Expected: Tests fail because `HasActiveJob` and filter options don't exist yet

**Step 3: Commit**

```bash
git add pkg/jobs/service_test.go
git commit -m "$(cat <<'EOF'
[Jobs] Add failing tests for HasActiveJob and list filters

Tests for library-specific active job checks and list filtering.
EOF
)"
```

---

## Task 5: Update Jobs Service - HasActiveJob and List Filters

**Files:**
- Modify: `pkg/jobs/service.go:19-26,108-157,159-173`

**Step 1: Add Type and LibraryIDOrGlobal to ListJobsOptions**

Update the struct:

```go
type ListJobsOptions struct {
	Limit              *int
	Offset             *int
	Statuses           []string
	ProcessIDToExclude *string
	Type               *string
	LibraryIDOrGlobal  *int // Matches library_id = X OR library_id IS NULL

	includeTotal bool
}
```

**Step 2: Update listJobsWithTotal to handle new filters**

Add filtering in `listJobsWithTotal` after the existing filters (around line 138):

```go
	if opts.Type != nil {
		q = q.Where("j.type = ?", *opts.Type)
	}
	if opts.LibraryIDOrGlobal != nil {
		q = q.WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.
				Where("j.library_id = ?", *opts.LibraryIDOrGlobal).
				WhereOr("j.library_id IS NULL")
		})
	}
```

**Step 3: Replace HasActiveJobByType with HasActiveJob**

Replace the existing `HasActiveJobByType` function:

```go
// HasActiveJob checks if there's a pending or in-progress job of the given type.
// If libraryID is nil, checks for any active scan.
// If libraryID is set, checks for active scan with that library_id OR library_id IS NULL (global).
func (svc *Service) HasActiveJob(ctx context.Context, jobType string, libraryID *int) (bool, error) {
	q := svc.db.NewSelect().
		Model((*models.Job)(nil)).
		Where("type = ?", jobType).
		WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Where("status = ?", models.JobStatusPending).
				WhereOr("status = ?", models.JobStatusInProgress)
		})

	if libraryID != nil {
		q = q.WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.
				Where("library_id = ?", *libraryID).
				WhereOr("library_id IS NULL")
		})
	}

	count, err := q.Count(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return count > 0, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `make test`
Expected: All tests pass

**Step 5: Commit**

```bash
git add pkg/jobs/service.go
git commit -m "$(cat <<'EOF'
[Jobs] Add HasActiveJob and list filter support

- HasActiveJob supports library-specific checks (blocks if global scan running)
- ListJobsOptions supports Type and LibraryIDOrGlobal filters
EOF
)"
```

---

## Task 6: Update Jobs Handler - Create and List Endpoints

**Files:**
- Modify: `pkg/jobs/handlers.go:17-56,75-99`

**Step 1: Update create handler to set LibraryID and use new HasActiveJob**

```go
func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := CreateJobPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Check if a scan job is already running or pending.
	if params.Type == models.JobTypeScan {
		hasActive, err := h.jobService.HasActiveJob(ctx, models.JobTypeScan, params.LibraryID)
		if err != nil {
			return errors.WithStack(err)
		}
		if hasActive {
			return errcodes.Conflict("A scan job is already running or pending.")
		}
	}

	job := &models.Job{
		Type:       params.Type,
		Status:     models.JobStatusPending,
		DataParsed: params.Data,
		LibraryID:  params.LibraryID,
	}

	err := h.jobService.CreateJob(ctx, job)
	if err != nil {
		return errors.WithStack(err)
	}

	job, err = h.jobService.RetrieveJob(ctx, RetrieveJobOptions{
		ID: &job.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, job))
}
```

**Step 2: Update list handler to pass new filter options**

```go
func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListJobsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	jobs, total, err := h.jobService.ListJobsWithTotal(ctx, ListJobsOptions{
		Limit:             &params.Limit,
		Offset:            &params.Offset,
		Statuses:          params.Status,
		Type:              params.Type,
		LibraryIDOrGlobal: params.LibraryIDOrGlobal,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Jobs  []*models.Job `json:"jobs"`
		Total int           `json:"total"`
	}{jobs, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
```

**Step 3: Run tests**

Run: `make test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/jobs/handlers.go
git commit -m "$(cat <<'EOF'
[Jobs] Update handlers for library_id support

- Create handler accepts library_id and uses HasActiveJob
- List handler passes type and library_id_or_global filters
EOF
)"
```

---

## Task 7: Update Worker to Filter Libraries

**Files:**
- Modify: `pkg/worker/scan.go:42-52`

**Step 1: Update ProcessScanJob to filter by library_id**

Update the beginning of `ProcessScanJob`:

```go
func (w *Worker) ProcessScanJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	jobLog.Info("processing scan job", nil)

	allLibraries, err := w.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	// Filter to specific library if set
	if job.LibraryID != nil {
		filtered := make([]*models.Library, 0, 1)
		for _, lib := range allLibraries {
			if lib.ID == *job.LibraryID {
				filtered = append(filtered, lib)
				break
			}
		}
		allLibraries = filtered
	}

	jobLog.Info("processing libraries", logger.Data{"count": len(allLibraries)})

	// ... rest of function unchanged
```

**Step 2: Run tests**

Run: `make test`
Expected: All tests pass

**Step 3: Commit**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Worker] Filter libraries when job has library_id

Scan job only processes the specified library if library_id is set.
EOF
)"
```

---

## Task 8: Generate TypeScript Types

**Step 1: Run tygo to regenerate types**

Run: `make tygo`
Expected: Types regenerated (or "Nothing to be done" if already current)

**Step 2: Verify generated types include library_id**

Check that `app/types/generated/models.ts` includes `library_id?: number` in Job interface and `app/types/generated/jobs.ts` includes `library_id?: number` in CreateJobPayload and new fields in ListJobsQuery.

---

## Task 9: Add QueryKey for LatestScanJob

**Files:**
- Modify: `app/hooks/queries/jobs.ts:11-15`

**Step 1: Add new query key**

```typescript
export enum QueryKey {
  RetrieveJob = "RetrieveJob",
  ListJobs = "ListJobs",
  ListJobLogs = "ListJobLogs",
  LatestScanJob = "LatestScanJob",
}
```

**Step 2: Commit**

```bash
git add app/hooks/queries/jobs.ts
git commit -m "$(cat <<'EOF'
[UI] Add LatestScanJob query key
EOF
)"
```

---

## Task 10: Create useLatestScanJob Hook

**Files:**
- Modify: `app/hooks/queries/jobs.ts`

**Step 1: Add useLatestScanJob hook after useJobs**

```typescript
export const useLatestScanJob = (libraryId: number | undefined) => {
  return useQuery<ListJobsData, ShishoAPIError>({
    queryKey: [QueryKey.LatestScanJob, libraryId],
    queryFn: ({ signal }) => {
      return API.request("GET", "/jobs", null, {
        type: "scan",
        library_id_or_global: libraryId,
        limit: 1,
      }, signal);
    },
    enabled: libraryId !== undefined,
    refetchInterval: (query) => {
      const job = query.state.data?.jobs[0];
      const isActive = job?.status === "pending" || job?.status === "in_progress";
      return isActive ? 2000 : 30000;
    },
  });
};
```

**Step 2: Commit**

```bash
git add app/hooks/queries/jobs.ts
git commit -m "$(cat <<'EOF'
[UI] Add useLatestScanJob hook

Polls for latest scan job with dynamic refetch interval.
EOF
)"
```

---

## Task 11: Create ResyncButton Component

**Files:**
- Create: `app/components/library/ResyncButton.tsx`

**Step 1: Create the component file**

```tsx
import { AlertTriangle, RefreshCw } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useCreateJob, useLatestScanJob } from "@/hooks/queries/jobs";
import { formatDistanceToNow } from "@/libraries/date";

interface ResyncButtonProps {
  libraryId: number;
}

export function ResyncButton({ libraryId }: ResyncButtonProps) {
  const navigate = useNavigate();
  const { data, isLoading } = useLatestScanJob(libraryId);
  const createJob = useCreateJob();

  const latestJob = data?.jobs[0];
  const isActive = latestJob?.status === "pending" || latestJob?.status === "in_progress";
  const isFailed = latestJob?.status === "failed";

  const handleClick = () => {
    if (isActive || isFailed) {
      navigate(`/settings/jobs/${latestJob?.id}`);
    } else {
      createJob.mutate({
        payload: { type: "scan", library_id: libraryId, data: {} },
      });
    }
  };

  const getTooltipContent = () => {
    if (isActive && latestJob) {
      return `Scan started ${formatDistanceToNow(new Date(latestJob.created_at))}`;
    }
    if (isFailed) {
      return "Last scan failed - view logs";
    }
    return "Resync library";
  };

  if (isLoading) {
    return (
      <Button className="h-9 w-9" disabled size="icon" variant="ghost">
        <RefreshCw className="h-4 w-4" />
      </Button>
    );
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            className="h-9 w-9 relative"
            disabled={createJob.isPending}
            onClick={handleClick}
            size="icon"
            variant="ghost"
          >
            <RefreshCw
              className={`h-4 w-4 ${isActive ? "animate-spin" : ""}`}
            />
            {isFailed && (
              <AlertTriangle className="h-3 w-3 text-yellow-500 absolute -top-0.5 -right-0.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>{getTooltipContent()}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
```

**Step 2: Commit**

```bash
git add app/components/library/ResyncButton.tsx
git commit -m "$(cat <<'EOF'
[UI] Create ResyncButton component

Shows scan status with three states:
- Idle: static icon, click starts scan
- Active: spinning icon, click navigates to job page
- Failed: warning badge, click navigates to job logs
EOF
)"
```

---

## Task 12: Integrate ResyncButton into TopNav

**Files:**
- Modify: `app/components/library/TopNav.tsx`

**Step 1: Add import for ResyncButton**

Add to imports:

```typescript
import { ResyncButton } from "@/components/library/ResyncButton";
```

**Step 2: Add permission check for jobs:write**

Add after `canCreateLibrary`:

```typescript
const canResync = hasPermission("jobs", "write");
```

**Step 3: Render ResyncButton after Library Switcher**

After the closing `</DropdownMenu>` for the Library Switcher (around line 180), add:

```tsx
            {/* Resync Button */}
            {libraryId && canResync && (
              <ResyncButton libraryId={Number(libraryId)} />
            )}
```

**Step 4: Run linting**

Run: `yarn lint`
Expected: No lint errors

**Step 5: Commit**

```bash
git add app/components/library/TopNav.tsx
git commit -m "$(cat <<'EOF'
[UI] Add ResyncButton to TopNav

Shows after Library Switcher when user has jobs:write permission.
EOF
)"
```

---

## Task 13: Run Full Check Suite

**Step 1: Run all checks**

Run: `make check`
Expected: All tests pass, no lint errors

**Step 2: Manual testing checklist**

Start the dev server with `make start` and verify:

- [ ] Button appears in TopNav after library switcher
- [ ] Clicking idle button creates a scan job (verify in Settings > Jobs)
- [ ] Button spins while scan is in progress
- [ ] Clicking spinning button navigates to job page
- [ ] If scan fails, warning badge appears
- [ ] Clicking failed button navigates to job page
- [ ] Button only appears for users with jobs:write permission

---

## Task 14: Final Commit - Feature Complete

**Step 1: Create final summary commit if needed**

If all individual commits are clean, no additional commit needed. Otherwise:

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Feature] Add resync button to TopNav

Complete implementation of library-specific resync functionality:
- Database: Added library_id column to jobs table
- Backend: HasActiveJob with library filtering, list filters
- Worker: Filter libraries when library_id is set
- Frontend: ResyncButton component with idle/active/failed states
EOF
)"
```
