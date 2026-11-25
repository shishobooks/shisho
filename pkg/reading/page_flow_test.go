package reading

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Page flow chosen later keeps over one chosen earlier and sent late, as
// from a phone that was offline, and a merge carries when the flow it keeps
// was chosen.
func TestSetPageFlowAt_ALaterChoiceKeeps(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	scrolled, paginated := "scrolled", "paginated"
	flow := func(bookID int) *string {
		var got *string
		require.NoError(t, f.db.NewRaw(`SELECT page_flow FROM user_books WHERE user_id = ? AND book_id = ?`, f.user.ID, bookID).
			Scan(f.ctx, &got))
		return got
	}

	saved, err := SetPageFlowAt(f.ctx, f.db, f.user.ID, f.book.ID, &scrolled, now)
	require.NoError(t, err)
	assert.True(t, saved)
	saved, err = SetPageFlowAt(f.ctx, f.db, f.user.ID, f.book.ID, &paginated, now.Add(-time.Minute))
	require.NoError(t, err)
	assert.False(t, saved)
	assert.Equal(t, &scrolled, flow(f.book.ID))

	target, _ := f.addBook("Target")
	_, err = f.db.NewRaw(`INSERT INTO user_books (user_id, book_id, rating) VALUES (?, ?, 10)`, f.user.ID, target.ID).Exec(f.ctx)
	require.NoError(t, err)
	require.NoError(t, MergeBooks(f.ctx, f.db, f.book.ID, target.ID))
	assert.Equal(t, &scrolled, flow(target.ID))
	saved, err = SetPageFlowAt(f.ctx, f.db, f.user.ID, target.ID, &paginated, now.Add(-time.Minute))
	require.NoError(t, err)
	assert.False(t, saved)
}
