# File Hashing & Move Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add sha256 content hashing to files, enabling move/rename detection in both the filesystem monitor (real-time) and the scan job (safety net). Establish a `file_fingerprints` table and async hash generation infrastructure as the foundation for future fuzzy/perceptual fingerprints.

**Architecture:** A new `file_fingerprints` sibling table stores `(file_id, algorithm, value)` rows. A new `pkg/fingerprint/` package exposes `ComputeSHA256(path) (string, error)`. A new `JobTypeHashGeneration` job type computes sha256 for files missing a fingerprint. The monitor detects moves in real-time by computing hashes synchronously for CREATE events in any batch that also contains REMOVE events. The scan detects moves via an end-of-walk reconciliation phase that matches orphans to new files by `(size, sha256)`. All infrastructure is designed to support future algorithms (pHash, SimHash, Chromaprint) by adding new rows with different `algorithm` values.

**Tech Stack:** Go, Bun ORM, SQLite, Echo, fsnotify (existing). No new dependencies.

**Design spec:** `docs/superpowers/specs/2026-04-12-file-hashing-move-detection-design.md`

---

## File Structure

### New files

| Path | Responsibility |
|---|---|
| `pkg/fingerprint/fingerprint.go` | `ComputeSHA256(path) (string, error)` — streaming sha256 over file contents |
| `pkg/fingerprint/fingerprint_test.go` | Unit tests for `ComputeSHA256` |
| `pkg/migrations/20260412000000_add_file_fingerprints.go` | Create `file_fingerprints` table with indexes |
| `pkg/models/file_fingerprint.go` | Bun model for `FileFingerprint` |
| `pkg/fingerprints/service.go` | CRUD for `file_fingerprints` — insert, lookup by hash, delete by file ID |
| `pkg/fingerprints/service_test.go` | Integration tests for the service using test DB |
| `pkg/worker/hash_generation.go` | `ProcessHashGenerationJob` handler + `EnsureHashGenerationJob` helper |
| `pkg/worker/hash_generation_test.go` | Tests for hash generation job |
| `website/docs/file-fingerprints.md` | User-facing docs for the feature |

### Modified files

| Path | Change |
|---|---|
| `pkg/models/job.go` | Add `JobTypeHashGeneration` constant and `JobHashGenerationData` struct; wire into `UnmarshalData` |
| `pkg/worker/worker.go` | Add `fingerprintService *fingerprints.Service` field, initialize in `New()`, register in `processFuncs` map |
| `pkg/worker/scan.go` | Add end-of-scan `EnsureHashGenerationJob` call; add move reconciliation phase between walk and parallel processing |
| `pkg/worker/scan_unified.go` | Invalidate fingerprints when file is rescanned due to size/mtime change |
| `pkg/worker/monitor.go` | Add move detection logic to `processPendingEvents`; call `EnsureHashGenerationJob` at end of batch |
| `pkg/CLAUDE.md` | Remove "file identity not preserved across renames" limitation; add fingerprint invalidation gotcha |
| `website/docs/supported-formats.md` | Note that file fingerprinting is supported |
| `website/docs/metadata.md` | Brief mention and cross-link to new file-fingerprints.md page |

---

## Task 1: Fingerprint package — `ComputeSHA256`

**Files:**
- Create: `pkg/fingerprint/fingerprint.go`
- Create: `pkg/fingerprint/fingerprint_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/fingerprint/fingerprint_test.go`:

```go
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeSHA256_FixedContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	content := []byte("hello shisho")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	expected := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expected[:])

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expectedHex, got)
}

func TestComputeSHA256_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	// sha256 of empty content
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestComputeSHA256_LargeFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	f, err := os.Create(path)
	require.NoError(t, err)

	// Write 5 MB of pseudo-random data.
	r := rand.New(rand.NewSource(42))
	buf := make([]byte, 1024*1024)
	h := sha256.New()
	for i := 0; i < 5; i++ {
		_, _ = r.Read(buf)
		_, err := f.Write(buf)
		require.NoError(t, err)
		_, _ = h.Write(buf)
	}
	require.NoError(t, f.Close())
	expectedHex := hex.EncodeToString(h.Sum(nil))

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expectedHex, got)
}

func TestComputeSHA256_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := ComputeSHA256(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
}

// Sanity: ComputeSHA256 must match crypto/sha256 over the same bytes.
func TestComputeSHA256_MatchesStdlib(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sanity.bin")
	content := []byte("the quick brown fox jumps over the lazy dog")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	expected := hex.EncodeToString(h.Sum(nil))

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/fingerprint/...`
Expected: FAIL — `pkg/fingerprint/fingerprint.go` does not exist, `undefined: ComputeSHA256`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/fingerprint/fingerprint.go`:

```go
// Package fingerprint computes content fingerprints for files.
//
// For the MVP this is sha256 over the raw file bytes, used for exact-match
// move/rename detection and future dedup work. The package is structured so
// additional algorithms (pHash, SimHash, Chromaprint, TLSH, etc.) can be added
// as siblings to ComputeSHA256 without callers needing to know which one
// applies to which file type.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"
)

// AlgorithmSHA256 is the algorithm identifier stored in file_fingerprints.value's
// companion column for exact-content hashes.
const AlgorithmSHA256 = "sha256"

