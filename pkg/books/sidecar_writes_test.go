package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
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
