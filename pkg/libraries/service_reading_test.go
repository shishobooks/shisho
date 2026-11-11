package libraries

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a library deletes its Books, and like any Book delete it keeps
// their readers' Read-throughs with the Books' titles (ADR 0015).
func TestDeleteLibrary_KeepsReadThroughsOfItsBooks(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	seeded := seedLibraryWithContent(ctx, t, db, "Doomed")

	user := &models.User{Username: "reader", PasswordHash: "x", RoleID: 2, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	now := time.Now()
	rt := &models.ReadThrough{CreatedAt: now, UpdatedAt: now, UserID: user.ID, BookID: &seeded.BookID, Outcome: models.ReadThroughOutcomeUnderway}
	_, err = db.NewInsert().Model(rt).Exec(ctx)
	require.NoError(t, err)
	var title string
	require.NoError(t, db.NewSelect().Model((*models.Book)(nil)).Column("title").Where("id = ?", seeded.BookID).Scan(ctx, &title))

	require.NoError(t, NewService(db).DeleteLibrary(ctx, seeded.LibraryID))

	kept := &models.ReadThrough{}
	require.NoError(t, db.NewSelect().Model(kept).Where("rt.id = ?", rt.ID).Scan(ctx))
	assert.Nil(t, kept.BookID)
	require.NotNil(t, kept.BookTitle)
	assert.Equal(t, title, *kept.BookTitle)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, kept.Outcome, "it isn't ended for the reader")
}