// ComputeSHA256 returns the lowercase hex-encoded sha256 of the file's contents.
// It streams the file in fixed-size chunks so it can handle multi-GB files
// without loading them into memory.
func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open file for sha256")
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(err, "read file for sha256")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/fingerprint/... -v`
Expected: PASS — all 5 tests.

- [ ] **Step 5: Run go vet and build**

Run: `go vet ./pkg/fingerprint/... && go build ./pkg/fingerprint/...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add pkg/fingerprint/
git commit -m "[Backend] Add pkg/fingerprint package with ComputeSHA256"
```

---

## Task 2: Database migration — `file_fingerprints` table

**Files:**
- Create: `pkg/migrations/20260412000000_add_file_fingerprints.go`

Read `pkg/migrations/20260113000001_add_file_identifiers.go` first as a reference — it creates a similar child-of-files table.

- [ ] **Step 1: Write the migration**

Create `pkg/migrations/20260412000000_add_file_fingerprints.go`:

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
			CREATE TABLE file_fingerprints (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				algorithm TEXT NOT NULL,
				value TEXT NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// One fingerprint per algorithm per file.
		_, err = db.Exec(`
			CREATE UNIQUE INDEX ux_file_fingerprints_file_algorithm
				ON file_fingerprints (file_id, algorithm)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Fast lookup for "find file by hash" (move detection).
		_, err = db.Exec(`
			CREATE INDEX ix_file_fingerprints_algorithm_value
				ON file_fingerprints (algorithm, value)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS file_fingerprints")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Run migration against a fresh test DB to verify it applies**

Run: `mise db:rollback 2>/dev/null; mise db:migrate`
Expected: migration applies cleanly, no errors. (If the target DB is already at this migration, rollback first.)

- [ ] **Step 3: Verify the table exists and has expected indexes**

Run: `sqlite3 tmp/data.sqlite ".schema file_fingerprints"`
Expected: CREATE TABLE output matching the migration, plus the two indexes.

- [ ] **Step 4: Test rollback and re-apply**

Run: `mise db:rollback && mise db:migrate`
Expected: table dropped and recreated cleanly.

- [ ] **Step 5: Commit**

```bash
git add pkg/migrations/20260412000000_add_file_fingerprints.go
git commit -m "[Backend] Add file_fingerprints table migration"
```

---

## Task 3: `FileFingerprint` Bun model

**Files:**
- Create: `pkg/models/file_fingerprint.go`

Read `pkg/models/file-identifier.go` and `pkg/models/file.go` first to see the Bun struct tag patterns in use.

- [ ] **Step 1: Write the model**

Create `pkg/models/file_fingerprint.go`:

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Fingerprint algorithm identifiers stored in FileFingerprint.Algorithm.
const (
	// FingerprintAlgorithmSHA256 is the exact-content sha256 hash over the
	// file's raw bytes. Used for move/rename detection.
	FingerprintAlgorithmSHA256 = "sha256"
)

// FileFingerprint is a content fingerprint for a file. A single file may have
// multiple fingerprints, one per algorithm (e.g. sha256 for exact matching,
// phash for cover similarity, simhash for text similarity).
type FileFingerprint struct {
	bun.BaseModel `bun:"table:file_fingerprints,alias:ffp" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	FileID    int       `bun:",nullzero" json:"file_id"`
	Algorithm string    `bun:",nullzero" json:"algorithm"`
	Value     string    `bun:",nullzero" json:"value"`

	File *File `bun:"rel:belongs-to" json:"-"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./pkg/models/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/models/file_fingerprint.go
git commit -m "[Backend] Add FileFingerprint Bun model"
```

---

## Task 4: `pkg/fingerprints/` service — CRUD operations

**Files:**
- Create: `pkg/fingerprints/service.go`
- Create: `pkg/fingerprints/service_test.go`

Read `pkg/genres/service.go` first as a reference for a simple service with DB operations.

- [ ] **Step 1: Write the service tests**

Create `pkg/fingerprints/service_test.go`:

```go
package fingerprints_test

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestService_Insert(t *testing.T) {
	t.Parallel()
	db := testutils.NewTestDB(t)
	svc := fingerprints.NewService(db)

	// Need a file row to satisfy the FK.
	file := testutils.InsertTestFile(t, db, "/lib/foo.epub")

	err := svc.Insert(context.Background(), file.ID, models.FingerprintAlgorithmSHA256, "abc123")
	require.NoError(t, err)

	// Insert again with same (file_id, algorithm) — should be a no-op under ON CONFLICT DO NOTHING.
	err = svc.Insert(context.Background(), file.ID, models.FingerprintAlgorithmSHA256, "abc123")
	require.NoError(t, err)

	// Verify only one row exists.
	var count int
	count, err = svc.CountForFile(context.Background(), file.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestService_FindByHash(t *testing.T) {
	t.Parallel()
	db := testutils.NewTestDB(t)
	svc := fingerprints.NewService(db)

	file := testutils.InsertTestFile(t, db, "/lib/foo.epub")
	require.NoError(t, svc.Insert(context.Background(), file.ID, models.FingerprintAlgorithmSHA256, "deadbeef"))

	matches, err := svc.FindFilesByHash(context.Background(), file.LibraryID, models.FingerprintAlgorithmSHA256, "deadbeef")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, file.ID, matches[0].ID)

	// Different library → no matches.
	matches, err = svc.FindFilesByHash(context.Background(), file.LibraryID+999, models.FingerprintAlgorithmSHA256, "deadbeef")
	require.NoError(t, err)
	require.Empty(t, matches)

	// Different hash → no matches.
	matches, err = svc.FindFilesByHash(context.Background(), file.LibraryID, models.FingerprintAlgorithmSHA256, "notfound")
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestService_DeleteForFile(t *testing.T) {
	t.Parallel()
	db := testutils.NewTestDB(t)
	svc := fingerprints.NewService(db)

	file := testutils.InsertTestFile(t, db, "/lib/foo.epub")
	require.NoError(t, svc.Insert(context.Background(), file.ID, models.FingerprintAlgorithmSHA256, "abc"))
	require.NoError(t, svc.DeleteForFile(context.Background(), file.ID))

	count, err := svc.CountForFile(context.Background(), file.ID)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestService_ListFilesMissingAlgorithm(t *testing.T) {
	t.Parallel()
	db := testutils.NewTestDB(t)
	svc := fingerprints.NewService(db)

	hasFile := testutils.InsertTestFile(t, db, "/lib/a.epub")
	lackingFile := testutils.InsertTestFile(t, db, "/lib/b.epub")
	require.NoError(t, svc.Insert(context.Background(), hasFile.ID, models.FingerprintAlgorithmSHA256, "h"))

	ids, err := svc.ListFilesMissingAlgorithm(context.Background(), hasFile.LibraryID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Equal(t, []int{lackingFile.ID}, ids)
}
```

**Note:** `testutils.NewTestDB` and `testutils.InsertTestFile` may not exist yet — look at existing `*_test.go` files in `pkg/books/` or `pkg/jobs/` for how test DBs are set up in this codebase. If there's no shared helper, create inline helpers at the top of `service_test.go` that run migrations against an in-memory sqlite DB and insert a minimal library+book+file row. Use Bun's `NewDB(sql.Open("sqlite", ":memory:"))` pattern.

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./pkg/fingerprints/...`
Expected: FAIL — `pkg/fingerprints/service.go` does not exist.

- [ ] **Step 3: Write the service**

Create `pkg/fingerprints/service.go`:

```go
// Package fingerprints provides CRUD operations on the file_fingerprints table.
//
// A file_fingerprint row stores one algorithm's fingerprint for one file.
// The MVP only writes sha256 rows for exact-content move/rename detection,
// but the schema is already shape-compatible with future fuzzy algorithms
// (cover pHash, text SimHash, Chromaprint, etc.) so they can reuse the same
// table, service, and generation job.
package fingerprints

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

// Insert upserts a fingerprint for (file_id, algorithm). If a row already
// exists for this pair the call is a no-op (ON CONFLICT DO NOTHING) — callers
// that need to replace a stale value should DeleteForFile first.
func (svc *Service) Insert(ctx context.Context, fileID int, algorithm, value string) error {
	fp := &models.FileFingerprint{
		FileID:    fileID,
		Algorithm: algorithm,
		Value:     value,
		CreatedAt: time.Now(),
	}
	_, err := svc.db.
		NewInsert().
		Model(fp).
		On("CONFLICT (file_id, algorithm) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// FindFilesByHash returns all files in the library whose fingerprint for the
// given algorithm matches value. Used for move detection.
func (svc *Service) FindFilesByHash(ctx context.Context, libraryID int, algorithm, value string) ([]*models.File, error) {
	var files []*models.File
	err := svc.db.
		NewSelect().
		Model(&files).
		Join("JOIN file_fingerprints AS ffp ON ffp.file_id = f.id").
		Where("ffp.algorithm = ?", algorithm).
		Where("ffp.value = ?", value).
		Where("f.library_id = ?", libraryID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return files, nil
}

// DeleteForFile removes all fingerprints for a file. Called when a file's
// content changes (size/mtime mismatch during rescan) so the next hash
// generation job recomputes a fresh fingerprint.
func (svc *Service) DeleteForFile(ctx context.Context, fileID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.FileFingerprint)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// CountForFile returns how many fingerprints exist for a file (across all algorithms).
func (svc *Service) CountForFile(ctx context.Context, fileID int) (int, error) {
	count, err := svc.db.
		NewSelect().
		Model((*models.FileFingerprint)(nil)).
		Where("file_id = ?", fileID).
		Count(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return count, nil
}

// ListFilesMissingAlgorithm returns IDs of files in the library that do not
// yet have a fingerprint for the given algorithm. Used by the hash generation
// job to determine what work needs doing.
func (svc *Service) ListFilesMissingAlgorithm(ctx context.Context, libraryID int, algorithm string) ([]int, error) {
	var ids []int
	err := svc.db.
		NewSelect().
		Model((*models.File)(nil)).
		Column("f.id").
		Join("LEFT JOIN file_fingerprints AS ffp ON ffp.file_id = f.id AND ffp.algorithm = ?", algorithm).
		Where("f.library_id = ?", libraryID).
		Where("ffp.id IS NULL").
		Scan(ctx, &ids)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return ids, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./pkg/fingerprints/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/fingerprints/
git commit -m "[Backend] Add fingerprints service with CRUD operations"
```

---

## Task 5: Job type and data struct for hash generation

**Files:**
- Modify: `pkg/models/job.go`

Read `pkg/models/job.go` first to see the existing patterns for `JobType*` constants and `UnmarshalData`.

- [ ] **Step 1: Add the job type constant and data struct**

Edit `pkg/models/job.go`. Add `JobTypeHashGeneration` to the existing job type constants:

```go
const (
	JobTypeExport         = "export"
	JobTypeScan           = "scan"
	JobTypeBulkDownload   = "bulk_download"
	JobTypeHashGeneration = "hash_generation"
)
```

Add a case for `JobTypeHashGeneration` in `UnmarshalData`:

```go
func (job *Job) UnmarshalData() error {
	switch job.Type {
	case JobTypeExport:
		job.DataParsed = &JobExportData{}
	case JobTypeScan:
		job.DataParsed = &JobScanData{}
	case JobTypeBulkDownload:
		job.DataParsed = &JobBulkDownloadData{}
	case JobTypeHashGeneration:
		job.DataParsed = &JobHashGenerationData{}
	}

	err := json.Unmarshal([]byte(job.Data), job.DataParsed)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
```

Add the data struct at the bottom of the file alongside the other `Job*Data` types:

```go
// JobHashGenerationData is the payload for a hash generation job.
// The job processes all files in the given library that do not yet have
// a sha256 fingerprint in file_fingerprints.
type JobHashGenerationData struct {
	LibraryID int `json:"library_id"`
}
```

Update the `DataParsed` TypeScript union tag to include the new type. The existing line reads:

```go
DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData | JobBulkDownloadData"`
```

Change it to:

```go
DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData | JobBulkDownloadData | JobHashGenerationData"`
```

- [ ] **Step 2: Run mise tygo to regenerate types**

Run: `mise tygo`
Expected: either updates `app/types/generated/` or prints "skipping, outputs are up-to-date". Either is fine. If it prints generated file paths, note that those are gitignored and cannot be committed.

- [ ] **Step 3: Verify build**

Run: `go build ./pkg/models/...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add pkg/models/job.go
git commit -m "[Backend] Add JobTypeHashGeneration job type and data struct"
```

---

## Task 6: Hash generation job handler

**Files:**
- Create: `pkg/worker/hash_generation.go`
- Create: `pkg/worker/hash_generation_test.go`

Read `pkg/worker/worker.go` (lines 39-145) to see the Worker struct and how services are initialized. Read `pkg/worker/scan.go` (lines 240-300) to see the job handler signature and how `jobLog` is used.

- [ ] **Step 1: Write the handler tests**

Create `pkg/worker/hash_generation_test.go`:

```go
package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/require"
)

// TestHashGenerationJob_ComputesMissingFingerprints writes two files to disk,
// inserts file rows for them with no fingerprints, runs the hash generation
// job, and verifies fingerprints were created with the correct sha256 values.
func TestHashGenerationJob_ComputesMissingFingerprints(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.epub")
	path2 := filepath.Join(dir, "b.epub")
	require.NoError(t, os.WriteFile(path1, []byte("content-a"), 0o644))
	require.NoError(t, os.WriteFile(path2, []byte("content-b"), 0o644))

	library := harness.InsertLibrary(t, dir)
	file1 := harness.InsertFile(t, library.ID, path1)
	file2 := harness.InsertFile(t, library.ID, path2)

	expected1, err := fingerprint.ComputeSHA256(path1)
	require.NoError(t, err)
	expected2, err := fingerprint.ComputeSHA256(path2)
	require.NoError(t, err)

	job := harness.CreateJob(t, models.JobTypeHashGeneration, &models.JobHashGenerationData{LibraryID: library.ID})
	require.NoError(t, harness.Worker.ProcessHashGenerationJob(context.Background(), job, harness.JobLog(t, job)))

	got1 := harness.GetSHA256(t, file1.ID)
	got2 := harness.GetSHA256(t, file2.ID)
	require.Equal(t, expected1, got1)
	require.Equal(t, expected2, got2)
}

// TestHashGenerationJob_Idempotent verifies running the job twice produces
// the same result and no duplicate rows.
func TestHashGenerationJob_Idempotent(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.epub")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))

	library := harness.InsertLibrary(t, dir)
	file := harness.InsertFile(t, library.ID, path)

	job := harness.CreateJob(t, models.JobTypeHashGeneration, &models.JobHashGenerationData{LibraryID: library.ID})
	require.NoError(t, harness.Worker.ProcessHashGenerationJob(context.Background(), job, harness.JobLog(t, job)))
	require.NoError(t, harness.Worker.ProcessHashGenerationJob(context.Background(), job, harness.JobLog(t, job)))

	require.Equal(t, 1, harness.CountFingerprints(t, file.ID))
}

// TestHashGenerationJob_MissingFileContinues verifies that a file which has
// been deleted from disk is logged but does not fail the job, and the job
// still processes other files successfully.
func TestHashGenerationJob_MissingFileContinues(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing.epub")
	okPath := filepath.Join(dir, "ok.epub")
	require.NoError(t, os.WriteFile(okPath, []byte("ok"), 0o644))

	library := harness.InsertLibrary(t, dir)
	harness.InsertFile(t, library.ID, missingPath) // no file on disk
	okFile := harness.InsertFile(t, library.ID, okPath)

	job := harness.CreateJob(t, models.JobTypeHashGeneration, &models.JobHashGenerationData{LibraryID: library.ID})
	require.NoError(t, harness.Worker.ProcessHashGenerationJob(context.Background(), job, harness.JobLog(t, job)))

	// OK file was processed.
	require.NotEmpty(t, harness.GetSHA256(t, okFile.ID))
}

// TestEnsureHashGenerationJob_Dedupes verifies that calling
// EnsureHashGenerationJob twice for the same library while the first
// is pending does not create a duplicate job.
func TestEnsureHashGenerationJob_Dedupes(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	library := harness.InsertLibrary(t, dir)

	require.NoError(t, worker.EnsureHashGenerationJob(context.Background(), harness.JobService, library.ID))
	require.NoError(t, worker.EnsureHashGenerationJob(context.Background(), harness.JobService, library.ID))

	// Only one pending hash generation job for this library.
	jobs := harness.ListPendingJobs(t, models.JobTypeHashGeneration, library.ID)
	require.Len(t, jobs, 1)
}
```

**Note:** `testutils.NewWorkerHarness`, `InsertLibrary`, `InsertFile`, `CreateJob`, `JobLog`, `GetSHA256`, `CountFingerprints`, `ListPendingJobs` may not exist yet. Look for existing worker tests (e.g. `pkg/worker/scan_cache_test.go`, `pkg/worker/scan_orphans_test.go`) to see how the worker is wired up in tests. If no shared harness exists, either:
(a) build one inline at the top of this test file using Bun sqlite in-memory and real service instances, or
(b) create a minimal `pkg/testutils/worker.go` that sets up a full worker and exposes the helpers this plan references. If you create it, make it small and scoped to what these tests need.

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./pkg/worker/... -run HashGeneration`
Expected: FAIL — `ProcessHashGenerationJob`/`EnsureHashGenerationJob` not defined.

- [ ] **Step 3: Write the handler**

Create `pkg/worker/hash_generation.go`:

```go
package worker

import (
	"context"
	"os"
	"runtime"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// ProcessHashGenerationJob computes sha256 fingerprints for every file in the
// target library that does not yet have one. This is both the initial backfill
// (first run hashes every file) and the ongoing population mechanism (scan and
// monitor enqueue this job whenever new files land without fingerprints).
//
// Failures on individual files (missing on disk, permission denied, read error)
// are logged and the job continues. The job only returns an error on
// infrastructure failures (DB unavailable, etc.).
func (w *Worker) ProcessHashGenerationJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	data, ok := job.DataParsed.(*models.JobHashGenerationData)
	if !ok {
		return errors.New("hash generation job: invalid job data")
	}

	jobLog.Info("processing hash generation job", logger.Data{"library_id": data.LibraryID})

	fileIDs, err := w.fingerprintService.ListFilesMissingAlgorithm(ctx, data.LibraryID, models.FingerprintAlgorithmSHA256)
	if err != nil {
		return errors.Wrap(err, "list files missing sha256")
	}

	total := len(fileIDs)
	if total == 0 {
		jobLog.Info("no files need hashing", nil)
		return nil
	}
	jobLog.Info("hashing files", logger.Data{"count": total})

	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}

	type task struct{ fileID int }
	tasks := make(chan task, workers)

	var (
		done  int
		doneM sync.Mutex
	)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for t := range tasks {
				if ctx.Err() != nil {
					return
				}
				file, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &t.fileID})
				if err != nil {
					jobLog.Warn("failed to retrieve file for hashing", logger.Data{"file_id": t.fileID, "error": err.Error()})
					continue
				}
				if _, err := os.Stat(file.Filepath); err != nil {
					jobLog.Warn("file missing on disk, skipping", logger.Data{"file_id": t.fileID, "path": file.Filepath, "error": err.Error()})
					continue
				}
				hash, err := fingerprint.ComputeSHA256(file.Filepath)
				if err != nil {
					jobLog.Warn("failed to compute sha256", logger.Data{"file_id": t.fileID, "path": file.Filepath, "error": err.Error()})
					continue
				}
				if err := w.fingerprintService.Insert(ctx, t.fileID, models.FingerprintAlgorithmSHA256, hash); err != nil {
					jobLog.Warn("failed to insert fingerprint", logger.Data{"file_id": t.fileID, "error": err.Error()})
					continue
				}
				doneM.Lock()
				done++
				if done%50 == 0 || done == total {
					jobLog.Info("hashing progress", logger.Data{"done": done, "total": total})
				}
				doneM.Unlock()
			}
		}()
	}
	for _, id := range fileIDs {
		tasks <- task{fileID: id}
	}
	close(tasks)
	wg.Wait()

	jobLog.Info("finished hash generation job", logger.Data{"done": done, "total": total})
	return nil
}

// EnsureHashGenerationJob creates a pending hash generation job for the
// library if one does not already exist. It's safe to call from anywhere
// that discovers files without fingerprints (end of scan, end of monitor
// batch). Called repeatedly is cheap and idempotent.
func EnsureHashGenerationJob(ctx context.Context, jobService *jobs.Service, libraryID int) error {
	existing, err := jobService.ListJobs(ctx, jobs.ListJobsOptions{
		Types:     []string{models.JobTypeHashGeneration},
		Statuses:  []string{models.JobStatusPending, models.JobStatusInProgress},
		LibraryID: &libraryID,
	})
	if err != nil {
		return errors.Wrap(err, "list existing hash generation jobs")
	}
	if len(existing) > 0 {
		return nil
	}

	data := models.JobHashGenerationData{LibraryID: libraryID}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = jobService.CreateJob(ctx, jobs.CreateJobOptions{
		Type:      models.JobTypeHashGeneration,
		Data:      string(dataBytes),
		LibraryID: &libraryID,
	})
	if err != nil {
		return errors.Wrap(err, "create hash generation job")
	}
	return nil
}
```

**Note:** The exact `jobs.Service` API (`ListJobs`, `CreateJob`, option struct names) may differ. Read `pkg/jobs/service.go` and adjust calls to match the real API. The algorithm is correct; the method names may need tweaking.

Also add the import for `books` if not already in the file. The `books.RetrieveFileOptions{ID: &t.fileID}` call assumes `RetrieveFile` accepts an ID — verify from `pkg/books/service.go` and adjust if the option struct uses a different field name.

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./pkg/worker/... -run HashGeneration -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/hash_generation.go pkg/worker/hash_generation_test.go
# Also commit any new testutils file if you created one.
git commit -m "[Backend] Add hash generation job handler"
```

---

## Task 7: Wire fingerprint service into Worker and register job handler

**Files:**
- Modify: `pkg/worker/worker.go`

Read `pkg/worker/worker.go` first — specifically the `Worker` struct (around lines 39-73), the `New()` constructor (lines 75-137), and the `processFuncs` map initialization (lines 121-124).

- [ ] **Step 1: Add field to Worker struct**

Add `fingerprintService` alongside the other service fields in the `Worker` struct:

```go
type Worker struct {
	// ... existing fields ...

	bookService        *books.Service
	chapterService     *chapters.Service
	fingerprintService *fingerprints.Service // NEW
	genreService       *genres.Service
	// ... rest of fields ...
}
```

Add the import:

```go
import (
	// ... existing imports ...
	"github.com/shishobooks/shisho/pkg/fingerprints"
)
```

- [ ] **Step 2: Initialize in `New()` constructor**

Locate where other services are initialized in `New()` and add fingerprint service initialization in the same pattern. The existing services are passed via the `Services` struct or initialized directly — match whichever pattern is in use.

If services are passed in via a `Services` struct, add `Fingerprints *fingerprints.Service` to that struct and wire it through `cmd/api/main.go` (grep for other services like `chapterService` to find the construction site).

If services are initialized directly in `New()`, add:

```go
fingerprintService := fingerprints.NewService(db)
// ...
w := &Worker{
	// ...
	fingerprintService: fingerprintService,
	// ...
}
```

- [ ] **Step 3: Register in `processFuncs` map**

Find the line that initializes the `processFuncs` map (around line 121):

```go
w.processFuncs = map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error{
	models.JobTypeScan:         w.ProcessScanJob,
	models.JobTypeBulkDownload: w.ProcessBulkDownloadJob,
}
```

Add the new entry:

```go
w.processFuncs = map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error{
	models.JobTypeScan:           w.ProcessScanJob,
	models.JobTypeBulkDownload:   w.ProcessBulkDownloadJob,
	models.JobTypeHashGeneration: w.ProcessHashGenerationJob,
}
```

- [ ] **Step 4: Wire in main.go if needed**

Run: `grep -n "fingerprintService\|NewService" cmd/api/main.go`

If the worker constructor needs to be updated at the call site in `cmd/api/main.go`, update it to construct and pass the fingerprint service.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Run the tests**

Run: `go test ./pkg/worker/... -run HashGeneration`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/worker/worker.go cmd/api/main.go
git commit -m "[Backend] Wire fingerprint service into worker and register hash generation job"
```

---

## Task 8: Scan — call `EnsureHashGenerationJob` at end of scan

**Files:**
- Modify: `pkg/worker/scan.go`

This is the simpler of the two scan-related tasks. The reconciliation logic comes in Task 10.

Read `pkg/worker/scan.go` around `ProcessScanJob` (starting at line 240), specifically the end of the library processing loop (after orphan cleanup and search index updates).

- [ ] **Step 1: Write the test**

Add to an existing scan test file or create `pkg/worker/scan_hash_job_test.go`:

```go
package worker_test

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestScanJob_QueuesHashGenerationAtEnd verifies that after a scan completes,
// a hash generation job is queued for the library (for backfill / new files).
func TestScanJob_QueuesHashGenerationAtEnd(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	library := harness.InsertLibrary(t, t.TempDir())

	job := harness.CreateJob(t, models.JobTypeScan, &models.JobScanData{})
	job.LibraryID = &library.ID
	require.NoError(t, harness.Worker.ProcessScanJob(context.Background(), job, harness.JobLog(t, job)))

	pending := harness.ListPendingJobs(t, models.JobTypeHashGeneration, library.ID)
	require.Len(t, pending, 1, "expected exactly one pending hash generation job")
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./pkg/worker/... -run TestScanJob_QueuesHashGenerationAtEnd`
Expected: FAIL — no pending hash generation job.

- [ ] **Step 3: Add the call to `EnsureHashGenerationJob`**

In `pkg/worker/scan.go`, at the end of the per-library loop inside `ProcessScanJob` — after orphan cleanup, search index updates, and any other post-processing — add:

```go
// Queue async sha256 hash generation for files that still lack a fingerprint.
// Handles both initial backfill and newly-discovered files from this scan.
if err := EnsureHashGenerationJob(ctx, w.jobService, library.ID); err != nil {
	jobLog.Warn("failed to ensure hash generation job", logger.Data{"error": err.Error()})
}
```

Place this inside the `for _, library := range allLibraries` loop so it runs once per library, not once per scan job.

- [ ] **Step 4: Run the test and verify it passes**

Run: `go test ./pkg/worker/... -run TestScanJob_QueuesHashGenerationAtEnd -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_hash_job_test.go
git commit -m "[Backend] Queue hash generation job at end of scan"
```

---

## Task 9: Fingerprint invalidation on file change

**Files:**
- Modify: `pkg/worker/scan_unified.go`

When `scanFileByPath` detects that an existing file's size or mtime has changed (indicating out-of-band content modification), any stored sha256 becomes stale. We must delete the fingerprints for that file so the next hash generation job recomputes them.

Read `pkg/worker/scan_unified.go` around `scanFileByPath` (lines 207-278). The current code delegates to `scanFileByID` when a change is detected — we need to delete fingerprints before that delegation.

- [ ] **Step 1: Write the test**

Add to a new or existing scan test file:

```go
// TestScanFileByPath_InvalidatesFingerprintOnContentChange verifies that when
// a file's size or mtime changes, its stored fingerprints are deleted so the
// next hash generation job will recompute them.
func TestScanFileByPath_InvalidatesFingerprintOnContentChange(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.epub")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	library := harness.InsertLibrary(t, dir)
	file := harness.InsertFile(t, library.ID, path)
	harness.InsertFingerprint(t, file.ID, models.FingerprintAlgorithmSHA256, "stale-hash")

	// Simulate content change by rewriting the file with different size.
	require.NoError(t, os.WriteFile(path, []byte("new content that is longer"), 0o644))

	_, err := harness.Worker.ScanInternalForTest(context.Background(), worker.ScanOptions{
		FilePath:  path,
		LibraryID: library.ID,
	}, nil)
	require.NoError(t, err)

	require.Equal(t, 0, harness.CountFingerprints(t, file.ID), "stale fingerprint should have been deleted")
}
```

**Note:** `ScanInternalForTest` is a hypothetical test-only accessor. If `scanInternal` is unexported, add an exported wrapper in a `_test.go` file (`export_test.go` pattern) or call `ProcessScanJob` instead.

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./pkg/worker/... -run TestScanFileByPath_InvalidatesFingerprint`
Expected: FAIL.

- [ ] **Step 3: Add invalidation in `scanFileByPath`**

In `pkg/worker/scan_unified.go`, within `scanFileByPath`, find the branch that detects a changed file and delegates to `scanFileByID`. Currently it looks like:

```go
if !opts.ForceRefresh && existingFile.FileModifiedAt != nil {
	stat, err := os.Stat(opts.FilePath)
	if err == nil && stat.Size() == existingFile.FilesizeBytes &&
		stat.ModTime().Truncate(time.Second).Equal(existingFile.FileModifiedAt.Truncate(time.Second)) {
		// File unchanged — skip re-parsing entirely
		return &ScanResult{File: existingFile}, nil
	}
}
// File changed or ForceRefresh — delegate to scanFileByID
return w.scanFileByID(ctx, ScanOptions{...}, cache)
```

Add fingerprint invalidation immediately before the delegation to `scanFileByID`:

```go
// File changed or ForceRefresh — invalidate stale fingerprints so the next
// hash generation job recomputes them against the new content.
if err := w.fingerprintService.DeleteForFile(ctx, existingFile.ID); err != nil {
	return nil, errors.Wrap(err, "invalidate stale fingerprints")
}

// File changed or ForceRefresh — delegate to scanFileByID
return w.scanFileByID(ctx, ScanOptions{
	FileID:       existingFile.ID,
	ForceRefresh: opts.ForceRefresh,
	SkipPlugins:  opts.SkipPlugins,
	JobLog:       opts.JobLog,
}, cache)
```

Do the same for the fallback DB-query path later in the same function where it also delegates to `scanFileByID` (look for the second `return w.scanFileByID(...)` in `scanFileByPath`).

- [ ] **Step 4: Run the test and verify it passes**

Run: `go test ./pkg/worker/... -run TestScanFileByPath_InvalidatesFingerprint -v`
Expected: PASS.

- [ ] **Step 5: Run the full worker test suite to check nothing else regressed**

Run: `go test ./pkg/worker/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/
git commit -m "[Backend] Invalidate stale fingerprints when file content changes"
```

---

## Task 10: Scan — move reconciliation phase

**Files:**
- Modify: `pkg/worker/scan.go`

This is the most involved scan change. We add a reconciliation phase between the walk and parallel processing that matches orphans to new files by `(size, sha256)`.

Read `pkg/worker/scan.go` around `ProcessScanJob` — specifically the file walk phase (lines 288-357) and the parallel processing phase (lines 364-400). Also read the orphan identification logic in `pkg/worker/scan_orphans.go`.

- [ ] **Step 1: Write the test**

Create `pkg/worker/scan_move_reconciliation_test.go`:

```go
package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestScanReconciliation_DetectsMove verifies that a file row whose path is
// missing on disk is repurposed as the target of a new path with the same
// sha256, rather than being deleted and recreated.
func TestScanReconciliation_DetectsMove(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "book.epub")
	newPath := filepath.Join(dir, "new", "book.epub")
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))

	// Content exists only at the new path — the old path is gone.
	content := []byte("epub content")
	require.NoError(t, os.WriteFile(newPath, content, 0o644))

	library := harness.InsertLibrary(t, dir)
	oldFile := harness.InsertFile(t, library.ID, oldPath)
	hash, err := fingerprint.ComputeSHA256(newPath)
	require.NoError(t, err)
	harness.InsertFingerprint(t, oldFile.ID, models.FingerprintAlgorithmSHA256, hash)
	harness.SetFileSize(t, oldFile.ID, int64(len(content)))

	job := harness.CreateJob(t, models.JobTypeScan, &models.JobScanData{})
	job.LibraryID = &library.ID
	require.NoError(t, harness.Worker.ProcessScanJob(context.Background(), job, harness.JobLog(t, job)))

	// Old file row should now point at the new path (not deleted).
	updated := harness.GetFile(t, oldFile.ID)
	require.NotNil(t, updated, "old file row should still exist")
	require.Equal(t, newPath, updated.Filepath)

	// No duplicate row should have been created for the new path.
	byPath := harness.FindFileByPath(t, library.ID, newPath)
	require.Equal(t, oldFile.ID, byPath.ID)
}

// TestScanReconciliation_SizeMismatch_DoesNotMatch verifies that size-based
// pruning skips hashing when sizes differ.
func TestScanReconciliation_SizeMismatch_DoesNotMatch(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "book.epub")
	newPath := filepath.Join(dir, "new", "book.epub")
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("totally different length content"), 0o644))

	library := harness.InsertLibrary(t, dir)
	oldFile := harness.InsertFile(t, library.ID, oldPath)
	harness.InsertFingerprint(t, oldFile.ID, models.FingerprintAlgorithmSHA256, "unrelated-hash")
	harness.SetFileSize(t, oldFile.ID, 5) // different size

	job := harness.CreateJob(t, models.JobTypeScan, &models.JobScanData{})
	job.LibraryID = &library.ID
	require.NoError(t, harness.Worker.ProcessScanJob(context.Background(), job, harness.JobLog(t, job)))

	// Old file should be deleted (orphan), new file should exist with a new row.
	require.Nil(t, harness.GetFile(t, oldFile.ID))
	byPath := harness.FindFileByPath(t, library.ID, newPath)
	require.NotNil(t, byPath)
	require.NotEqual(t, oldFile.ID, byPath.ID)
}

