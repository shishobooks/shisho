package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bun writes a zero time.Time instead of omitting the column, so the
// table's DEFAULT CURRENT_TIMESTAMP never applies. First-run setup must set
// both timestamps itself.
func TestServiceCreateFirstAdmin_SetsTimestamps(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour)

	user, err := svc.CreateFirstAdmin(context.Background(), "admin", nil, "securepassword123")
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now(), user.CreatedAt, time.Minute)
	assert.WithinDuration(t, time.Now(), user.UpdatedAt, time.Minute)
}
