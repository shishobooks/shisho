package worker

import (
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Book whose files are gone from disk is deleted, but its readers keep
// their Read-throughs of it, with its title and authors (ADR 0015).
func TestCleanupOrphanedFiles_KeepsReadThroughsOfDeletedBooks(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Single File")
	testgen.GenerateEPUB(t, bookDir, "only.epub", testgen.EPUBOptions{
		Title:   "Single File",
		Authors: []string{"Ann Author"},
	})
	require.NoError(t, tc.runScan())
	books := tc.listBooks()
	require.Len(t, books, 1)

	user := &models.User{Username: "reader", PasswordHash: "x", RoleID: 2, IsActive: true}
	_, err := tc.db.NewInsert().Model(user).Exec(tc.ctx)
	require.NoError(t, err)
	now := time.Now()
	endDay := "2026-03-01"
	rt := &models.ReadThrough{CreatedAt: now, UpdatedAt: now, UserID: user.ID, BookID: &books[0].ID, EndDay: &endDay, Outcome: models.ReadThroughOutcomeFinished}
	_, err = tc.db.NewInsert().Model(rt).Exec(tc.ctx)
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, logger.FromContext(tc.ctx))
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, map[string]struct{}{}, library, jobLog)

	assert.Empty(t, tc.listBooks())
	kept := &models.ReadThrough{}
	require.NoError(t, tc.db.NewSelect().Model(kept).Where("rt.id = ?", rt.ID).Scan(tc.ctx))
	assert.Nil(t, kept.BookID)
	require.NotNil(t, kept.BookTitle)
	assert.Equal(t, "Single File", *kept.BookTitle)
	assert.Equal(t, []string{"Ann Author"}, kept.BookAuthors)
}