// TestScanReconciliation_NoFingerprint_Deletes verifies that an orphan
// without a sha256 fingerprint is still deleted (no reconciliation possible).
func TestScanReconciliation_NoFingerprint_Deletes(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "book.epub")
	newPath := filepath.Join(dir, "new", "book.epub")
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("content"), 0o644))

	library := harness.InsertLibrary(t, dir)
	oldFile := harness.InsertFile(t, library.ID, oldPath) // no fingerprint inserted

	job := harness.CreateJob(t, models.JobTypeScan, &models.JobScanData{})
	job.LibraryID = &library.ID
	require.NoError(t, harness.Worker.ProcessScanJob(context.Background(), job, harness.JobLog(t, job)))

	require.Nil(t, harness.GetFile(t, oldFile.ID), "orphan without fingerprint should be deleted")
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./pkg/worker/... -run TestScanReconciliation`
Expected: FAIL — move is not detected; old file deleted and new file created instead.

- [ ] **Step 3: Add reconciliation phase to `ProcessScanJob`**

In `pkg/worker/scan.go`, find the section between:
- The file walk (which populates `filesToScan` and processes which paths were seen)
- The parallel processing worker pool

The walk produces two things the reconciliation needs:
- The set of paths seen on disk during the walk
- The pre-loaded `cache.knownFiles` map (DB rows)

Add a reconciliation phase inside the per-library block, after the walk and before the parallel processing worker pool spins up:

```go
// --- Move reconciliation ---
// Before handing off to the scan workers, check whether any DB rows whose
// paths weren't seen on disk (orphans) match a newly-discovered path by
// (size, sha256). If so, repurpose the orphan row's filepath instead of
// deleting it and creating a new book. This is the safety-net path for
// renames that happened while the monitor wasn't running.
type orphanHashEntry struct {
	fileID int
	hash   string
}
// Bucket orphans by size → []orphanHashEntry. Orphans without an sha256
// fingerprint are excluded (can't match them).
orphansBySize := make(map[int64][]orphanHashEntry)
for _, f := range allFiles {
	if f.FileRole == models.FileRoleSupplement {
		continue
	}
	if _, seen := seenPaths[f.Filepath]; seen {
		continue
	}
	// Load sha256 fingerprint for this file if any.
	fps, err := w.fingerprintService.ListForFile(ctx, f.ID, models.FingerprintAlgorithmSHA256)
	if err != nil || len(fps) == 0 {
		continue
	}
	orphansBySize[f.FilesizeBytes] = append(orphansBySize[f.FilesizeBytes], orphanHashEntry{
		fileID: f.ID,
		hash:   fps[0].Value,
	})
}

