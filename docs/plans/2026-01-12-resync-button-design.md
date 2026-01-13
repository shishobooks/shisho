# Resync Button Design

A resync button in the top navigation that triggers library scans and shows scan status.

## Overview

Add a button next to the Library Switcher that:
- Creates a scan job for the current library when clicked
- Spins while a scan is in progress (library-specific or global)
- Shows a warning indicator if the latest scan failed
- Navigates to the job page when clicked during an active scan or after failure

## Data Model Changes

### Database Migration

Add `library_id` column to jobs table:

```sql
ALTER TABLE jobs ADD COLUMN library_id INTEGER REFERENCES libraries(id);
CREATE INDEX ix_jobs_type_library_created ON jobs (type, library_id, created_at DESC);
```

### Job Model (`pkg/models/job.go`)

```go
type Job struct {
    // ... existing fields
    LibraryID *int `json:"library_id,omitempty"`
}

type JobScanData struct {
    // Keep empty - library_id is now a top-level column
}
```

## Backend Changes

### Jobs Service (`pkg/jobs/service.go`)

Update `ListJobsOptions`:
```go
type ListJobsOptions struct {
    // ... existing fields
    Type              *string
    LibraryIDOrGlobal *int  // Matches library_id = X OR library_id IS NULL
}
```

Update `listJobsWithTotal` to filter by new fields.

Update `HasActiveJobByType` to `HasActiveJob(ctx, jobType string, libraryID *int)`:
- If libraryID is nil, check for any active scan
- If libraryID is set, check for active scan with that library_id OR library_id IS NULL

### Jobs Handler (`pkg/jobs/handlers.go`)

Create handler:
- Accept `library_id` in payload
- Set on job before creation

List handler:
- Accept `type` query param
- Accept `library_id_or_global` query param

### Jobs Validators (`pkg/jobs/validators.go`)

```go
type CreateJobPayload struct {
    Type      string `json:"type" validate:"required,oneof=export scan"`
    LibraryID *int   `json:"library_id,omitempty"`
    Data      interface{} `json:"data" validate:"required"`
}

type ListJobsQuery struct {
    // ... existing fields
    Type              *string `query:"type"`
    LibraryIDOrGlobal *int    `query:"library_id_or_global"`
}
```

### Worker (`pkg/worker/scan.go`)

Modify `ProcessScanJob`:
```go
func (w *Worker) ProcessScanJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
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

    // ... rest of scan logic unchanged
}
```

## Frontend Changes

### New Hook (`app/hooks/queries/jobs.ts`)

```typescript
export function useLatestScanJob(libraryId: number | undefined) {
  return useQuery({
    queryKey: [QueryKey.LatestScanJob, libraryId],
    queryFn: () => api.get<{ jobs: Job[] }>("/jobs", {
      params: { type: "scan", library_id_or_global: libraryId, limit: 1 }
    }),
    enabled: !!libraryId,
    refetchInterval: (query) => {
      const job = query.state.data?.jobs[0];
      const isActive = job?.status === "pending" || job?.status === "in_progress";
      return isActive ? 2000 : 30000;
    },
  });
}
```

### New Component (`app/components/library/ResyncButton.tsx`)

States:
1. **Idle** - Static RefreshCw icon, click triggers new scan
2. **Active** - Spinning icon, tooltip shows "Scan started {time}", click navigates to job page
3. **Failed** - Static icon with yellow warning badge, tooltip shows failure message with link

```tsx
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
      navigate(`/settings/jobs/${latestJob.id}`);
    } else {
      createJob.mutate({
        payload: { type: "scan", library_id: libraryId, data: {} }
      });
    }
  };

  // Render button with tooltip...
}
```

### TopNav Integration (`app/components/library/TopNav.tsx`)

Render after Library Switcher, only when libraryId exists and user has jobs:write permission.

## UI Behavior

| State | Icon | Tooltip | Click Action |
|-------|------|---------|--------------|
| Idle (no job or completed) | Static RefreshCw | "Resync library" | Create new scan job |
| Pending/In Progress | Spinning RefreshCw | "Scan started {time ago}" | Navigate to job page |
| Failed | Static RefreshCw + yellow badge | "Last scan failed - view logs" | Navigate to job page |

## Edge Cases

- **No previous scan**: Shows idle state, click creates first scan
- **Global scan running**: Button spins for all libraries
- **Library-specific scan running**: Button spins only for that library
- **Global scan failed**: Warning shows on all library buttons
- **Library-specific scan failed**: Warning shows only on that library
- **Warning clears**: When any new scan starts (global or for this library)
