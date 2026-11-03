package worker

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestProcessRecomputeReviewJob_PopulatesReviewed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := tc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "T",
		TitleSource:     "filepath",
		SortTitle:       "T",
		SortTitleSource: "filepath",
		AuthorSource:    "filepath",
		Filepath:        "/tmp",
	}
	_, err = tc.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	main := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/m.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	_, err = tc.db.NewInsert().Model(main).Exec(ctx)
	require.NoError(t, err)
	supp := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/s.pdf",
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1,
	}
	_, err = tc.db.NewInsert().Model(supp).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":false}`,
	}
	_, err = tc.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))

	var mainReviewed *bool
	require.NoError(t, tc.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", main.ID).Scan(ctx, &mainReviewed))
	require.NotNil(t, mainReviewed)
	require.False(t, *mainReviewed)

	var suppReviewed *bool
	require.NoError(t, tc.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", supp.ID).Scan(ctx, &suppReviewed))
	require.Nil(t, suppReviewed)
}

func TestProcessRecomputeReviewJob_ClearOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := tc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "T",
		TitleSource:     "filepath",
		SortTitle:       "T",
		SortTitleSource: "filepath",
		AuthorSource:    "filepath",
		Filepath:        "/tmp",
	}
	_, err = tc.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	override := models.ReviewOverrideReviewed
	file := &models.File{
		LibraryID:      library.ID,
		BookID:         book.ID,
		Filepath:       "/tmp/x.epub",
		FileType:       models.FileTypeEPUB,
		FileRole:       models.FileRoleMain,
		FilesizeBytes:  1,
		ReviewOverride: &override,
	}
	_, err = tc.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":true}`,
	}
	_, err = tc.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))

	var got models.File
	require.NoError(t, tc.db.NewSelect().Model(&got).Where("f.id = ?", file.ID).Scan(ctx))
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
	require.NotNil(t, got.Reviewed)
	require.False(t, *got.Reviewed)
}

// afterQueryHook calls fn after every query.
type afterQueryHook struct {
	fn func(event *bun.QueryEvent)
}

func (h *afterQueryHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *afterQueryHook) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	h.fn(event)
}

// seedRecomputeJob inserts bookCount Books with one main File marked reviewed,
// which the default criteria flip to not reviewed, and a pending job.
func seedRecomputeJob(ctx context.Context, t *testing.T, tc *testContext, bookCount int) *models.Job {
	t.Helper()
	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := tc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	reviewed := true
	for i := range bookCount {
		path := fmt.Sprintf("/tmp/book-%d", i)
		book := &models.Book{LibraryID: library.ID, Title: path, TitleSource: "filepath", SortTitle: path, SortTitleSource: "filepath", AuthorSource: "filepath", Filepath: path}
		_, err := tc.db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: path + "/a.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, FilesizeBytes: 1, Reviewed: &reviewed}
		_, err = tc.db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}
	job := &models.Job{Type: models.JobTypeRecomputeReview, Status: models.JobStatusPending, Data: `{"clear_overrides":false}`}
	_, err = tc.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)
	return job
}

func filesStillReviewed(ctx context.Context, t *testing.T, tc *testContext) int {
	t.Helper()
	count, err := tc.db.NewSelect().Model((*models.File)(nil)).Where("reviewed = ?", true).Count(ctx)
	require.NoError(t, err)
	return int(count)
}

// The job covers a whole library, so its query count must follow the number of
// batches rather than the number of files.
func TestProcessRecomputeReviewJob_QueryCountFollowsBatchesNotFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	queriesFor := func(bookCount int) int64 {
		tc := newTestContext(t)
		job := seedRecomputeJob(ctx, t, tc, bookCount)
		var queries atomic.Int64
		testdb.AddQueryHook(t, tc.db, &afterQueryHook{fn: func(*bun.QueryEvent) { queries.Add(1) }})
		require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))
		assert.Zero(t, filesStillReviewed(ctx, t, tc))
		return queries.Load()
	}

	assert.Equal(t, queriesFor(2), queriesFor(40))
}

// The job reports progress after each batch, ends at 100, and reaches books
// past a batch boundary.
func TestProcessRecomputeReviewJob_CompletesAcrossBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)
	job := seedRecomputeJob(ctx, t, tc, review.RecomputeBatchSize+1)
	var progressWrites atomic.Int64
	testdb.AddQueryHook(t, tc.db, &afterQueryHook{fn: func(event *bun.QueryEvent) {
		if strings.Contains(event.Query, "progress") {
			progressWrites.Add(1)
		}
	}})

	require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))

	assert.Zero(t, filesStillReviewed(ctx, t, tc))
	assert.Equal(t, int64(2), progressWrites.Load())
	var got models.Job
	require.NoError(t, tc.db.NewSelect().Model(&got).Where("j.id = ?", job.ID).Scan(ctx))
	assert.Equal(t, 100, got.Progress)
}

// A shutdown stops the job between batches: the finished batch stays written
// and the next one doesn't start.
func TestProcessRecomputeReviewJob_StopsBetweenBatchesWhenCanceled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)
	remaining := 1
	job := seedRecomputeJob(ctx, t, tc, review.RecomputeBatchSize+remaining)
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	testdb.AddQueryHook(t, tc.db, &afterQueryHook{fn: func(event *bun.QueryEvent) {
		if strings.Contains(event.Query, "progress") {
			cancel()
		}
	}})

	err := tc.worker.ProcessRecomputeReviewJob(jobCtx, job, nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, remaining, filesStillReviewed(ctx, t, tc), "only the batch after the cancel is left")
}