if len(orphansBySize) > 0 {
	jobLog.Info("move reconciliation: candidate orphans", logger.Data{"count": len(orphansBySize)})
	// movedOrphanIDs tracks orphan file IDs whose paths were repurposed —
	// these should NOT be deleted by the orphan cleanup at the end.
	movedOrphanIDs := make(map[int]struct{})
	// For each path that is new (not in cache.knownFiles), if its on-disk
	// size matches an orphan bucket, compute sha256 and check for match.
	for i := 0; i < len(filesToScan); i++ {
		path := filesToScan[i]
		if cache.GetKnownFile(path) != nil {
			continue // known file, not a candidate
		}
		stat, err := os.Stat(path)
		if err != nil || stat.IsDir() {
			continue
		}
		candidates, ok := orphansBySize[stat.Size()]
		if !ok || len(candidates) == 0 {
			continue
		}
		hash, err := fingerprint.ComputeSHA256(path)
		if err != nil {
			jobLog.Warn("reconciliation: failed to compute sha256", logger.Data{"path": path, "error": err.Error()})
			continue
		}
		// Find a matching orphan in the bucket.
		var matched *orphanHashEntry
		for j := range candidates {
			if candidates[j].hash == hash {
				matched = &candidates[j]
				break
			}
		}
		if matched == nil {
			continue
		}
		// Update the orphan file's filepath to the new path.
		orphan, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &matched.fileID})
		if err != nil {
			jobLog.Warn("reconciliation: failed to retrieve orphan", logger.Data{"file_id": matched.fileID, "error": err.Error()})
			continue
		}
		orphan.Filepath = path
		if err := w.bookService.UpdateFile(ctx, orphan, books.UpdateFileOptions{Columns: []string{"filepath"}}); err != nil {
			jobLog.Warn("reconciliation: failed to update filepath", logger.Data{"file_id": matched.fileID, "error": err.Error()})
			continue
		}
		jobLog.Info("reconciliation: detected move", logger.Data{"file_id": matched.fileID, "new_path": path})
		movedOrphanIDs[matched.fileID] = struct{}{}
		// Remove the matched orphan from the bucket so it can't match another file.
		remaining := candidates[:0]
		for j := range candidates {
			if candidates[j].fileID != matched.fileID {
				remaining = append(remaining, candidates[j])
			}
		}
		orphansBySize[stat.Size()] = remaining
		// Add the orphan (with updated path) to the cache so downstream scan
		// logic treats it as known.
		cache.AddKnownFile(orphan)
	}
	// Make movedOrphanIDs available to orphan cleanup so they're not deleted.
	cache.SetMovedOrphanIDs(movedOrphanIDs)
}
```

**Notes on required new methods referenced above:**

1. `fingerprintService.ListForFile(ctx, fileID, algorithm) ([]*models.FileFingerprint, error)` — add this to `pkg/fingerprints/service.go` if it doesn't exist. Simple `NewSelect()` filtered by `file_id` and `algorithm`.

2. `cache.AddKnownFile(file *models.File)` — add to `pkg/worker/scan_cache.go`. Mutex-protected insert into the `knownFiles` map keyed by `file.Filepath`.

3. `cache.SetMovedOrphanIDs(ids map[int]struct{})` — add to `pkg/worker/scan_cache.go`. A simple setter for a map the orphan cleanup can consult.

4. `orphan cleanup must consult movedOrphanIDs` — modify `pkg/worker/scan_orphans.go` `cleanupOrphanedFiles` (or equivalent) to skip file IDs in that set. They've been repurposed, not orphaned.

5. `seenPaths` — make sure the walk populates a `map[string]struct{}` of all paths discovered. Look at the existing walk to confirm; if it doesn't yet, add the map population.

6. `filesToScan` and `allFiles` — verify these variable names match what's in the current scan function; rename as needed.

- [ ] **Step 4: Add `ListForFile` to fingerprints service**

In `pkg/fingerprints/service.go`, add:

```go
// ListForFile returns all fingerprints for a file matching the given algorithm.
func (svc *Service) ListForFile(ctx context.Context, fileID int, algorithm string) ([]*models.FileFingerprint, error) {
	var fps []*models.FileFingerprint
	err := svc.db.
		NewSelect().
		Model(&fps).
		Where("file_id = ?", fileID).
		Where("algorithm = ?", algorithm).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return fps, nil
}
```

- [ ] **Step 5: Add helpers to ScanCache**

In `pkg/worker/scan_cache.go`, add to the `ScanCache` struct:

```go
type ScanCache struct {
	// ... existing fields ...

	movedOrphanIDs   map[int]struct{}
	movedOrphanMu    sync.RWMutex
	knownFilesMu     sync.Mutex // protects knownFiles for late additions
}
```

Add methods:

```go
// AddKnownFile registers a file in the known-files map after the initial
// preload. Used by the move reconciliation phase to mark a moved orphan as
// known at its new path.
func (c *ScanCache) AddKnownFile(f *models.File) {
	c.knownFilesMu.Lock()
	defer c.knownFilesMu.Unlock()
	if c.knownFiles == nil {
		c.knownFiles = make(map[string]*models.File)
	}
	c.knownFiles[f.Filepath] = f
}

