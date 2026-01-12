# Job Logs Design

## Overview

Add a job logging system that persists logs to the database, enables viewing logs in the UI, and supports job failure states with proper error capture.

## Goals

1. Persist job logs to database for debugging via UI
2. Add `failed` status for jobs with immediate failure on panic/error
3. Live-tailing UI with autoscroll behavior
4. Configurable retention with automatic cleanup

## Database Schema

### New `job_logs` table

```sql
CREATE TABLE job_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  level TEXT NOT NULL,
  message TEXT NOT NULL,
  data TEXT,
  stack_trace TEXT
);

CREATE INDEX idx_job_logs_job_id ON job_logs(job_id);
CREATE INDEX idx_job_logs_created_at ON job_logs(created_at);
```

### Jobs table modification

Add `failed` status:

```go
const (
    JobStatusPending    = "pending"
    JobStatusInProgress = "in_progress"
    JobStatusCompleted  = "completed"
    JobStatusFailed     = "failed"
)
```

Add index for retention cleanup:

```sql
CREATE INDEX idx_jobs_status_created_at ON jobs(status, created_at);
```

## Configuration

Add to `config.Config`:

```go
JobRetentionDays int `yaml:"job_retention_days" env:"JOB_RETENTION_DAYS" default:"30"`
```

## Log Levels

- `info` - General progress information
- `warn` - Non-fatal issues
- `error` - Errors that cause job failure
- `fatal` - Panics/crashes that cause job failure

Both `error` and `fatal` automatically capture stack traces.

## Backend Architecture

### New package: `pkg/joblogs/`

```
pkg/joblogs/
├── service.go      # CRUD operations
├── handlers.go     # HTTP handlers
├── routes.go       # Route registration
└── logger.go       # JobLogger wrapper
```

### JobLogger

Writes to both database and stdout:

```go
type JobLogger struct {
    jobID     int
    service   *Service
    stdLogger *logger.Logger
    ctx       context.Context
}

func (l *JobLogger) Info(msg string, data logger.Data)
func (l *JobLogger) Warn(msg string, data logger.Data)
func (l *JobLogger) Error(msg string, err error, data logger.Data)   // auto stack trace
func (l *JobLogger) Fatal(msg string, err error, data logger.Data)   // auto stack trace
```

### Data truncation

Individual data field values truncated at 1KB using middle truncation:

```go
func truncateMiddle(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    half := (maxLen - 5) / 2
    return s[:half] + " ... " + s[len(s)-half:]
}
```

### Worker integration

Panic recovery with failure handling:

```go
func (w *Worker) processJob(ctx context.Context, job *models.Job) {
    jobLog := w.jobLogService.NewLogger(ctx, job.ID)

    defer func() {
        if r := recover(); r != nil {
            stack := string(debug.Stack())
            jobLog.Fatal("job panicked", fmt.Errorf("%v", r), logger.Data{"panic": r})
            w.jobService.UpdateStatus(ctx, job.ID, models.JobStatusFailed)
        }
    }()

    var err error
    switch job.Type {
    case models.JobTypeScan:
        err = w.ProcessScanJob(ctx, job, jobLog)
    }

    if err != nil {
        jobLog.Error("job failed", err, nil)
        w.jobService.UpdateStatus(ctx, job.ID, models.JobStatusFailed)
        return
    }

    w.jobService.UpdateStatus(ctx, job.ID, models.JobStatusCompleted)
}
```

### Retention cleanup

Runs hourly in worker, deletes entire jobs (logs cascade):

```go
func (s *Service) CleanupOldJobs(ctx context.Context, retentionDays int) error {
    cutoff := time.Now().AddDate(0, 0, -retentionDays)
    _, err := s.db.NewDelete().Model((*models.Job)(nil)).
        Where("created_at < ?", cutoff).
        Where("status IN (?, ?)", JobStatusCompleted, JobStatusFailed).
        Exec(ctx)
    return err
}
```

## API Endpoints

### List job logs

```
GET /api/jobs/:id/logs
    Query params:
    - after_id: int       # For polling - return logs with id > after_id
    - level: string       # Filter by level (comma-separated)

    Response: { logs: JobLog[], job: Job }
```

## Frontend

### New route

`/jobs/:id` - Job detail page

### Component structure

```
JobDetail.tsx
├── Header (type, status, timestamps, duration, process ID)
├── Toolbar
│   ├── Search input (client-side filter on message + data)
│   └── Log level filter (multi-select chips)
└── Log container
    ├── LogEntry[] (expandable)
    │   ├── Caret icon (rotates on expand)
    │   ├── Timestamp (gray)
    │   ├── Level badge (colored)
    │   ├── Message (white)
    │   └── Data preview (gray, CSS truncate)
    │   └── [Expanded] Full data + stack trace
    └── Autoscroll checkbox
```

### Polling

2-second interval while job status is `pending` or `in_progress`:

```typescript
useEffect(() => {
  if (job?.status === "pending" || job?.status === "in_progress") {
    const interval = setInterval(refetch, 2000);
    return () => clearInterval(interval);
  }
}, [job?.status]);
```

### Autoscroll behavior

- Checkbox at bottom, checked by default
- Unchecks automatically when user scrolls up
- Re-checking scrolls to bottom and resumes autoscroll

### Level badge colors

| Level | Background | Text |
|-------|------------|------|
| info | `bg-blue-500/20` | `text-blue-400` |
| warn | `bg-yellow-500/20` | `text-yellow-400` |
| error | `bg-red-500/20` | `text-red-400` |
| fatal | `bg-red-700/30` | `text-red-300` |

## Files to Create/Modify

### New files

- `pkg/models/job_log.go`
- `pkg/joblogs/service.go`
- `pkg/joblogs/handlers.go`
- `pkg/joblogs/routes.go`
- `pkg/joblogs/logger.go`
- `app/components/pages/JobDetail.tsx`

### Modified files

- `pkg/models/job.go` - add `failed` status
- `pkg/db/migrations/000001_init.up.sql` - add job_logs table + indexes
- `pkg/worker/worker.go` - integrate JobLogger, panic recovery, retention cleanup
- `pkg/worker/scan.go` - replace log calls with JobLogger
- `pkg/jobs/service.go` - add CleanupOldJobs
- `pkg/config/config.go` - add JobRetentionDays
- `shisho.example.yaml` - add job_retention_days
- `app/hooks/queries/jobs.ts` - add useJobLogs hook
- `app/router.tsx` - add route
- `tygo.yaml` - add joblogs package
