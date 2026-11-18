package books

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Book's stale mark is cleared only after its sidecars are written, and
// only when no rename marked it again in the meantime. Clearing first left a
// Book unmarked with an old sidecar when the server died mid-write.
func TestRewriteBookSidecars_ClearsTheMarkOnlyAfterWriting(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))
	_, book, _ := seedBookAndFile(t, db, "A Book", nil, nil, models.FileRoleMain)

	marked := time.Now().Add(-time.Minute)
	_, err := db.NewUpdate().Model((*models.Book)(nil)).Set("sidecars_stale_at = ?", marked).Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)

	// A rename marks the Book again while its sidecars are being written.
	svc.beforeClearStaleMark = func() {
		_, err := db.NewUpdate().Model((*models.Book)(nil)).Set("sidecars_stale_at = ?", time.Now()).Where("id = ?", book.ID).Exec(ctx)
		require.NoError(t, err)
	}
	require.NoError(t, svc.rewriteBookSidecars(ctx, book.ID))
	assert.True(t, isMarked(t, svc, book.ID), "a mark set during the write must stay for the next pass")

	// With nothing marking it in between, the pass clears it.
	svc.beforeClearStaleMark = nil
	require.NoError(t, svc.rewriteBookSidecars(ctx, book.ID))
	assert.False(t, isMarked(t, svc, book.ID))
}

func isMarked(t *testing.T, svc *Service, bookID int) bool {
	t.Helper()
	var count int
	require.NoError(t, svc.db.NewSelect().Model((*models.Book)(nil)).
		Where("b.id = ? AND b.sidecars_stale_at IS NOT NULL", bookID).
		ColumnExpr("COUNT(*)").Scan(context.Background(), &count))
	return count > 0
}

// A File created after the mark keeps its sidecar: it can't name the old
// name, and may be one carried over from another library that the scan
// importing the File has yet to read.
func TestRewriteBookSidecars_LeavesNewFilesSidecars(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))
	_, book, file := seedBookAndFile(t, db, "A Book", nil, nil, models.FileRoleMain)
	require.NoError(t, os.WriteFile(file.Filepath, []byte("x"), 0o644))

	marked := time.Now().Add(-time.Hour)
	_, err := db.NewUpdate().Model((*models.Book)(nil)).Set("sidecars_stale_at = ?", marked).Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)
	// The scan importing the File created it after the mark.
	_, err = db.NewUpdate().Model((*models.File)(nil)).Set("created_at = ?", time.Now()).Where("id = ?", file.ID).Exec(ctx)
	require.NoError(t, err)
	carried := []byte(`{"version":1,"publisher":"Carried Press"}`)
	require.NoError(t, os.WriteFile(sidecar.FileSidecarPath(file.Filepath), carried, 0o644))

	require.NoError(t, svc.rewriteBookSidecars(ctx, book.ID))

	got, err := os.ReadFile(sidecar.FileSidecarPath(file.Filepath))
	require.NoError(t, err)
	assert.Equal(t, carried, got, "a File newer than the mark keeps its sidecar")
	assert.False(t, isMarked(t, svc, book.ID))
}

// When a scan or edit changes the Book while its sidecars are written, the
// written snapshot may be older than that writer's sidecar, so the Book is
// marked again, with its original mark time.
func TestRewriteBookSidecars_MarksAgainWhenTheBookChangedMeanwhile(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))
	_, book, _ := seedBookAndFile(t, db, "A Book", nil, nil, models.FileRoleMain)

	marked := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	_, err := db.NewUpdate().Model((*models.Book)(nil)).Set("sidecars_stale_at = ?", marked).Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)
	svc.beforeClearStaleMark = func() {
		// A scan clears the mark itself and stores newer values.
		_, err := db.NewUpdate().Model((*models.Book)(nil)).
			Set("sidecars_stale_at = NULL").Set("updated_at = ?", time.Now().Add(time.Second)).
			Where("id = ?", book.ID).Exec(ctx)
		require.NoError(t, err)
	}

	require.NoError(t, svc.rewriteBookSidecars(ctx, book.ID))

	var stored models.Book
	require.NoError(t, db.NewSelect().Model(&stored).Where("b.id = ?", book.ID).Scan(ctx))
	require.NotNil(t, stored.SidecarsStaleAt, "the Book is marked again for the next pass")
	assert.True(t, stored.SidecarsStaleAt.Equal(marked), "the original mark time is kept, got %v", stored.SidecarsStaleAt)
}

// A merge marks only the Books its source held.
func TestMarkBookSidecarsStale(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))
	_, book, _ := seedBookAndFile(t, db, "A Book", nil, nil, models.FileRoleMain)

	svc.MarkBookSidecarsStale(ctx, nil)
	assert.False(t, isMarked(t, svc, book.ID))

	svc.MarkBookSidecarsStale(ctx, []int{book.ID})
	assert.True(t, isMarked(t, svc, book.ID))
	var jobs int
	require.NoError(t, db.NewSelect().Model((*models.Job)(nil)).Where("type = ?", models.JobTypeSidecarRewrite).ColumnExpr("COUNT(*)").Scan(ctx, &jobs))
	assert.Equal(t, 1, jobs)
}