// SetMovedOrphanIDs records which file IDs were repurposed by move
// reconciliation so orphan cleanup can skip them.
func (c *ScanCache) SetMovedOrphanIDs(ids map[int]struct{}) {
	c.movedOrphanMu.Lock()
	defer c.movedOrphanMu.Unlock()
	c.movedOrphanIDs = ids
}

// IsMovedOrphan reports whether the given file ID was repurposed by move
// reconciliation and should therefore be skipped by orphan cleanup.
func (c *ScanCache) IsMovedOrphan(id int) bool {
	c.movedOrphanMu.RLock()
	defer c.movedOrphanMu.RUnlock()
	_, ok := c.movedOrphanIDs[id]
	return ok
}
```

Also: the existing `GetKnownFile` may already use `sync.Map`. If so, add an insert there as well. Read the file to see which approach is in use and keep both patterns consistent (or migrate to one).

- [ ] **Step 6: Update orphan cleanup to skip moved orphans**

In `pkg/worker/scan_orphans.go`, find `cleanupOrphanedFiles` (or equivalent). It currently iterates files and deletes those not in `scannedPaths`. Update it to also check the cache's `IsMovedOrphan` and skip those IDs.

The signature may need a `cache *ScanCache` parameter if it doesn't already have one. Update callers to pass it.

- [ ] **Step 7: Run the tests and verify they pass**

Run: `go test ./pkg/worker/... -run TestScanReconciliation -v`
Expected: PASS for all three test cases.

- [ ] **Step 8: Run the full worker test suite**

Run: `go test ./pkg/worker/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_cache.go pkg/worker/scan_orphans.go pkg/fingerprints/service.go pkg/worker/scan_move_reconciliation_test.go
git commit -m "[Backend] Add move reconciliation phase to scan job"
```

---

## Task 11: Monitor — move detection via sync hash on mixed batches

**Files:**
- Modify: `pkg/worker/monitor.go`

This is the most complex task. Read `pkg/worker/monitor.go` in full first, especially:
- `processPendingEvents` (lines 467-556)
- `processDirectoryEvent` (lines 578-608)
- `processEvent` (lines 611-668)
- `pendingEvent` struct (lines 18-27)

The goal is to intercept the existing event processing to:
1. Detect whether the current batch contains any REMOVE events (`needsSyncHash`)
2. When processing file CREATE events, if `needsSyncHash` is true, compute sha256 inline and check for an existing fingerprint match in the library
3. If a match is found and the matched file's path is gone from disk, update that file's `filepath` to the new path (move detected) and skip the "create new" step
4. REMOVE/directory-remove processing must skip file IDs that were repurposed as move targets

- [ ] **Step 1: Write the tests**

Create `pkg/worker/monitor_move_test.go`:

```go
package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestMonitor_DetectsFileMove simulates a rename by injecting a REMOVE event
// for the old path and a CREATE event for the new path, both in the same
// pending-events batch. After processing, the original file row should now
// point at the new path (not deleted and recreated).
func TestMonitor_DetectsFileMove(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "book.epub")
	newPath := filepath.Join(dir, "new", "book.epub")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))

	// Write to old first, so it exists, compute fingerprint, then move (delete old).
	content := []byte("epub data")
	require.NoError(t, os.WriteFile(oldPath, content, 0o644))

	library := harness.InsertLibrary(t, dir)
	file := harness.InsertFile(t, library.ID, oldPath)
	hash, err := fingerprint.ComputeSHA256(oldPath)
	require.NoError(t, err)
	harness.InsertFingerprint(t, file.ID, models.FingerprintAlgorithmSHA256, hash)

	// Simulate the filesystem rename: remove old, create at new path.
	require.NoError(t, os.Rename(oldPath, newPath))

	// Inject events into monitor and process.
	harness.EnqueueMonitorEvent(t, oldPath, fsnotify.Remove, library.ID)
	harness.EnqueueMonitorEvent(t, newPath, fsnotify.Create, library.ID)
	harness.ProcessMonitorEvents(t, context.Background())

	updated := harness.GetFile(t, file.ID)
	require.NotNil(t, updated, "file row should still exist")
	require.Equal(t, newPath, updated.Filepath)
}

// TestMonitor_CreateOnlyBatch_NoSyncHashing verifies that a CREATE-only
// batch does not compute hashes inline and queues a hash generation job instead.
func TestMonitor_CreateOnlyBatch_NoSyncHashing(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	newPath := filepath.Join(dir, "new.epub")
	require.NoError(t, os.WriteFile(newPath, []byte("content"), 0o644))

	library := harness.InsertLibrary(t, dir)

	harness.EnqueueMonitorEvent(t, newPath, fsnotify.Create, library.ID)
	harness.ProcessMonitorEvents(t, context.Background())

	// File row was created but no fingerprint yet (async path).
	f := harness.FindFileByPath(t, library.ID, newPath)
	require.NotNil(t, f)
	require.Equal(t, 0, harness.CountFingerprints(t, f.ID))

	// A hash generation job should be queued.
	jobs := harness.ListPendingJobs(t, models.JobTypeHashGeneration, library.ID)
	require.Len(t, jobs, 1)
}

// TestMonitor_PathStillExists_TreatAsCopy verifies that when a hash match is
// found but the old file's path still exists on disk, the new file is created
// as a fresh row (copy semantics), not a move.
func TestMonitor_PathStillExists_TreatAsCopy(t *testing.T) {
	t.Parallel()
	harness := testutils.NewWorkerHarness(t)

	dir := t.TempDir()
	origPath := filepath.Join(dir, "orig.epub")
	copyPath := filepath.Join(dir, "copy.epub")
	content := []byte("duplicate content")
	require.NoError(t, os.WriteFile(origPath, content, 0o644))
	require.NoError(t, os.WriteFile(copyPath, content, 0o644))

	library := harness.InsertLibrary(t, dir)
	origFile := harness.InsertFile(t, library.ID, origPath)
	hash, _ := fingerprint.ComputeSHA256(origPath)
	harness.InsertFingerprint(t, origFile.ID, models.FingerprintAlgorithmSHA256, hash)

	// Inject a mixed batch: REMOVE for unrelated, CREATE for the copy.
	harness.EnqueueMonitorEvent(t, filepath.Join(dir, "unrelated.epub"), fsnotify.Remove, library.ID)
	harness.EnqueueMonitorEvent(t, copyPath, fsnotify.Create, library.ID)
	harness.ProcessMonitorEvents(t, context.Background())

	// Original file row should still point at origPath.
	orig := harness.GetFile(t, origFile.ID)
	require.Equal(t, origPath, orig.Filepath)

	// A new file row should exist for copyPath.
	copyFile := harness.FindFileByPath(t, library.ID, copyPath)
	require.NotNil(t, copyFile)
	require.NotEqual(t, origFile.ID, copyFile.ID)
}
```

**Note:** `testutils.EnqueueMonitorEvent` and `ProcessMonitorEvents` are hypothetical test helpers. To support them, you may need to add a small test-only method to `Monitor` that inserts a synthetic pending event and directly calls `processPendingEvents`. The simplest approach: add `func (m *Monitor) InjectEventForTest(path string, op fsnotify.Op, libID int)` and `func (m *Monitor) ProcessPendingEventsForTest()` in `monitor_export_test.go` (which is only compiled during tests — name it `export_test.go` if you want it package-internal and accessible only to tests in the same package, or put it in `monitor.go` behind a build tag if needed).

Alternatively, drive the monitor through the real fsnotify watcher by creating/renaming actual files in `t.TempDir()` and waiting for the debounce to fire. This is more integration-level but exercises the full path.

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./pkg/worker/... -run TestMonitor`
Expected: FAIL — move is not detected; new file row created and old deleted.

- [ ] **Step 3: Refactor `processPendingEvents` to support move detection**

Modify `pkg/worker/monitor.go`. The plan:

1. Scan the pending map to compute `needsSyncHash` and to collect REMOVE events and CREATE events into separate slices.
2. Process CREATE events first. For each CREATE:
   - If `needsSyncHash`, compute sha256 for the file.
   - Query `fingerprintService.FindFilesByHash(ctx, libID, "sha256", hash)`.
   - For each match, `os.Stat` its stored `filepath`. Collect matches where stat fails (path gone).
   - If any such "displaced" match exists, pick the one with the most recent `file_modified_at` and update its `filepath` to the new path. Insert the file ID into a local `movedFileIDs` set. Skip the normal create-new path for this event.
   - Otherwise (no match, or all matches still exist), proceed with the existing create-new path (`processEvent`).
3. Process REMOVE and directory-remove events second. When dispatching, skip any file IDs in `movedFileIDs`.
4. At the end of the batch, if any new files were created without being matched as moves, call `EnsureHashGenerationJob(ctx, m.worker.jobService, libID)` for each library touched.

Here is the rewritten `processPendingEvents` (comments mark the new logic):

```go
func (m *Monitor) processPendingEvents() {
	if !m.processing.TryLock() {
		return
	}
	defer m.processing.Unlock()

	m.mu.Lock()
	events := m.pending
	m.pending = make(map[string]pendingEvent)
	m.mu.Unlock()

	if len(events) == 0 {
		return
	}

	ctx := context.Background()

	hasActive, err := m.worker.jobService.HasActiveJob(ctx, models.JobTypeScan, nil)
	if err != nil {
		m.log.Err(err).Warn("failed to check for active scan job, re-queuing events")
		m.requeue(events)
		return
	}
	if hasActive {
		m.log.Debug("scan job active, re-queuing events for later")
		m.requeue(events)
		return
	}

	m.log.Info("processing filesystem events", logger.Data{"count": len(events)})

	// --- NEW: Split events into creates vs removes and determine needsSyncHash ---
	type evt struct {
		path  string
		event pendingEvent
	}
	var createEvents []evt
	var removeEvents []evt
	needsSyncHash := false
	librariesWithNewFiles := make(map[int]struct{})
	for path, event := range events {
		isRemove := event.Op.Has(fsnotify.Remove) || event.Op.Has(fsnotify.Rename)
		isCreateOrWrite := event.Op.Has(fsnotify.Create) || event.Op.Has(fsnotify.Write)
		if isRemove && !isCreateOrWrite {
			removeEvents = append(removeEvents, evt{path: path, event: event})
			needsSyncHash = true
			continue
		}
		createEvents = append(createEvents, evt{path: path, event: event})
	}

	hadDeletes := false
	booksToOrganize := make(map[int]struct{})
	affectedBookIDs := make(map[int]struct{})
	movedFileIDs := make(map[int]struct{})
	applyResult := func(result *ScanResult) {
		if result == nil {
			return
		}
		if result.FileDeleted || result.BookDeleted {
			hadDeletes = true
		}
		if result.FileCreated && result.Book != nil {
			booksToOrganize[result.Book.ID] = struct{}{}
		}
		if result.Book != nil {
			affectedBookIDs[result.Book.ID] = struct{}{}
		}
	}

	// --- NEW: Process CREATE events with move detection ---
	for _, e := range createEvents {
		if e.event.IsDirectory {
			// Directory create — fan out via processDirectoryEvent (existing path).
			for _, result := range m.processDirectoryEvent(ctx, e.path, e.event) {
				applyResult(result)
			}
			continue
		}
		if needsSyncHash {
			// Try move detection.
			moved, err := m.tryDetectMove(ctx, e.path, e.event.LibraryID)
			if err != nil {
				m.log.Err(err).Warn("move detection failed, falling back to normal create", logger.Data{"path": e.path})
			}
			if moved != nil {
				movedFileIDs[moved.ID] = struct{}{}
				if moved.BookID != 0 {
					affectedBookIDs[moved.BookID] = struct{}{}
				}
				continue
			}
		}
		// Fall through to normal create/write processing.
		applyResult(m.processEvent(ctx, e.path, e.event))
		librariesWithNewFiles[e.event.LibraryID] = struct{}{}
	}

	// --- Process REMOVE events, skipping moved file IDs ---
	for _, e := range removeEvents {
		if e.event.IsDirectory {
			for _, result := range m.processDirectoryEventSkipping(ctx, e.path, e.event, movedFileIDs) {
				applyResult(result)
			}
			continue
		}
		// Look up the file; if it's in movedFileIDs, skip.
		file, err := m.worker.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{Filepath: &e.path})
		if err != nil {
			// File not in DB — nothing to do.
			continue
		}
		if _, skip := movedFileIDs[file.ID]; skip {
			m.log.Debug("skipping remove for moved file", logger.Data{"path": e.path, "file_id": file.ID})
			continue
		}
		applyResult(m.processEvent(ctx, e.path, e.event))
	}

	// Organize new books (existing behavior).
	if len(booksToOrganize) > 0 {
		m.organizeBooks(ctx, booksToOrganize)
	}

	if hadDeletes {
		m.runOrphanCleanup(ctx)
	}

	if m.worker.searchService != nil {
		for bookID := range affectedBookIDs {
			book, err := m.worker.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
			if err != nil {
				_ = m.worker.searchService.DeleteFromBookIndex(ctx, bookID)
				continue
			}
			if err := m.worker.searchService.IndexBook(ctx, book); err != nil {
				m.log.Warn("failed to index book", logger.Data{"book_id": bookID, "error": err.Error()})
			}
		}
	}

	// --- NEW: Ensure hash generation job for any library where new files were created ---
	for libID := range librariesWithNewFiles {
		if err := EnsureHashGenerationJob(ctx, m.worker.jobService, libID); err != nil {
			m.log.Warn("failed to ensure hash generation job", logger.Data{"library_id": libID, "error": err.Error()})
		}
	}
}

// tryDetectMove computes sha256 for a newly-appeared path and checks whether
// any existing file in the library has that fingerprint. If so, and the
// matched file's stored path is gone from disk, repurpose the matched file's
// filepath to the new path and return the updated file. Returns nil if no
// move was detected.
func (m *Monitor) tryDetectMove(ctx context.Context, path string, libraryID int) (*models.File, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	hash, err := fingerprint.ComputeSHA256(path)
	if err != nil {
		return nil, err
	}
	matches, err := m.worker.fingerprintService.FindFilesByHash(ctx, libraryID, models.FingerprintAlgorithmSHA256, hash)
	if err != nil {
		return nil, err
	}
	var displaced []*models.File
	for _, f := range matches {
		if _, err := os.Stat(f.Filepath); err != nil {
			displaced = append(displaced, f)
		}
	}
	if len(displaced) == 0 {
		return nil, nil // copy, not a move
	}
	// Pick the most recently modified displaced file.
	best := displaced[0]
	for _, f := range displaced[1:] {
		if f.FileModifiedAt != nil && best.FileModifiedAt != nil && f.FileModifiedAt.After(*best.FileModifiedAt) {
			best = f
		}
	}
	best.Filepath = path
	if err := m.worker.bookService.UpdateFile(ctx, best, books.UpdateFileOptions{Columns: []string{"filepath"}}); err != nil {
		return nil, errors.Wrap(err, "update filepath for moved file")
	}
	m.log.Info("monitor: detected move via hash", logger.Data{"file_id": best.ID, "new_path": path})
	return best, nil
}

// processDirectoryEventSkipping is like processDirectoryEvent but skips any
// file IDs that were repurposed as move targets earlier in this batch.
func (m *Monitor) processDirectoryEventSkipping(ctx context.Context, path string, event pendingEvent, skipIDs map[int]struct{}) []*ScanResult {
	log := m.log.Root(logger.Data{"path": path, "op": event.Op.String()})

	files, err := m.worker.bookService.ListFiles(ctx, books.ListFilesOptions{
		LibraryID:      &event.LibraryID,
		FilepathPrefix: &path,
	})
	if err != nil {
		log.Err(err).Warn("failed to list files under removed directory")
		return nil
	}
	if len(files) == 0 {
		return nil
	}

	results := make([]*ScanResult, 0, len(files))
	for _, file := range files {
		if _, skip := skipIDs[file.ID]; skip {
			continue
		}
		result, err := m.worker.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
		if err != nil {
			log.Err(err).Warn("failed to cleanup file under removed directory", logger.Data{"file_id": file.ID})
			continue
		}
		results = append(results, result)
	}
	return results
}
```

Also add the required imports to `monitor.go`:

```go
import (
	// existing imports ...
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fingerprint"
)
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./pkg/worker/... -run TestMonitor -v`
Expected: PASS for all three test cases.

- [ ] **Step 5: Run the full worker suite to check no regressions**

Run: `go test ./pkg/worker/...`
Expected: PASS.

- [ ] **Step 6: Run the full Go test suite**

Run: `mise test`
Expected: PASS (or at least no new failures from this change).

- [ ] **Step 7: Commit**

```bash
git add pkg/worker/monitor.go pkg/worker/monitor_move_test.go
git commit -m "[Backend] Add move detection to library monitor via sha256 hashing"
```

---

## Task 12: Documentation updates

**Files:**
- Modify: `pkg/CLAUDE.md`
- Modify: `website/docs/supported-formats.md`
- Modify: `website/docs/metadata.md`
- Create: `website/docs/file-fingerprints.md`

- [ ] **Step 1: Update `pkg/CLAUDE.md`**

Find the paragraph in `pkg/CLAUDE.md` that currently documents the monitor. It contains this text:

> File-identity across directory renames is not preserved — the old book row is deleted and a new one is created by the Create event on the new path; content-hash-based move detection is a separate ticket.

Replace that limitation note with:

> **Move detection via content hashing.** When the monitor sees a batch containing any REMOVE events, it computes sha256 synchronously for CREATE events in the same batch and looks up matches in `file_fingerprints`. If an existing file row has a matching sha256 and its stored path is gone from disk, the monitor repurposes that row's `filepath` rather than deleting + recreating. This preserves book identity and user-edited metadata across folder renames.
>
> The scan job performs the same reconciliation as a safety net: after the walk phase, orphans with sha256 fingerprints are matched against newly-discovered paths by `(size, sha256)` before orphan cleanup runs. Handles renames that happened while the server was offline.
>
> Sha256 hashes are populated by the `hash_generation` job, which is queued at the end of scan and at the end of every monitor batch that creates new files. Fingerprints are invalidated (deleted) whenever a file's size/mtime changes, so the next job run recomputes them against the new content.

- [ ] **Step 2: Create `website/docs/file-fingerprints.md`**

Create `website/docs/file-fingerprints.md`:

```markdown
---
sidebar_position: 11
---

# File Fingerprints & Move Detection

Shisho stores a sha256 content hash for every file in your library. This lets
the library monitor detect when you rename or move files on disk, so your
books keep their identity (and any metadata you've edited) instead of being
deleted and re-imported.

## How it works

Every file that Shisho tracks has an entry in the `file_fingerprints` table
with a sha256 of its contents. When Shisho sees a new file appear on disk, it
checks whether any existing file has the same sha256 whose stored path no
longer exists on disk. If so, the existing file is updated to point at the
new path — no duplicate book row is created, and your custom metadata stays
intact.

## When move detection runs

**In real time (library monitor)** — When you rename a folder in Finder or
Explorer, Shisho's filesystem watcher fires within a few seconds. If the batch
of events contains any deletions, Shisho computes sha256 synchronously for the
new files and looks for matches. Typical folder renames are detected
immediately.

**On the next scan (safety net)** — If you rename files while Shisho is not
running, the monitor won't see the events. The next scan reconciles the
library's state with disk — any file rows whose paths are missing get matched
against newly-discovered files by size and sha256 before being deleted.

## When does move detection *not* work?

- **On the very first scan after upgrading.** Fingerprints are populated by a
  background job that runs after each scan. Until that job has processed your
  library at least once, there are no fingerprints to match against. After
  the first hash-generation job completes, move detection is fully enabled.
- **If you rename a file AND change its contents at the same time.** The new
  file has a different sha256, so it doesn't match — Shisho treats it as a
  fresh import and deletes the old row.
- **Across libraries.** Move detection only works within a single library.
  Moving a file from one library to another is treated as a delete + create.

## Background hash generation

Shisho computes sha256 hashes asynchronously in a background job so large
audiobooks don't block the scan. You can see it running in the Jobs view with
progress like "Hashing files (142/500)". The job is queued automatically:

- At the end of every scan job, for any files in that library still missing
  a sha256 hash
- At the end of every monitor batch that created new files

The job is idempotent — running it multiple times does not produce duplicate
fingerprints, and a pending job is never created if one is already pending or
running for that library.

## Future fingerprint types

The `file_fingerprints` table is designed to hold more than just sha256.
Future Shisho releases will add perceptual/fuzzy fingerprints for detecting
duplicates that aren't byte-identical:

- **Cover pHash** — "same cover, different file" detection across formats
- **Text SimHash** — "same book, different edition" for EPUBs
- **Chromaprint** — acoustic fingerprints for audiobooks encoded differently
- **CBZ page pHash** — comic book rescans and re-encodes
- **TLSH** — fuzzy hash fallback for arbitrary formats

These will all share the same table and generation infrastructure as the
exact-match sha256 hashes.
```

- [ ] **Step 3: Add brief mention in `website/docs/metadata.md`**

Read `website/docs/metadata.md` first. Add a short section (2-3 sentences) near where file metadata is discussed, with a cross-link to `file-fingerprints.md`:

```markdown
## Content fingerprints

Shisho stores a sha256 hash of every file's contents to preserve file identity
across renames and moves. See [File Fingerprints](./file-fingerprints.md) for
details on how move detection works and how the feature degrades when the
monitor isn't running.
```

- [ ] **Step 4: Update `website/docs/supported-formats.md`**

Read the file first. Add a sentence near the top that notes fingerprinting is supported for all formats:

```markdown
All supported formats are fingerprinted with a content sha256 hash for move
and rename detection. Future versions will add format-specific fuzzy
fingerprints (cover pHash, text SimHash, etc.) — see
[File Fingerprints](./file-fingerprints.md).
```

- [ ] **Step 5: Verify docs build**

Run: `cd website && pnpm build` (or `mise docs:build` if that exists)
Expected: no broken links, no markdown errors.

If the docs don't build, check that the new file's sidebar position doesn't collide with existing files, and that all links resolve. Adjust `sidebar_position` as needed.

- [ ] **Step 6: Commit**

```bash
git add pkg/CLAUDE.md website/docs/file-fingerprints.md website/docs/metadata.md website/docs/supported-formats.md
git commit -m "[Docs] Document file fingerprinting and move detection"
```

---

## Task 13: Final verification

- [ ] **Step 1: Run `mise check:quiet`**

Run: `mise check:quiet`
Expected: all green. If anything fails, investigate and fix before marking this task complete.

- [ ] **Step 2: Smoke test the feature manually against a real DB**

```bash
# Start the dev server
mise start
```

In another terminal:

1. Add a small test library pointing at a directory with a few EPUB/PDF files
2. Trigger a scan via the API
3. Wait for the hash generation job to complete (check jobs UI)
4. Confirm `file_fingerprints` has rows: `sqlite3 tmp/data.sqlite "SELECT file_id, algorithm, substr(value,1,8) FROM file_fingerprints LIMIT 10"`
5. Rename a folder under the library path on disk
6. Wait ~10 seconds for the monitor debounce
7. Verify the book's `filepath` now points at the new path: `sqlite3 tmp/data.sqlite "SELECT id, filepath FROM files WHERE library_id = <libID>"`
8. Confirm no duplicate book row was created and no `UNIQUE constraint` errors appear in logs

- [ ] **Step 3: Run the full test suite one more time**

Run: `mise check`
Expected: all green.

- [ ] **Step 4: Final commit with any trailing fixes**

If any test fixes or lint adjustments were needed, commit them.

```bash
git add -A
git status
git commit -m "[Fix] Trailing fixes from final verification"
```

(Skip this step if there's nothing to commit.)

---

## Follow-up Notion Tasks (create after implementation lands)

After this MVP is merged, create separate Notion tasks for each of the following under the Shisho Tasks board (https://www.notion.so/31df24d3107d80ac8669dcf7281c8537). **Do NOT create a Cross-library move detection task** — per the user's decision, that's not being tracked.

1. **Cover pHash** — image fingerprints for all formats. Add `AlgorithmPHashCover` to `pkg/fingerprint/`, hook into hash generation job.
2. **Text SimHash for EPUBs** — new algorithm, reuses the same table and job infrastructure.
3. **CBZ per-page pHash** — comic rescans.
4. **Image-based PDF per-page pHash** — scanned books.
5. **Chromaprint for M4B/MP3** — audiobook acoustic fingerprints.
6. **TLSH fallback** — generic fuzzy byte hash.
7. **Dedup UI** — surface potential duplicates, user-facing merge/delete actions, handles the "auto-associate copies with same book" decision.
